import os
import json
import hashlib
from glob import glob
from io import BytesIO
from typing import Dict, List, Optional, Tuple

import cv2
import numpy as np
import onnxruntime as ort
import pydicom
from fastapi import FastAPI, File, HTTPException, UploadFile
from PIL import Image
from pydantic import BaseModel


LABEL_PNEUMONIA = "肺炎(Pneumonia)"
LABEL_NODULE = "结节(Nodule)"
LABEL_MASS = "肿块(Mass)"
LABEL_OPACITY = "浸润/实变(Opacity)"
TASK_LABELS = [LABEL_PNEUMONIA, LABEL_NODULE, LABEL_MASS, LABEL_OPACITY]
TASK_PNEUMONIA = "pneumonia"
TASK_LESION = "lesion"


class Region(BaseModel):
    x: float
    y: float
    width: float
    height: float
    confidence: float
    label: str


class PredictResponse(BaseModel):
    modelVersion: str
    cancerProbability: float
    regions: List[Region]
    labelScores: Dict[str, float]
    infectionCoverage: Dict[str, float]
    whiteLungAssessment: Dict[str, object]
    topFindings: List[str]
    calibrationTemperature: float
    decisionHighSensitivity: bool
    decisionHighSpecificity: bool
    taskDecisions: Dict[str, Dict[str, bool]]
    operatingPointsUsed: Dict[str, Dict[str, float]]
    clinicalUse: str
    clinicalStage: str
    detectorModelLoaded: bool
    heatmapPath: Optional[str] = None


