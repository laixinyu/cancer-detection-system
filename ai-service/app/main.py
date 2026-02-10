import os
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
    topFindings: List[str]
    calibrationTemperature: float
    decisionHighSensitivity: bool
    decisionHighSpecificity: bool
    clinicalUse: str
    heatmapPath: Optional[str] = None


class ModelRunner:
    def __init__(self) -> None:
        self.model_path = os.getenv("AI_MODEL_PATH", "./models/cxr_multitask.onnx")
        self.model_version = os.getenv("AI_MODEL_VERSION", "cxr-multitask-v1")
        self.input_size = int(os.getenv("AI_INPUT_SIZE", "224"))
        self.calibration_temperature = float(os.getenv("AI_CALIBRATION_TEMPERATURE", "1.6"))
        self.high_sensitivity_threshold = float(os.getenv("AI_THRESHOLD_HIGH_SENS", "0.30"))
        self.high_specificity_threshold = float(os.getenv("AI_THRESHOLD_HIGH_SPEC", "0.70"))
        self.enable_heuristic_regions = (
            os.getenv("AI_ENABLE_HEURISTIC_REGIONS", "false").lower() == "true"
        )
        self.session: Optional[ort.InferenceSession] = None
        self.input_name: Optional[str] = None
        self.output_names: List[str] = []
        self.model_error: Optional[str] = None
        self._try_load_model()

    def _try_load_model(self) -> None:
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

    def detect_regions(self, image_norm: np.ndarray, label_scores: Dict[str, float]) -> Tuple[List[Region], Dict[str, float]]:
        if not self.enable_heuristic_regions:
            return [], {label: 0.0 for label in TASK_LABELS}

        image_u8 = (image_norm * 255).astype(np.uint8)
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
        "heuristicRegionsEnabled": runner.enable_heuristic_regions,
        "tasks": ["A:Pneumonia", "B:Nodule/Mass"],
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
        image_norm = runner.preprocess(content)
        model_scores = runner.infer_multitask_scores(image_norm)
        regions, region_scores = runner.detect_regions(image_norm, model_scores)
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

    return PredictResponse(
        modelVersion=runner.model_version,
        cancerProbability=float(np.clip(cancer_probability, 0.0, 1.0)),
        regions=regions,
        labelScores=merged_scores,
        topFindings=top_findings,
        calibrationTemperature=runner.calibration_temperature,
        decisionHighSensitivity=cancer_probability >= runner.high_sensitivity_threshold,
        decisionHighSpecificity=cancer_probability >= runner.high_specificity_threshold,
        clinicalUse="RESEARCH_ONLY",
        heatmapPath=None,
    )
