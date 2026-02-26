import unittest

import numpy as np
from PIL import Image

from app.preprocess import ImagePreprocessor


class PreprocessUnitTests(unittest.TestCase):
    def test_preprocess_with_original_shape(self):
        pre = ImagePreprocessor(input_size=224, max_image_pixels=224 * 224 * 4)
        arr = np.full((64, 64), 128, dtype=np.uint8)
        im = Image.fromarray(arr, mode="L")
        import io

        buf = io.BytesIO()
        im.save(buf, format="PNG")

        norm, orig = pre.preprocess_with_original(buf.getvalue())
        self.assertEqual(orig.shape, (64, 64))
        self.assertEqual(norm.shape, (224, 224))
        self.assertGreaterEqual(float(norm.min()), 0.0)
        self.assertLessEqual(float(norm.max()), 1.0)

    def test_reject_too_many_pixels(self):
        pre = ImagePreprocessor(input_size=32, max_image_pixels=32 * 32)
        arr = np.zeros((128, 128), dtype=np.uint8)
        im = Image.fromarray(arr, mode="L")
        import io

        buf = io.BytesIO()
        im.save(buf, format="PNG")
        with self.assertRaises(ValueError):
            pre.preprocess_with_original(buf.getvalue())


if __name__ == "__main__":
    unittest.main()
