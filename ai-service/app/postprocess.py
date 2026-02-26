from typing import Dict, List, Optional, Tuple

import cv2
import numpy as np


class InferencePostprocessor:
    def __init__(
        self,
        detector_input_width: int,
        detector_input_height: int,
        detector_conf_threshold: float,
        detector_iou_threshold: float,
        fp_min_lung_overlap: float,
        enable_fp_reduction: bool,
        enable_heuristic_regions: bool,
        detector_profile: Dict[str, object],
        labels: Dict[str, str],
    ) -> None:
        self.detector_input_width = detector_input_width
        self.detector_input_height = detector_input_height
        self.detector_conf_threshold = detector_conf_threshold
        self.detector_iou_threshold = detector_iou_threshold
        self.fp_min_lung_overlap = fp_min_lung_overlap
        self.enable_fp_reduction = enable_fp_reduction
        self.enable_heuristic_regions = enable_heuristic_regions
        self.detector_profile = detector_profile
        self.labels = labels

    def update_runtime(
        self,
        detector_input_width: int,
        detector_input_height: int,
        detector_profile: Dict[str, object],
    ) -> None:
        self.detector_input_width = detector_input_width
        self.detector_input_height = detector_input_height
        self.detector_profile = detector_profile

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
        label_scores: Dict[str, float], white_lung_assessment: Dict[str, object], labels: Dict[str, str]
    ) -> Dict[str, float]:
        pneumonia = float(np.clip(label_scores.get(labels["pneumonia"], 0.0), 0.0, 1.0))
        nodule = float(np.clip(label_scores.get(labels["nodule"], 0.0), 0.0, 1.0))
        mass = float(np.clip(label_scores.get(labels["mass"], 0.0), 0.0, 1.0))
        opacity = float(np.clip(label_scores.get(labels["opacity"], 0.0), 0.0, 1.0))
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
    def apply_low_evidence_guard(
        label_scores: Dict[str, float],
        regions: List[object],
        white_lung_assessment: Dict[str, object],
        labels: Dict[str, str],
    ) -> Tuple[Dict[str, float], bool]:
        max_region_conf = max((float(getattr(r, "confidence", 0.0)) for r in regions), default=0.0)
        white_score = float(np.clip(float(white_lung_assessment.get("whiteLungScore", 0.0)), 0.0, 1.0))
        opacity_ratio = float(np.clip(float(white_lung_assessment.get("lungOpacityRatio", 0.0)), 0.0, 1.0))

        low_evidence = max_region_conf < 0.35 and white_score < 0.20 and opacity_ratio < 0.15
        if not low_evidence:
            return label_scores, False

        damped = dict(label_scores)
        damped[labels["nodule"]] = float(np.clip(damped.get(labels["nodule"], 0.0) * 0.72, 0.0, 1.0))
        damped[labels["mass"]] = float(np.clip(damped.get(labels["mass"], 0.0) * 0.72, 0.0, 1.0))
        damped[labels["opacity"]] = float(np.clip(damped.get(labels["opacity"], 0.0) * 0.85, 0.0, 1.0))
        return damped, True