class ModelRunner:
    def __init__(self) -> None:
        self.model_path = os.getenv("AI_MODEL_PATH", "./models/cxr_multitask.onnx")
        self.model_version = os.getenv("AI_MODEL_VERSION", "cxr-multitask-v1")
        self.input_size = int(os.getenv("AI_INPUT_SIZE", "224"))
        self.calibration_temperature = float(os.getenv("AI_CALIBRATION_TEMPERATURE", "1.6"))
        self.high_sensitivity_threshold = float(os.getenv("AI_THRESHOLD_HIGH_SENS", "0.30"))
        self.high_specificity_threshold = float(os.getenv("AI_THRESHOLD_HIGH_SPEC", "0.70"))
        self.clinical_config_path = os.getenv("AI_CLINICAL_CONFIG_PATH", "./models/clinical_config.json")
        self.enable_heuristic_regions = (
            os.getenv("AI_ENABLE_HEURISTIC_REGIONS", "false").lower() == "true"
        )
        self.detector_model_path = os.getenv("AI_DETECTOR_MODEL_PATH", "./models/cxr_detector.onnx")
        self.detector_input_size = int(os.getenv("AI_DETECTOR_INPUT_SIZE", "640"))
        self.detector_input_width = self.detector_input_size
        self.detector_input_height = self.detector_input_size
        self.detector_conf_threshold = float(os.getenv("AI_DETECTOR_CONF_THRESHOLD", "0.25"))
        self.detector_iou_threshold = float(os.getenv("AI_DETECTOR_IOU_THRESHOLD", "0.45"))
        self.detector_profile_path = os.getenv(
            "AI_DETECTOR_PROFILE_PATH", "./models/detector_profile.json"
        )
        self.detector_expected_sha256 = os.getenv("AI_DETECTOR_SHA256", "").strip().lower()
        self.enforce_detector_startup_check = (
            os.getenv("AI_ENFORCE_DETECTOR_STARTUP_CHECK", "true").lower() == "true"
        )
        self.enforce_governance_gate = (
            os.getenv("AI_ENFORCE_GOVERNANCE_GATE", "true").lower() == "true"
        )
        self.clinical_governance_path = os.getenv(
            "AI_CLINICAL_GOVERNANCE_PATH", "./models/clinical_governance.json"
        )
        self.governance_min_site_count = int(os.getenv("AI_GOV_MIN_SITE_COUNT", "2"))
        self.governance_min_auroc = float(os.getenv("AI_GOV_MIN_AUROC", "0.90"))
        self.governance_min_sensitivity = float(os.getenv("AI_GOV_MIN_SENSITIVITY", "0.90"))
        self.governance_min_specificity = float(os.getenv("AI_GOV_MIN_SPECIFICITY", "0.85"))
        self.task_thresholds: Dict[str, Dict[str, float]] = {
            TASK_LESION: {
                "high_sensitivity_threshold": self.high_sensitivity_threshold,
                "high_specificity_threshold": self.high_specificity_threshold,
            },
            TASK_PNEUMONIA: {
                "high_sensitivity_threshold": self.high_sensitivity_threshold,
                "high_specificity_threshold": self.high_specificity_threshold,
            },
        }
        self.config_source = "env-default"
        self.clinical_stage = "RESEARCH_ONLY"
        self.governance_gate_passed = True
        self.governance_gate_message = "research-only mode"
        self.detector_session: Optional[ort.InferenceSession] = None
        self.detector_input_name: Optional[str] = None
        self.detector_output_names: List[str] = []
        self.detector_check_passed = False
        self.detector_check_message = "detector not checked"
        self.detector_sha256: Optional[str] = None
        self.detector_profile: Dict[str, object] = {
            "decoder_format": "auto",  # auto | yolo | xyxy_cls
            "class_map": {
                "0": LABEL_NODULE,
                "1": LABEL_MASS,
                "2": LABEL_OPACITY,
                "3": LABEL_PNEUMONIA,
            },
            "input_scale": 1.0,
            "input_mean": [0.0],
            "input_std": [1.0],
            "input_channels": 1,
        }
        self.session: Optional[ort.InferenceSession] = None
        self.input_name: Optional[str] = None
        self.output_names: List[str] = []
        self.model_error: Optional[str] = None
        self._try_load_model()

    @staticmethod
    def _file_sha256(path: str) -> str:
        h = hashlib.sha256()
        with open(path, "rb") as f:
            for chunk in iter(lambda: f.read(1024 * 1024), b""):
                h.update(chunk)
        return h.hexdigest().lower()

    def _load_clinical_config(self) -> None:
        if not os.path.exists(self.clinical_config_path):
            return

        try:
            with open(self.clinical_config_path, "r", encoding="utf-8") as f:
                payload = json.load(f)

            global_cfg = payload.get("global", {})
            if isinstance(global_cfg, dict):
                temp = global_cfg.get("temperature")
                if isinstance(temp, (int, float)) and temp > 0:
                    self.calibration_temperature = float(temp)

            tasks_cfg = payload.get("tasks", {})
            if isinstance(tasks_cfg, dict):
                for task in [TASK_PNEUMONIA, TASK_LESION]:
                    task_cfg = tasks_cfg.get(task, {})
                    if not isinstance(task_cfg, dict):
                        continue
                    hs = task_cfg.get("high_sensitivity_threshold")
                    hp = task_cfg.get("high_specificity_threshold")
                    if isinstance(hs, (int, float)):
                        self.task_thresholds[task]["high_sensitivity_threshold"] = float(hs)
                    if isinstance(hp, (int, float)):
                        self.task_thresholds[task]["high_specificity_threshold"] = float(hp)

            # keep aggregate compatibility fields bound to lesion task
            self.high_sensitivity_threshold = self.task_thresholds[TASK_LESION]["high_sensitivity_threshold"]
            self.high_specificity_threshold = self.task_thresholds[TASK_LESION]["high_specificity_threshold"]
            self.config_source = self.clinical_config_path
        except Exception as exc:
            self.model_error = f"Clinical config load failed: {exc}"

    def _load_governance_config(self) -> None:
        if not os.path.exists(self.clinical_governance_path):
            self.clinical_stage = "RESEARCH_ONLY"
            self.governance_gate_passed = True
            self.governance_gate_message = "governance file missing; stage forced to RESEARCH_ONLY"
            return

        try:
            with open(self.clinical_governance_path, "r", encoding="utf-8") as f:
                payload = json.load(f)
            stage = str(payload.get("deploymentStage", "RESEARCH_ONLY")).upper()
            allowed = {"RESEARCH_ONLY", "PILOT_DECISION_SUPPORT", "CLINICAL_DECISION_SUPPORT"}
            self.clinical_stage = stage if stage in allowed else "RESEARCH_ONLY"
            ok, msg = self._evaluate_governance_gate(payload)
            self.governance_gate_passed = ok
            self.governance_gate_message = msg
            if self.enforce_governance_gate and not ok:
                raise RuntimeError(f"Governance gate failed: {msg}")
        except Exception as exc:
            self.clinical_stage = "RESEARCH_ONLY"
            self.governance_gate_passed = False
            self.governance_gate_message = f"governance parse/eval failed: {exc}"

    def _load_detector_profile(self) -> None:
        if not os.path.exists(self.detector_profile_path):
            return
        try:
            with open(self.detector_profile_path, "r", encoding="utf-8") as f:
                payload = json.load(f)
            if isinstance(payload, dict):
                self.detector_profile = {**self.detector_profile, **payload}
        except Exception as exc:
            if self.enforce_detector_startup_check:
                raise RuntimeError(f"Detector profile load failed: {exc}") from exc

    def _evaluate_governance_gate(self, payload: Dict[str, object]) -> Tuple[bool, str]:
        if self.clinical_stage == "RESEARCH_ONLY":
            return True, "research-only mode"

        validation = payload.get("validationEvidence", {})
        if not isinstance(validation, dict):
            return False, "validationEvidence missing"

        external = validation.get("externalValidation", {})
        if not isinstance(external, dict):
            return False, "externalValidation missing"

        site_count = int(external.get("siteCount", 0))
        auroc = float(external.get("auroc", 0.0))
        sensitivity = float(external.get("sensitivity", 0.0))
        specificity = float(external.get("specificity", 0.0))

        if site_count < self.governance_min_site_count:
            return False, f"siteCount {site_count} < min {self.governance_min_site_count}"
        if auroc < self.governance_min_auroc:
            return False, f"auroc {auroc:.3f} < min {self.governance_min_auroc:.3f}"
        if sensitivity < self.governance_min_sensitivity:
            return False, f"sensitivity {sensitivity:.3f} < min {self.governance_min_sensitivity:.3f}"
        if specificity < self.governance_min_specificity:
            return False, f"specificity {specificity:.3f} < min {self.governance_min_specificity:.3f}"

        regulatory = validation.get("regulatory", {})
        if not isinstance(regulatory, dict):
            return False, "regulatory section missing"

        status = str(regulatory.get("status", "NOT_SUBMITTED")).upper()
        if self.clinical_stage == "PILOT_DECISION_SUPPORT":
            if status in {"NOT_SUBMITTED", "REJECTED"}:
                return False, f"regulatory status {status} not acceptable for pilot"
        if self.clinical_stage == "CLINICAL_DECISION_SUPPORT":
            if status not in {"APPROVED", "CLEARED", "CERTIFIED"}:
                return False, f"regulatory status {status} not acceptable for clinical"

        return True, f"passed for stage {self.clinical_stage}"

    def _try_load_model(self) -> None:
        self._load_clinical_config()
        self._load_governance_config()
        self._load_detector_profile()
        resolved_path = self._resolve_model_path(self.model_path)
        if not resolved_path:
            self.model_error = "No ONNX model file found in configured path or models directory"
            return

        try:
            self.session = ort.InferenceSession(resolved_path, providers=["CPUExecutionProvider"])
            self.input_name = self.session.get_inputs()[0].name
            self.output_names = [output.name for output in self.session.get_outputs()]
            self.model_path = resolved_path
            self.model_error = None
        except Exception as exc:
            self.model_error = f"Failed to load model: {exc}"
            self.session = None
            self.input_name = None
            self.output_names = []

        self._try_load_detector()

    def _try_load_detector(self) -> None:
        resolved = self._resolve_model_path(self.detector_model_path)
        if not resolved:
            self.detector_session = None
            self.detector_input_name = None
            self.detector_output_names = []
            self.detector_check_passed = False
            self.detector_check_message = "detector model not found"
            return
        try:
            so = ort.SessionOptions()
            so.enable_mem_pattern = False
            so.enable_cpu_mem_arena = True
            so.intra_op_num_threads = 1
            so.inter_op_num_threads = 1
            self.detector_session = ort.InferenceSession(
                resolved, sess_options=so, providers=["CPUExecutionProvider"]
            )
            self.detector_input_name = self.detector_session.get_inputs()[0].name
            self._sync_detector_input_shape()
            self.detector_output_names = [o.name for o in self.detector_session.get_outputs()]
            self.detector_model_path = resolved
            self.detector_sha256 = self._file_sha256(resolved)
            if self.detector_expected_sha256 and self.detector_sha256 != self.detector_expected_sha256:
                raise RuntimeError(
                    f"detector sha256 mismatch: actual={self.detector_sha256} expected={self.detector_expected_sha256}"
                )
            ok, msg = self._run_detector_startup_check()
            self.detector_check_passed = ok
            self.detector_check_message = msg
            if self.enforce_detector_startup_check and not ok:
                raise RuntimeError(f"Detector startup check failed: {msg}")
        except Exception as exc:
            self.detector_session = None
            self.detector_input_name = None
            self.detector_output_names = []
            self.detector_check_passed = False
            self.detector_check_message = f"detector init or check failed: {exc}"

    def _sync_detector_input_shape(self) -> None:
        if self.detector_session is None:
            return
        shape = self.detector_session.get_inputs()[0].shape
        if not isinstance(shape, list) or len(shape) < 4:
            return

        h = shape[2]
        w = shape[3]
        if isinstance(h, int) and h > 0:
            self.detector_input_height = h
        if isinstance(w, int) and w > 0:
            self.detector_input_width = w
        if self.detector_input_height == self.detector_input_width:
            self.detector_input_size = self.detector_input_height

    def _run_detector_startup_check(self) -> Tuple[bool, str]:
        if self.detector_session is None or self.detector_input_name is None:
            return False, "detector session unavailable"

        try:
            sample = np.random.rand(
                1, 1, self.detector_input_height, self.detector_input_width
            ).astype(np.float32)
            outputs = self.detector_session.run(None, {self.detector_input_name: sample})
            if not outputs:
                return False, "detector returned no outputs"
            primary = np.array(outputs[0])
            shape = tuple(primary.shape)

            # Accept [1, N, C], [1, C, N], [N, C], [C, N] with C >= 6.
            if primary.ndim == 3 and primary.shape[0] == 1:
                c1, c2 = primary.shape[1], primary.shape[2]
                if c2 >= 6 or c1 >= 6:
                    return True, f"compatible shape {shape}"
                return False, f"incompatible 3D shape {shape}; expected channel dim >= 6"

            if primary.ndim == 2:
                if primary.shape[1] >= 6 or primary.shape[0] >= 6:
                    return True, f"compatible shape {shape}"
                return False, f"incompatible 2D shape {shape}; expected channel dim >= 6"

            return False, f"unsupported output rank {primary.ndim} with shape {shape}"
        except Exception as exc:
            return False, f"runtime check error: {exc}"

    @staticmethod
    def _resolve_model_path(configured_path: str) -> Optional[str]:
        if os.path.exists(configured_path):
            return configured_path

        candidates = sorted(glob("./models/*.onnx"))
        return candidates[0] if candidates else None

    def preprocess(self, image_bytes: bytes) -> np.ndarray:
        image_np = self._read_grayscale(image_bytes)
        resized = cv2.resize(image_np, (self.input_size, self.input_size))
        normalized = resized.astype(np.float32) / 255.0
        return normalized

    def preprocess_with_original(self, image_bytes: bytes) -> Tuple[np.ndarray, np.ndarray]:
        image_np = self._read_grayscale(image_bytes)
        resized = cv2.resize(image_np, (self.input_size, self.input_size))
        normalized = resized.astype(np.float32) / 255.0
        return normalized, image_np

    @staticmethod
    def _read_grayscale(image_bytes: bytes) -> np.ndarray:
        try:
            image = Image.open(BytesIO(image_bytes)).convert("L")
            return np.array(image)
        except Exception:
            dataset = pydicom.dcmread(BytesIO(image_bytes), force=True)
            pixels = dataset.pixel_array.astype(np.float32)
            pixels -= pixels.min()
            max_value = float(pixels.max())
            if max_value > 0:
                pixels /= max_value
            return (pixels * 255).astype(np.uint8)

    @staticmethod
    def _fallback_lung_mask(image_u8: np.ndarray) -> np.ndarray:
        h, w = image_u8.shape
        yy, xx = np.ogrid[:h, :w]
        cx, cy = w / 2.0, h / 2.0
        rx, ry = w * 0.42, h * 0.45
        mask = (((xx - cx) ** 2) / (rx ** 2 + 1e-6) + ((yy - cy) ** 2) / (ry ** 2 + 1e-6)) <= 1.0
        return mask.astype(np.uint8)

    def estimate_lung_mask(self, image_u8: np.ndarray) -> np.ndarray:
        blurred = cv2.GaussianBlur(image_u8, (5, 5), 0)
        inv = cv2.bitwise_not(blurred)
        _, binary = cv2.threshold(inv, 0, 255, cv2.THRESH_BINARY + cv2.THRESH_OTSU)
        kernel = np.ones((7, 7), np.uint8)
        binary = cv2.morphologyEx(binary, cv2.MORPH_OPEN, kernel)
        binary = cv2.morphologyEx(binary, cv2.MORPH_CLOSE, kernel)

        num_labels, labels, stats, centroids = cv2.connectedComponentsWithStats(binary, connectivity=8)
        h, w = image_u8.shape
        min_area = int(h * w * 0.02)
        candidate_ids: List[int] = []

        for idx in range(1, num_labels):
            area = int(stats[idx, cv2.CC_STAT_AREA])
            if area < min_area:
                continue
            cx = float(centroids[idx][0])
            cy = float(centroids[idx][1])
            if cx < w * 0.1 or cx > w * 0.9:
                continue
            if cy < h * 0.05 or cy > h * 0.95:
                continue
            candidate_ids.append(idx)

        if not candidate_ids:
            return self._fallback_lung_mask(image_u8)

        candidate_ids = sorted(
            candidate_ids,
            key=lambda idx: int(stats[idx, cv2.CC_STAT_AREA]),
            reverse=True,
        )[:2]
        mask = np.isin(labels, candidate_ids).astype(np.uint8)
        if int(mask.sum()) < int(h * w * 0.05):
            return self._fallback_lung_mask(image_u8)
        return mask

    def assess_white_lung(self, image_u8: np.ndarray, opacity_score: float) -> Dict[str, object]:
        lung_mask = self.estimate_lung_mask(image_u8).astype(bool)
        if lung_mask.sum() == 0:
            return {
                "lungOpacityRatio": 0.0,
                "whiteLungScore": float(np.clip(opacity_score * 0.3, 0.0, 1.0)),
                "severity": "none",
                "bilateralInvolvement": False,
            }

        clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8, 8))
        enhanced = clahe.apply(image_u8)
        lung_pixels = enhanced[lung_mask]
        opacity_threshold = float(max(np.percentile(lung_pixels, 84), 165.0))
        opacity_mask = (enhanced >= opacity_threshold) & lung_mask

        kernel = np.ones((3, 3), np.uint8)
        opacity_mask_u8 = cv2.morphologyEx(opacity_mask.astype(np.uint8) * 255, cv2.MORPH_OPEN, kernel)
        opacity_mask_u8 = cv2.morphologyEx(opacity_mask_u8, cv2.MORPH_CLOSE, kernel)
        opacity_mask = opacity_mask_u8 > 0

        lung_area = max(int(lung_mask.sum()), 1)
        opacity_area = int((opacity_mask & lung_mask).sum())
        ratio = float(np.clip(opacity_area / lung_area, 0.0, 1.0))

        h, w = image_u8.shape
        left_lung = lung_mask.copy()
        left_lung[:, w // 2 :] = False
        right_lung = lung_mask.copy()
        right_lung[:, : w // 2] = False
        left_ratio = float(((opacity_mask & left_lung).sum()) / max(int(left_lung.sum()), 1))
        right_ratio = float(((opacity_mask & right_lung).sum()) / max(int(right_lung.sum()), 1))
        bilateral = left_ratio >= 0.12 and right_ratio >= 0.12

        white_score = float(np.clip(0.7 * ratio + 0.3 * opacity_score, 0.0, 1.0))
        if white_score >= 0.70 or ratio >= 0.55:
            severity = "severe"
        elif white_score >= 0.50 or ratio >= 0.35:
            severity = "moderate_to_severe"
        elif white_score >= 0.28 or ratio >= 0.18:
            severity = "moderate"
        elif white_score >= 0.12:
            severity = "mild"
        else:
            severity = "none"

        return {
            "lungOpacityRatio": ratio,
            "whiteLungScore": white_score,
            "severity": severity,
            "bilateralInvolvement": bilateral,
            "leftLungOpacityRatio": float(np.clip(left_ratio, 0.0, 1.0)),
            "rightLungOpacityRatio": float(np.clip(right_ratio, 0.0, 1.0)),
            "opacityThreshold": opacity_threshold,
        }

    @staticmethod
    def build_infection_coverage(
        label_scores: Dict[str, float], white_lung_assessment: Dict[str, object]
    ) -> Dict[str, float]:
        pneumonia = float(np.clip(label_scores.get(LABEL_PNEUMONIA, 0.0), 0.0, 1.0))
        nodule = float(np.clip(label_scores.get(LABEL_NODULE, 0.0), 0.0, 1.0))
        mass = float(np.clip(label_scores.get(LABEL_MASS, 0.0), 0.0, 1.0))
        opacity = float(np.clip(label_scores.get(LABEL_OPACITY, 0.0), 0.0, 1.0))
        lesion = max(nodule, mass)
        white_score = float(np.clip(float(white_lung_assessment.get("whiteLungScore", 0.0)), 0.0, 1.0))
        ratio = float(np.clip(float(white_lung_assessment.get("lungOpacityRatio", 0.0)), 0.0, 1.0))

        return {
            "infectionAny": float(np.clip(max(pneumonia, opacity), 0.0, 1.0)),
            "viralPneumoniaLike": float(np.clip(0.65 * pneumonia + 0.25 * opacity + 0.10 * (1.0 - lesion), 0.0, 1.0)),
            "bacterialPneumoniaLike": float(np.clip(0.55 * pneumonia + 0.25 * lesion + 0.20 * opacity, 0.0, 1.0)),
            "covidLikeWhiteLungPattern": float(np.clip(0.45 * pneumonia + 0.25 * opacity + 0.30 * white_score, 0.0, 1.0)),
            "atypicalInterstitialLike": float(np.clip(0.40 * pneumonia + 0.40 * opacity + 0.20 * ratio, 0.0, 1.0)),
            "pulmonaryEdemaLike": float(np.clip(0.35 * pneumonia + 0.35 * opacity + 0.30 * white_score, 0.0, 1.0)),
            "tbLikePattern": float(np.clip(0.50 * lesion + 0.30 * opacity + 0.20 * pneumonia, 0.0, 1.0)),
        }

    @staticmethod
    def _safe_logit(prob: float) -> float:
        clipped = float(np.clip(prob, 1e-6, 1.0 - 1e-6))
        return float(np.log(clipped / (1.0 - clipped)))

    def _calibrated_sigmoid(self, raw_logit: float) -> float:
        temperature = max(self.calibration_temperature, 0.05)
        calibrated = raw_logit / temperature
        prob = 1.0 / (1.0 + np.exp(-calibrated))
        return float(np.clip(prob, 0.0, 1.0))

    def _fallback_label_scores(self, image_norm: np.ndarray) -> Dict[str, float]:
        # Deterministic fallback using image texture only (no random behavior).
        laplacian = cv2.Laplacian((image_norm * 255).astype(np.uint8), cv2.CV_64F)
        complexity = float(np.std(laplacian) / 255.0)
        high_intensity_ratio = float((image_norm > 0.82).mean())
        opacity_like = float(np.clip((complexity * 0.8 + high_intensity_ratio * 1.2), 0.01, 0.99))
        nodule = float(np.clip(0.12 + complexity * 0.65, 0.01, 0.95))
        mass = float(np.clip(0.08 + high_intensity_ratio * 1.35, 0.01, 0.95))
        pneumonia = float(np.clip(0.10 + opacity_like * 0.75, 0.01, 0.95))
        return {
            LABEL_PNEUMONIA: self._calibrated_sigmoid(self._safe_logit(pneumonia)),
            LABEL_NODULE: self._calibrated_sigmoid(self._safe_logit(nodule)),
            LABEL_MASS: self._calibrated_sigmoid(self._safe_logit(mass)),
            LABEL_OPACITY: self._calibrated_sigmoid(self._safe_logit(opacity_like)),
        }

    def infer_multitask_scores(self, image_norm: np.ndarray) -> Dict[str, float]:
        if self.session is None or self.input_name is None:
            return self._fallback_label_scores(image_norm)

        input_tensor = image_norm[np.newaxis, np.newaxis, :, :].astype(np.float32)
        outputs = self.session.run(None, {self.input_name: input_tensor})
        output = np.array(outputs[0]).reshape(-1)

        if output.size >= 4:
            # Expected ordering: [Pneumonia, Nodule, Mass, Opacity] logits.
            return {
                TASK_LABELS[idx]: self._calibrated_sigmoid(float(output[idx]))
                for idx in range(4)
            }

        if output.size >= 2:
            # Legacy binary output; map to meaningful scores conservatively.
            exp = np.exp(output - np.max(output))
            probs = exp / np.sum(exp)
            disease_prob = float(np.clip(probs[1], 1e-6, 1.0 - 1e-6))
            calibrated = self._calibrated_sigmoid(self._safe_logit(disease_prob))
            return {
                LABEL_PNEUMONIA: float(np.clip(calibrated * 0.85, 0.01, 0.99)),
                LABEL_NODULE: float(np.clip(calibrated * 0.80, 0.01, 0.99)),
                LABEL_MASS: float(np.clip(calibrated * 0.90, 0.01, 0.99)),
                LABEL_OPACITY: float(np.clip(calibrated * 0.88, 0.01, 0.99)),
            }

        if output.size == 1:
            calibrated = self._calibrated_sigmoid(float(output[0]))
            return {
                LABEL_PNEUMONIA: float(np.clip(calibrated * 0.85, 0.01, 0.99)),
                LABEL_NODULE: float(np.clip(calibrated * 0.80, 0.01, 0.99)),
                LABEL_MASS: float(np.clip(calibrated * 0.90, 0.01, 0.99)),
                LABEL_OPACITY: float(np.clip(calibrated * 0.88, 0.01, 0.99)),
            }

        return self._fallback_label_scores(image_norm)

    @staticmethod
    def _calculate_iou(box_a: Region, box_b: Region) -> float:
        ax1, ay1 = box_a.x, box_a.y
        ax2, ay2 = box_a.x + box_a.width, box_a.y + box_a.height
        bx1, by1 = box_b.x, box_b.y
        bx2, by2 = box_b.x + box_b.width, box_b.y + box_b.height

        inter_x1 = max(ax1, bx1)
        inter_y1 = max(ay1, by1)
        inter_x2 = min(ax2, bx2)
        inter_y2 = min(ay2, by2)

        inter_w = max(0.0, inter_x2 - inter_x1)
        inter_h = max(0.0, inter_y2 - inter_y1)
        inter_area = inter_w * inter_h

        area_a = box_a.width * box_a.height
        area_b = box_b.width * box_b.height
        union = area_a + area_b - inter_area
        if union <= 0:
            return 0.0
        return inter_area / union

    def _nms(self, candidates: List[Region], iou_threshold: float = 0.35) -> List[Region]:
        if not candidates:
            return []

        sorted_candidates = sorted(candidates, key=lambda region: region.confidence, reverse=True)
        kept: List[Region] = []
        while sorted_candidates:
            current = sorted_candidates.pop(0)
            kept.append(current)
            sorted_candidates = [
                candidate
                for candidate in sorted_candidates
                if self._calculate_iou(current, candidate) < iou_threshold
            ]
        return kept

    def _nms_by_label(self, regions: List[Region], iou_threshold: float) -> List[Region]:
        grouped: Dict[str, List[Region]] = {}
        for r in regions:
            grouped.setdefault(r.label, []).append(r)
        kept: List[Region] = []
        for label_regions in grouped.values():
            kept.extend(self._nms(label_regions, iou_threshold=iou_threshold))
        kept.sort(key=lambda r: r.confidence, reverse=True)
        return kept

    @staticmethod
    def _label_from_class_id(class_id: int) -> str:
        mapping = {
            0: LABEL_NODULE,
            1: LABEL_MASS,
            2: LABEL_OPACITY,
            3: LABEL_PNEUMONIA,
        }
        return mapping.get(class_id, LABEL_OPACITY)

    def _label_from_profile(self, class_id: int) -> str:
        class_map = self.detector_profile.get("class_map", {})
        if isinstance(class_map, dict):
            mapped = class_map.get(str(class_id))
            if isinstance(mapped, str) and mapped:
                return mapped
        return self._label_from_class_id(class_id)

    def _decode_detector_output(
        self,
        output: np.ndarray,
        orig_w: int,
        orig_h: int,
    ) -> List[Region]:
        regions: List[Region] = []
        out = np.array(output)
        if out.ndim == 3 and out.shape[0] == 1:
            out = out[0]

        decoder_format = str(self.detector_profile.get("decoder_format", "auto")).lower()

        # YOLO-style: [C, N] => transpose to [N, C]
        if out.ndim == 2 and out.shape[0] < out.shape[1] and out.shape[0] >= 6:
            out = out.T

        if out.ndim != 2:
            return regions

        det_w = self.detector_input_width
        det_h = self.detector_input_height
        sx = orig_w / max(det_w, 1)
        sy = orig_h / max(det_h, 1)

        for row in out:
            if row.shape[0] < 6:
                continue

            # format A: xyxy + score + class
            if decoder_format == "xyxy_cls" or (decoder_format == "auto" and row.shape[0] <= 8):
                x1, y1, x2, y2, score, cls = row[:6]
                conf = float(score)
                class_id = int(cls)
            else:
                # format B (YOLOv8): cx,cy,w,h + class_probs...
                cx, cy, bw, bh = row[:4]
                class_scores = row[4:]
                class_id = int(np.argmax(class_scores))
                conf = float(class_scores[class_id])
                x1 = cx - bw / 2.0
                y1 = cy - bh / 2.0
                x2 = cx + bw / 2.0
                y2 = cy + bh / 2.0

            if conf < self.detector_conf_threshold:
                continue

            x1 = float(np.clip(x1 * sx, 0, orig_w - 1))
            y1 = float(np.clip(y1 * sy, 0, orig_h - 1))
            x2 = float(np.clip(x2 * sx, 0, orig_w - 1))
            y2 = float(np.clip(y2 * sy, 0, orig_h - 1))
            w = max(1.0, x2 - x1)
            h = max(1.0, y2 - y1)

            regions.append(
                Region(
                    x=x1,
                    y=y1,
                    width=w,
                    height=h,
                    confidence=float(np.clip(conf, 0.0, 1.0)),
                    label=self._label_from_profile(class_id),
                )
            )

        return self._nms_by_label(regions, iou_threshold=self.detector_iou_threshold)

    def detect_regions_with_detector(
        self, image_original: np.ndarray
    ) -> Tuple[List[Region], Dict[str, float]]:
        if self.detector_session is None or self.detector_input_name is None:
            return [], {label: 0.0 for label in TASK_LABELS}

        resized = cv2.resize(image_original, (self.detector_input_width, self.detector_input_height))
        normalized = resized.astype(np.float32) / max(float(self.detector_profile.get("input_scale", 1.0)), 1e-6)
        input_channels = int(self.detector_profile.get("input_channels", 1))
        input_mean = self.detector_profile.get("input_mean", [0.0])
        input_std = self.detector_profile.get("input_std", [1.0])
        if not isinstance(input_mean, list) or not input_mean:
            input_mean = [0.0]
        if not isinstance(input_std, list) or not input_std:
            input_std = [1.0]

        if input_channels == 3:
            rgb = np.stack([normalized, normalized, normalized], axis=0).astype(np.float32)
            means = np.array((input_mean + input_mean[:1] * 3)[:3], dtype=np.float32).reshape(3, 1, 1)
            stds = np.array((input_std + input_std[:1] * 3)[:3], dtype=np.float32).reshape(3, 1, 1)
            chw = (rgb - means) / np.maximum(stds, 1e-6)
            input_tensor = chw[np.newaxis, :, :, :]
        else:
            mean = float(input_mean[0])
            std = float(input_std[0]) if float(input_std[0]) != 0 else 1.0
            chw = ((normalized - mean) / std).astype(np.float32)
            input_tensor = chw[np.newaxis, np.newaxis, :, :]

        input_tensor = input_tensor.astype(np.float32)
        outputs = self.detector_session.run(None, {self.detector_input_name: input_tensor})
        decoded = self._decode_detector_output(outputs[0], orig_w=image_original.shape[1], orig_h=image_original.shape[0])
        decoded = decoded[:10]
        scores = {label: 0.0 for label in TASK_LABELS}
        for r in decoded:
            scores[r.label] = max(scores.get(r.label, 0.0), r.confidence)
        return decoded, scores

    def detect_regions(self, image_original: np.ndarray, label_scores: Dict[str, float]) -> Tuple[List[Region], Dict[str, float]]:
        if not self.enable_heuristic_regions:
            return [], {label: 0.0 for label in TASK_LABELS}

        image_u8 = image_original.astype(np.uint8)
        clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8, 8))
        enhanced = clahe.apply(image_u8)
        threshold_value = int(np.percentile(enhanced, 92))
        _, binary = cv2.threshold(enhanced, threshold_value, 255, cv2.THRESH_BINARY)
        kernel = np.ones((5, 5), np.uint8)
        binary = cv2.morphologyEx(binary, cv2.MORPH_OPEN, kernel)
        binary = cv2.morphologyEx(binary, cv2.MORPH_CLOSE, kernel)
        contours, _ = cv2.findContours(binary, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)

        regions: List[Region] = []
        h, w = image_u8.shape
        min_area = (h * w) * 0.0008
        max_area = (h * w) * 0.18

        for cnt in contours:
            x, y, bw, bh = cv2.boundingRect(cnt)
            area = bw * bh
            if area < min_area or area > max_area:
                continue
            roi = enhanced[y : y + bh, x : x + bw]
            confidence = float(np.clip(roi.mean() / 255.0, 0.2, 0.98))
            regions.append(
                Region(
                    x=float(x),
                    y=float(y),
                    width=float(bw),
                    height=float(bh),
                    confidence=confidence,
                    label=LABEL_OPACITY,
                )
            )

        nms_regions = self._nms(regions, iou_threshold=0.35)[:5]
        region_scores = {label: 0.0 for label in TASK_LABELS}
        for region in nms_regions:
            region_scores[region.label] = max(region_scores[region.label], region.confidence)
        return nms_regions, region_scores


