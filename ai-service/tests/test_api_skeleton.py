import unittest

from fastapi import FastAPI

from app.api import create_app
from app.config import AppSettings


class FakeRunner:
    def __init__(self):
        self.session = object()
        self.model_version = "test-v1"
        self.model_path = "./models/fake.onnx"
        self.calibration_temperature = 1.0
        self.high_sensitivity_threshold = 0.3
        self.high_specificity_threshold = 0.7
        self.task_thresholds = {
            "lesion": {"high_sensitivity_threshold": 0.3, "high_specificity_threshold": 0.7},
            "pneumonia": {"high_sensitivity_threshold": 0.3, "high_specificity_threshold": 0.7},
        }
        self.config_source = "test"
        self.clinical_stage = "RESEARCH_ONLY"
        self.governance_gate_passed = True
        self.governance_gate_message = "ok"
        self.governance_min_site_count = 2
        self.governance_min_auroc = 0.9
        self.governance_min_sensitivity = 0.9
        self.governance_min_specificity = 0.85
        self.enable_heuristic_regions = False
        self.detector_session = None
        self.detector_model_path = ""
        self.detector_sha256 = None
        self.detector_expected_sha256 = ""
        self.detector_profile_path = ""
        self.detector_profile = {"decoder_format": "auto"}
        self.enable_tta = False
        self.enable_fp_reduction = False
        self.fp_min_lung_overlap = 0.2
        self.detector_check_passed = False
        self.detector_check_message = "disabled"
        self.model_error = None
        self.ort_available_providers = ["CPUExecutionProvider"]
        self.model_active_providers = ["CPUExecutionProvider"]
        self.detector_active_providers = []
        self.prefer_cuda_ep = False
        self.prefer_dml_ep = False
        self.enable_io_binding = False
        self.ort_cuda_device_id = 0
        self.ort_graph_optimization = "all"

    def ensure_ready(self):
        return True

    def prediction_allowed(self):
        return True, ""


class ApiSkeletonTests(unittest.TestCase):
    def _build_app(self) -> FastAPI:
        return create_app(
            runner=FakeRunner(),
            settings=AppSettings(
                max_upload_bytes=1024,
                require_api_key=False,
                api_key="",
                max_concurrent_requests=1,
                inference_timeout_seconds=1,
                max_queue_wait_seconds=0.1,
            ),
        )

    def test_create_app_returns_fastapi(self):
        app = self._build_app()
        self.assertIsInstance(app, FastAPI)

    def test_expected_routes_registered(self):
        app = self._build_app()
        paths = {route.path for route in app.routes}
        self.assertIn("/livez", paths)
        self.assertIn("/health", paths)
        self.assertIn("/metrics", paths)
        self.assertIn("/predict", paths)


if __name__ == "__main__":
    unittest.main()
