"""Run with python -m unittest discover -s scripts -p test_raster_quality.py."""
import importlib.util
from pathlib import Path
import unittest
import numpy as np

spec = importlib.util.spec_from_file_location("raster_quality", Path(__file__).resolve().parents[1] / "internal/processing/raster_quality.py")
quality = importlib.util.module_from_spec(spec)
spec.loader.exec_module(quality)


class Band:
    def __init__(self, data, mask=None, nodata=None):
        self.data, self.mask, self.nodata = data, mask, nodata
    def ReadAsArray(self):
        return self.data
    def GetMaskBand(self):
        return Band(self.mask if self.mask is not None else np.full(self.data.shape, 255))
    def GetNoDataValue(self):
        return self.nodata


class Raster:
    def __init__(self, bands):
        self.bands = bands
        self.RasterCount = len(bands)
        self.RasterYSize, self.RasterXSize = bands[0].data.shape
    def GetRasterBand(self, n):
        return self.bands[n-1]


class QualityTests(unittest.TestCase):
    def test_outside_polygon_does_not_count_as_missing(self):
        valid = np.zeros((10, 10), bool)
        valid[2:4, 3:8] = True
        inside = valid.copy()
        result = quality.measure_quality(valid, inside, np.full((10,10), 4))
        self.assertEqual(result['total_pixels'], 10)
        self.assertEqual(result['nodata_pct'], 0)
        self.assertEqual(result['vegetacion_pct'], 100)

    def test_partial_coverage_is_not_cloud(self):
        valid = np.ones((10,10), bool)
        valid[:, :2] = False
        result = quality.measure_quality(valid, np.ones_like(valid), np.full((10,10), 4))
        self.assertEqual(result['nodata_pct'], 20)
        self.assertEqual(result['nube_pct'], 0)
        self.assertEqual(result['valid_pct'], 80)

    def test_all_missing_and_legacy(self):
        result = quality.measure_quality(np.zeros((2,2), bool), np.ones((2,2), bool))
        self.assertEqual(result['nodata_pct'], 100)
        self.assertFalse(result['has_scl'])

    def test_validity_combines_scl_zero_mask_and_nodata(self):
        bands = [Band(np.full((2,3), 500.0)) for _ in range(10)]
        bands[0].data[0,0] = 0
        bands[1].mask = np.array([[255,0,255], [255,255,255]])
        bands[2].nodata = -9999
        bands[2].data[0,2] = -9999
        bands[3].data[1,0] = np.nan
        scl = np.array([[4,4,4], [4,0,4]])
        bands.append(Band(scl))
        valid = quality.valid_pixels(Raster(bands))
        np.testing.assert_array_equal(valid, [[False,False,False], [False,False,True]])

    def test_dark_but_nonzero_is_not_nodata(self):
        raster = Raster([Band(np.ones((2,2))) for _ in range(10)])
        self.assertTrue(quality.valid_pixels(raster).all())

    def test_empty_polygon_is_error(self):
        with self.assertRaises(ValueError):
            quality.measure_quality(np.ones((2,2), bool), np.zeros((2,2), bool))


if __name__ == '__main__':
    unittest.main()
