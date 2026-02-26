from io import BytesIO

import cv2
import numpy as np
import pydicom
from PIL import Image


class ImagePreprocessor:
    def __init__(self, input_size: int, max_image_pixels: int) -> None:
        self.input_size = input_size
        self.max_image_pixels = max_image_pixels
        Image.MAX_IMAGE_PIXELS = max_image_pixels

    def preprocess(self, image_bytes: bytes) -> np.ndarray:
        image_np = self.read_grayscale(image_bytes)
        if image_np.size > self.max_image_pixels:
            raise ValueError(
                f"image too large in pixels: {image_np.size} > {self.max_image_pixels}"
            )
        resized = cv2.resize(image_np, (self.input_size, self.input_size))
        return resized.astype(np.float32) / 255.0

    def preprocess_with_original(self, image_bytes: bytes):
        image_np = self.read_grayscale(image_bytes)
        if image_np.size > self.max_image_pixels:
            raise ValueError(
                f"image too large in pixels: {image_np.size} > {self.max_image_pixels}"
            )
        resized = cv2.resize(image_np, (self.input_size, self.input_size))
        normalized = resized.astype(np.float32) / 255.0
        return normalized, image_np

    @staticmethod
    def read_grayscale(image_bytes: bytes) -> np.ndarray:
        try:
            image = Image.open(BytesIO(image_bytes)).convert("L")
            image_np = np.array(image)
            if image_np.ndim != 2:
                raise ValueError("decoded image is not grayscale")
            return image_np
        except Exception:
            dataset = pydicom.dcmread(BytesIO(image_bytes), force=True)
            if not hasattr(dataset, "pixel_array"):
                raise ValueError("DICOM missing pixel data")
            pixels = dataset.pixel_array.astype(np.float32)
            pixels -= pixels.min()
            max_value = float(pixels.max())
            if max_value > 0:
                pixels /= max_value
            image_np = (pixels * 255).astype(np.uint8)
            if image_np.ndim != 2:
                raise ValueError("decoded DICOM is not 2D grayscale")
            return image_np