runner = ModelRunner()
app = FastAPI(title="Cancer Detection AI Service", version="1.1.0")
MAX_UPLOAD_BYTES = int(os.getenv("AI_MAX_UPLOAD_BYTES", str(10 * 1024 * 1024)))


@app.get("/health")
def health():
    if runner.session is None:
        runner._try_load_model()

    available_models = sorted([os.path.basename(path) for path in glob("./models/*.onnx")])
    return {
        "status": "ok",
        "modelLoaded": runner.session is not None,
        "modelVersion": runner.model_version,
        "modelPath": runner.model_path,
        "calibrationTemperature": runner.calibration_temperature,
        "thresholdHighSensitivity": runner.high_sensitivity_threshold,
        "thresholdHighSpecificity": runner.high_specificity_threshold,
        "taskThresholds": runner.task_thresholds,
        "clinicalConfigSource": runner.config_source,
        "clinicalStage": runner.clinical_stage,
        "governanceGatePassed": runner.governance_gate_passed,
        "governanceGateMessage": runner.governance_gate_message,
        "governanceCriteria": {
            "minSiteCount": runner.governance_min_site_count,
            "minAuroc": runner.governance_min_auroc,
            "minSensitivity": runner.governance_min_sensitivity,
            "minSpecificity": runner.governance_min_specificity,
        },
        "heuristicRegionsEnabled": runner.enable_heuristic_regions,
        "detectorModelLoaded": runner.detector_session is not None,
        "detectorModelPath": runner.detector_model_path,
        "detectorModelSha256": runner.detector_sha256,
        "detectorExpectedSha256": runner.detector_expected_sha256 or None,
        "detectorProfilePath": runner.detector_profile_path,
        "detectorDecoderFormat": runner.detector_profile.get("decoder_format", "auto"),
        "detectorStartupCheckPassed": runner.detector_check_passed,
        "detectorStartupCheckMessage": runner.detector_check_message,
        "tasks": ["A:Pneumonia", "B:Nodule/Mass", "C:InfectionCoverage+WhiteLung"],
        "availableModels": available_models,
        "modelError": runner.model_error,
    }


