#!/usr/bin/env python3
"""
Dibuja el contorno del polígono de una producción sobre un PNG georeferenciado.

Uso:
  overlay_polygon.py <png_path> <geojson_path> <output_path> [r g b thickness]

Argumentos:
  png_path     - PNG de entrada (con georeferencia embebida, CRS en UTM)
  geojson_path - GeoJSON del polígono en WGS84 (escrito por WritePolygonGeoJSON)
  output_path  - PNG de salida con el contorno dibujado
  r g b        - Color del contorno (0-255), por defecto 255 255 0 (amarillo)
  thickness    - Grosor del contorno en píxeles, por defecto 3
"""
import sys
import json
import numpy as np
from osgeo import gdal, osr

gdal.UseExceptions()


def bresenham_line(x0, y0, x1, y1):
    """Genera los píxeles de una línea entre dos puntos (Bresenham)."""
    pts = []
    dx = abs(x1 - x0)
    dy = abs(y1 - y0)
    sx = 1 if x0 < x1 else -1
    sy = 1 if y0 < y1 else -1
    err = dx - dy
    while True:
        pts.append((x0, y0))
        if x0 == x1 and y0 == y1:
            break
        e2 = 2 * err
        if e2 > -dy:
            err -= dy
            x0 += sx
        if e2 < dx:
            err += dx
            y0 += sy
    return pts


def dilate_mask(mask, radius):
    """Expande la máscara booleana con un kernel cuadrado de radio dado."""
    if radius <= 0:
        return mask
    result = np.zeros_like(mask)
    for dr in range(-radius, radius + 1):
        for dc in range(-radius, radius + 1):
            shifted = np.roll(np.roll(mask, dr, axis=0), dc, axis=1)
            # Eliminar artefactos de roll en los bordes
            if dr > 0:
                shifted[:dr, :] = False
            elif dr < 0:
                shifted[dr:, :] = False
            if dc > 0:
                shifted[:, :dc] = False
            elif dc < 0:
                shifted[:, dc:] = False
            result |= shifted
    return result


def main():
    if len(sys.argv) < 4:
        print(__doc__)
        sys.exit(1)

    png_path = sys.argv[1]
    geojson_path = sys.argv[2]
    output_path = sys.argv[3]
    r_color = int(sys.argv[4]) if len(sys.argv) > 4 else 255
    g_color = int(sys.argv[5]) if len(sys.argv) > 5 else 255
    b_color = int(sys.argv[6]) if len(sys.argv) > 6 else 0
    thickness = int(sys.argv[7]) if len(sys.argv) > 7 else 3

    # Abrir imagen
    ds = gdal.Open(png_path)
    if ds is None:
        print(f"Error: no se pudo abrir {png_path}", file=sys.stderr)
        sys.exit(1)

    gt = ds.GetGeoTransform()   # (originX, pixelW, 0, originY, 0, pixelH)
    proj = ds.GetProjection()
    W = ds.RasterXSize
    H = ds.RasterYSize
    n_bands = ds.RasterCount

    img = np.stack([ds.GetRasterBand(i + 1).ReadAsArray() for i in range(n_bands)])
    ds = None

    # Leer polígono GeoJSON (coords en WGS84)
    with open(geojson_path) as f:
        fc = json.load(f)

    ring = fc['features'][0]['geometry']['coordinates'][0]  # [[lon, lat], ...]

    # Transformación WGS84 → SRS de la imagen (UTM)
    src_srs = osr.SpatialReference()
    src_srs.ImportFromEPSG(4326)
    src_srs.SetAxisMappingStrategy(osr.OAMS_TRADITIONAL_GIS_ORDER)

    dst_srs = osr.SpatialReference()
    dst_srs.ImportFromWkt(proj)
    dst_srs.SetAxisMappingStrategy(osr.OAMS_TRADITIONAL_GIS_ORDER)

    ct = osr.CoordinateTransformation(src_srs, dst_srs)

    # Convertir vértices del polígono a coordenadas de píxel
    pixel_pts = []
    for lon, lat in ring:
        x, y, _ = ct.TransformPoint(lon, lat)
        col = int(round((x - gt[0]) / gt[1]))
        row = int(round((y - gt[3]) / gt[5]))
        pixel_pts.append((col, row))

    # Dibujar contorno con líneas de Bresenham
    outline = np.zeros((H, W), dtype=bool)
    for i in range(len(pixel_pts) - 1):
        x0, y0 = pixel_pts[i]
        x1, y1 = pixel_pts[i + 1]
        for px, py in bresenham_line(x0, y0, x1, y1):
            if 0 <= py < H and 0 <= px < W:
                outline[py, px] = True

    # Engrosar el contorno
    outline = dilate_mask(outline, thickness // 2)

    # Aplicar color al contorno
    if n_bands >= 1:
        img[0][outline] = r_color
    if n_bands >= 2:
        img[1][outline] = g_color
    if n_bands >= 3:
        img[2][outline] = b_color
    if n_bands >= 4:
        img[3][outline] = 255  # opacidad total

    # PNG driver no soporta Create(); escribir primero a MEM y luego CreateCopy → PNG.
    mem_driver = gdal.GetDriverByName('MEM')
    mem_ds = mem_driver.Create('', W, H, n_bands, gdal.GDT_Byte)
    mem_ds.SetGeoTransform(gt)
    mem_ds.SetProjection(proj)
    for i in range(n_bands):
        mem_ds.GetRasterBand(i + 1).WriteArray(img[i])

    png_driver = gdal.GetDriverByName('PNG')
    png_driver.CreateCopy(output_path, mem_ds, strict=0)
    mem_ds = None


if __name__ == '__main__':
    main()
