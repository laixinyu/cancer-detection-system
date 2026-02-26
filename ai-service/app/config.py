import os
from dataclasses import dataclass


def _parse_bool(name: str, default: bool) -> bool:
    raw = os.getenv(name)
    if raw is None:
        return default
    value = raw.strip().lower()
    if value in {"1", "true", "yes", "on"}:
        return True
    if value in {"0", "false", "no", "off"}:
        return False
    raise ValueError(f"Invalid boolean for {name}: {raw}")


def _parse_int(name: str, default: int, minimum: int | None = None) -> int:
    raw = os.getenv(name)
    value = default if raw is None else int(raw.strip())
    if minimum is not None and value < minimum:
        raise ValueError(f"{name} must be >= {minimum}, got {value}")
    return value


def _parse_float(name: str, default: float, minimum: float | None = None) -> float:
    raw = os.getenv(name)
    value = default if raw is None else float(raw.strip())
    if minimum is not None and value < minimum:
        raise ValueError(f"{name} must be >= {minimum}, got {value}")
    return value


@dataclass(frozen=True)
class AppSettings:
    max_upload_bytes: int
    require_api_key: bool
    api_key: str
    max_concurrent_requests: int
    inference_timeout_seconds: float
    max_queue_wait_seconds: float


@dataclass(frozen=True)
class ModelSettings:
    model_path: str
    model_version: str
    input_size: int
    calibration_temperature: float
    high_sensitivity_threshold: float
    high_specificity_threshold: float
    clinical_config_path: str
    enable_heuristic_regions: bool
    detector_model_path: str
    detector_input_size: int
    detector_conf_threshold: float
    detector_iou_threshold: float
    enable_tta: bool
    enable_fp_reduction: bool
    fp_min_lung_overlap: float
    prefer_cuda_ep: bool
    prefer_dml_ep: bool
    enable_io_binding: bool
    ort_cuda_device_id: int
    ort_intra_threads: int
    ort_inter_threads: int
    ort_graph_optimization: str
    detector_profile_path: str
    detector_expected_sha256: str
    enforce_detector_startup_check: bool
    enforce_governance_gate: bool
    clinical_governance_path: str
    governance_min_site_count: int
    governance_min_auroc: float
    governance_min_sensitivity: float
    governance_min_specificity: float
    max_image_pixels: int



def load_settings() -> AppSettings:
    return AppSettings(
        max_upload_bytes=_parse_int("AI_MAX_UPLOAD_BYTES", 10 * 1024 * 1024, minimum=1),
        require_api_key=_parse_bool("AI_REQUIRE_API_KEY", False),
        api_key=os.getenv("AI_API_KEY", "").strip(),
        max_concurrent_requests=_parse_int("AI_MAX_CONCURRENT_REQUESTS", 2, minimum=1),
        inference_timeout_seconds=_parse_float("AI_INFERENCE_TIMEOUT_SECONDS", 45.0, minimum=0.1),
        max_queue_wait_seconds=_parse_float("AI_MAX_QUEUE_WAIT_SECONDS", 2.0, minimum=0.0),
    )



def load_model_settings() -> ModelSettings:
    graph_level = os.getenv("AI_ORT_GRAPH_OPT_LEVEL", "all").strip().lower()
    if graph_level not in {"disable", "basic", "extended", "all"}:
        raise ValueError(f"AI_ORT_GRAPH_OPT_LEVEL must be one of disable/basic/extended/all, got {graph_level}")

    return ModelSettings(
        model_path=os.getenv("AI_MODEL_PATH", "./models/cxr_multitask.onnx"),
        model_version=os.getenv("AI_MODEL_VERSION", "cxr-multitask-v1"),
        input_size=_parse_int("AI_INPUT_SIZE", 224, minimum=16),
        calibration_temperature=_parse_float("AI_CALIBRATION_TEMPERATURE", 1.6, minimum=0.01),
        high_sensitivity_threshold=_parse_float("AI_THRESHOLD_HIGH_SENS", 0.30, minimum=0.0),
        high_specificity_threshold=_parse_float("AI_THRESHOLD_HIGH_SPEC", 0.70, minimum=0.0),
        clinical_config_path=os.getenv("AI_CLINICAL_CONFIG_PATH", "./models/clinical_config.json"),
        enable_heuristic_regions=_parse_bool("AI_ENABLE_HEURISTIC_REGIONS", False),
        detector_model_path=os.getenv("AI_DETECTOR_MODEL_PATH", "./models/cxr_detector.onnx"),
        detector_input_size=_parse_int("AI_DETECTOR_INPUT_SIZE", 640, minimum=32),
        detector_conf_threshold=_parse_float("AI_DETECTOR_CONF_THRESHOLD", 0.25, minimum=0.0),
        detector_iou_threshold=_parse_float("AI_DETECTOR_IOU_THRESHOLD", 0.45, minimum=0.0),
        enable_tta=_parse_bool("AI_ENABLE_TTA", True),
        enable_fp_reduction=_parse_bool("AI_ENABLE_FP_REDUCTION", True),
        fp_min_lung_overlap=_parse_float("AI_FP_MIN_LUNG_OVERLAP", 0.20, minimum=0.0),
        prefer_cuda_ep=_parse_bool("AI_ORT_PREFER_CUDA", True),
        prefer_dml_ep=_parse_bool("AI_ORT_PREFER_DIRECTML", True),
        enable_io_binding=_parse_bool("AI_ORT_ENABLE_IO_BINDING", True),
        ort_cuda_device_id=_parse_int("AI_ORT_CUDA_DEVICE_ID", 0, minimum=0),
        ort_intra_threads=_parse_int("AI_ORT_INTRA_OP_THREADS", 1, minimum=1),
        ort_inter_threads=_parse_int("AI_ORT_INTER_OP_THREADS", 1, minimum=1),
        ort_graph_optimization=graph_level,
        detector_profile_path=os.getenv("AI_DETECTOR_PROFILE_PATH", "./models/detector_profile.json"),
        detector_expected_sha256=os.getenv("AI_DETECTOR_SHA256", "").strip().lower(),
        enforce_detector_startup_check=_parse_bool("AI_ENFORCE_DETECTOR_STARTUP_CHECK", True),
        enforce_governance_gate=_parse_bool("AI_ENFORCE_GOVERNANCE_GATE", True),
        clinical_governance_path=os.getenv("AI_CLINICAL_GOVERNANCE_PATH", "./models/clinical_governance.json"),
        governance_min_site_count=_parse_int("AI_GOV_MIN_SITE_COUNT", 2, minimum=1),
        governance_min_auroc=_parse_float("AI_GOV_MIN_AUROC", 0.90, minimum=0.0),
        governance_min_sensitivity=_parse_float("AI_GOV_MIN_SENSITIVITY", 0.90, minimum=0.0),
        governance_min_specificity=_parse_float("AI_GOV_MIN_SPECIFICITY", 0.85, minimum=0.0),
        max_image_pixels=_parse_int("AI_MAX_IMAGE_PIXELS", 16 * 1024 * 1024, minimum=1),
    )