@app.post("/predict", response_model=PredictResponse)
async def predict(file: UploadFile = File(...)):
    if runner.session is None:
        runner._try_load_model()

    if not file.content_type:
        raise HTTPException(status_code=400, detail="Missing content type")

    allowed = {
        "image/png",
        "image/jpeg",
        "image/jpg",
        "image/tiff",
        "application/dicom",
        "application/octet-stream",
    }
    if file.content_type not in allowed:
        raise HTTPException(status_code=400, detail=f"Unsupported content type: {file.content_type}")

    content = await file.read()
    if not content:
        raise HTTPException(status_code=400, detail="Empty file")
    if len(content) > MAX_UPLOAD_BYTES:
        raise HTTPException(
            status_code=413,
            detail=f"File too large. Max allowed is {MAX_UPLOAD_BYTES} bytes",
        )

    try:
        image_norm, image_original = runner.preprocess_with_original(content)
        model_scores = runner.infer_multitask_scores(image_norm)
        regions, region_scores = runner.detect_regions_with_detector(image_original)
        if not regions and runner.enable_heuristic_regions:
            regions, region_scores = runner.detect_regions(image_original, model_scores)
    except Exception as exc:
        raise HTTPException(status_code=500, detail=f"Inference failed: {exc}") from exc

    merged_scores = {
        label: float(np.clip(max(model_scores.get(label, 0.0), region_scores.get(label, 0.0)), 0.0, 1.0))
        for label in TASK_LABELS
    }

    # Keep backward-compatible aggregate probability for the existing system:
    # cancerProbability reflects lesion-like risk (Nodule/Mass/Opacity), not pneumonia.
    cancer_probability = max(
        merged_scores[LABEL_NODULE],
        merged_scores[LABEL_MASS],
        merged_scores[LABEL_OPACITY],
    )

    top_findings = [
        label
        for label, score in sorted(merged_scores.items(), key=lambda item: item[1], reverse=True)
        if score >= 0.2
    ][:3]
    white_lung_assessment = runner.assess_white_lung(image_original, merged_scores[LABEL_OPACITY])
    infection_coverage = runner.build_infection_coverage(merged_scores, white_lung_assessment)
    infection_top = [
        key
        for key, score in sorted(infection_coverage.items(), key=lambda item: item[1], reverse=True)
        if score >= 0.35
    ][:2]
    top_findings = top_findings + infection_top

    pneumonia_probability = merged_scores[LABEL_PNEUMONIA]
    lesion_hs = runner.task_thresholds[TASK_LESION]["high_sensitivity_threshold"]
    lesion_hsp = runner.task_thresholds[TASK_LESION]["high_specificity_threshold"]
    pneu_hs = runner.task_thresholds[TASK_PNEUMONIA]["high_sensitivity_threshold"]
    pneu_hsp = runner.task_thresholds[TASK_PNEUMONIA]["high_specificity_threshold"]

    task_decisions = {
        TASK_LESION: {
            "highSensitivity": cancer_probability >= lesion_hs,
            "highSpecificity": cancer_probability >= lesion_hsp,
        },
        TASK_PNEUMONIA: {
            "highSensitivity": pneumonia_probability >= pneu_hs,
            "highSpecificity": pneumonia_probability >= pneu_hsp,
        },
    }
    effective_stage = runner.clinical_stage if runner.governance_gate_passed else "RESEARCH_ONLY"

    return PredictResponse(
        modelVersion=runner.model_version,
        cancerProbability=float(np.clip(cancer_probability, 0.0, 1.0)),
        regions=regions,
        labelScores=merged_scores,
        infectionCoverage=infection_coverage,
        whiteLungAssessment=white_lung_assessment,
        topFindings=top_findings,
        calibrationTemperature=runner.calibration_temperature,
        decisionHighSensitivity=task_decisions[TASK_LESION]["highSensitivity"],
        decisionHighSpecificity=task_decisions[TASK_LESION]["highSpecificity"],
        taskDecisions=task_decisions,
        operatingPointsUsed={
            TASK_LESION: {
                "highSensitivityThreshold": lesion_hs,
                "highSpecificityThreshold": lesion_hsp,
            },
            TASK_PNEUMONIA: {
                "highSensitivityThreshold": pneu_hs,
                "highSpecificityThreshold": pneu_hsp,
            },
        },
        clinicalUse=effective_stage,
        clinicalStage=effective_stage,
        detectorModelLoaded=runner.detector_session is not None,
        heatmapPath=None,
    )
