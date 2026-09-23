"""Quality and common NoData mask on the actual raster, never on its bbox alone."""
import json
import sys

import numpy as np


def valid_pixels(ds):
    if ds.RasterCount < 10:
        raise ValueError("multiband must contain all 10 spectral bands")
    valid = np.ones((ds.RasterYSize, ds.RasterXSize), dtype=bool)
    for i in range(1, 11):
        band = ds.GetRasterBand(i)
        data = band.ReadAsArray()
        valid &= np.isfinite(data) & (band.GetMaskBand().ReadAsArray() != 0)
        # Sentinel-2 DN zero is reserved for NoData, including legacy TIFFs
        # that lost their NoData metadata.
        valid &= data != 0
        nd = band.GetNoDataValue()
        if nd is not None:
            valid &= data != nd
    if ds.RasterCount >= 11:
        scl = ds.GetRasterBand(11)
        valid &= (scl.ReadAsArray() != 0) & (scl.GetMaskBand().ReadAsArray() != 0)
    return valid


def polygon_mask(ds, path):
    from osgeo import gdal, ogr
    if not path:
        raise ValueError("production polygon is required for coverage evaluation")
    vector = ogr.Open(path)
    if vector is None:
        raise ValueError("cannot open production polygon")
    mask = gdal.GetDriverByName("MEM").Create("", ds.RasterXSize, ds.RasterYSize, 1, gdal.GDT_Byte)
    mask.SetGeoTransform(ds.GetGeoTransform())
    mask.SetProjection(ds.GetProjection())
    mask.GetRasterBand(1).Fill(0)
    # Pixel centers, same grid as the spectral bands. Outside-polygon pixels
    # are never included in the denominator, even when they are NoData.
    gdal.RasterizeLayer(mask, [1], vector.GetLayer(), burn_values=[1])
    inside = mask.ReadAsArray() == 1
    if not inside.any():
        raise ValueError("production polygon has no pixel centers in this raster")
    return inside


def measure_quality(valid, inside, scl=None):
    total = int(inside.sum())
    if total == 0:
        raise ValueError("production polygon has no pixels")
    observed = int((inside & valid).sum())
    nodata = 100.0 * (total - observed) / total
    has_scl = scl is not None
    result = {
        "nodata_pct": nodata, "valid_pct": 100.0 - nodata,
        "total_pixels": total, "valid_pixels": observed,
        "has_scl": has_scl, "source": "scl_and_masks" if has_scl else "spectral_masks",
        "vegetacion_pct": 0, "suelo_pct": 0, "agua_pct": 0, "nube_pct": 0,
    }
    if has_scl and observed:
        scl = scl[inside & valid]
        for key, classes in (("vegetacion_pct", [4]), ("suelo_pct", [5]),
                             ("agua_pct", [6]), ("nube_pct", [3, 8, 9, 10])):
            result[key] = 100.0 * float(np.isin(scl, classes).sum()) / observed
    return result


def prepare(source, polygon, destination):
    from osgeo import gdal
    gdal.UseExceptions()
    ds = gdal.Open(source)
    valid = valid_pixels(ds)
    inside = polygon_mask(ds, polygon)
    scl = ds.GetRasterBand(11).ReadAsArray() if ds.RasterCount >= 11 else None
    result = measure_quality(valid, inside, scl)
    # Work copy only: retain the original reusable raster and its SCL.
    out = gdal.GetDriverByName("GTiff").CreateCopy(destination, ds)
    for i in range(1, 11):
        band = out.GetRasterBand(i)
        data = band.ReadAsArray()
        data[~valid] = 0
        band.WriteArray(data)
        band.SetNoDataValue(0)
        # Do not reuse cached statistics from the unmasked raster.
        band.SetMetadata({k: v for k, v in band.GetMetadata().items() if not k.startswith("STATISTICS_")})
    out = None
    print(json.dumps(result, allow_nan=False))


def alpha(source, png):
    from osgeo import gdal
    gdal.UseExceptions()
    ds = gdal.Open(source)
    valid = valid_pixels(ds)
    img = gdal.Open(png)
    w, h = img.RasterXSize, img.RasterYSize
    out = gdal.GetDriverByName("MEM").Create("", w, h, 4, gdal.GDT_Byte)
    out.SetGeoTransform(img.GetGeoTransform())
    out.SetProjection(img.GetProjection())
    for i in range(1, 4):
        out.GetRasterBand(i).WriteArray(img.GetRasterBand(i).ReadAsArray())
        out.GetRasterBand(i).SetColorInterpretation((gdal.GCI_RedBand, gdal.GCI_GreenBand, gdal.GCI_BlueBand)[i-1])
    rows = np.minimum((np.arange(h) * valid.shape[0] / h).astype(int), valid.shape[0]-1)
    cols = np.minimum((np.arange(w) * valid.shape[1] / w).astype(int), valid.shape[1]-1)
    opacity = (valid[np.ix_(rows, cols)] * 255).astype(np.uint8)
    if img.RasterCount >= 4:
        opacity = np.minimum(opacity, img.GetRasterBand(4).ReadAsArray())
    out.GetRasterBand(4).WriteArray(opacity)
    out.GetRasterBand(4).SetColorInterpretation(gdal.GCI_AlphaBand)
    img = None
    gdal.GetDriverByName("PNG").CreateCopy(png, out)


if __name__ == "__main__":
    if sys.argv[1] == "prepare":
        prepare(*sys.argv[2:])
    elif sys.argv[1] == "alpha":
        alpha(*sys.argv[2:])
    else:
        raise ValueError("unknown operation")
