import unittest

import numpy as np

from app.model_runtime import LABEL_MASS, LABEL_NODULE, LABEL_OPACITY, LABEL_PNEUMONIA
from app.postprocess import InferencePostprocessor


class PostprocessUnitTests(unittest.TestCase):
    def _labels(self):
        return {
            "pneumonia": LABEL_PNEUMONIA,
            "nodule": LABEL_NODULE,
            "mass": LABEL_MASS,
            "opacity": LABEL_OPACITY,
        }

    def _pp(self):
        return InferencePostprocessor(
            detector_input_width=640,
            detector_input_height=640,
            detector_conf_threshold=0.25,
            detector_iou_threshold=0.45,
            fp_min_lung_overlap=0.2,
            enable_fp_reduction=True,
            enable_heuristic_regions=False,
            detector_profile={"decoder_format": "auto", "class_map": {"0": LABEL_NODULE}},
            labels=self._labels(),
        )

    def test_build_infection_coverage_keys(self):
        out = InferencePostprocessor.build_infection_coverage(
            {
                LABEL_PNEUMONIA: 0.5,
                LABEL_NODULE: 0.2,
                LABEL_MASS: 0.3,
                LABEL_OPACITY: 0.4,
            },
            {"whiteLungScore": 0.1, "lungOpacityRatio": 0.2},
            labels=self._labels(),
        )
        self.assertIn("infectionAny", out)
        self.assertIn("tbLikePattern", out)

    def test_assess_white_lung_structure(self):
        pp = self._pp()
        img = np.full((128, 128), 120, dtype=np.uint8)
        out = pp.assess_white_lung(img, 0.3)
        self.assertIn("whiteLungScore", out)
        self.assertIn("severity", out)


if __name__ == "__main__":
    unittest.main()
