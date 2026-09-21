#!/usr/bin/env python3
"""
Importa escenas desde un CSV exportado de DynamoDB a monitoring_escenas.

Columnas del CSV:
  id, clave, band_blue, band_green, band_nir, band_red, band_rededge1,
  band_rededge2, band_rededge3, band_swir16, bbox, cloud_cover, collection,
  fecha, folio, polygon, preview_image, preview_json, preview_svg,
  procesado, renderizado, scene_created

Mapeo:
  id          → produccion_id  (extrae número de "PROD#2044" → 2044)
  clave       → clave          (sort key, sin cambios)
  bbox        → list de Decimal (deserializado desde formato DynamoDB serializado)
  polygon     → list de lists  (deserializado desde formato DynamoDB serializado)
  cloud_cover → Decimal
  procesado / renderizado → bool

Uso:
  python3 scripts/import-escenas.py --file /ruta/archivo.csv
  python3 scripts/import-escenas.py --file /ruta/archivo.csv --dry-run
"""
import csv
import json
import re
import logging
import argparse
from decimal import Decimal

import boto3

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)


# ── Deserializadores del formato serializado de DynamoDB ─────────────────────

def _deserialize_value(val):
    """Convierte un valor en formato DynamoDB serializado a Python nativo."""
    if isinstance(val, dict):
        if 'N' in val:
            return Decimal(val['N'])
        if 'S' in val:
            return val['S']
        if 'BOOL' in val:
            return val['BOOL']
        if 'L' in val:
            return [_deserialize_value(v) for v in val['L']]
        if 'M' in val:
            return {k: _deserialize_value(v) for k, v in val['M'].items()}
    return val


def deserialize_dynamo_json(raw: str):
    """Parsea un string JSON en formato DynamoDB serializado y lo convierte."""
    data = json.loads(raw)
    if isinstance(data, list):
        return [_deserialize_value(v) for v in data]
    return _deserialize_value(data)


# ── Conversiones simples ──────────────────────────────────────────────────────

def parse_produccion_id(raw: str) -> int:
    m = re.search(r'\d+', raw)
    if not m:
        raise ValueError(f"No se pudo extraer produccion_id de: {raw!r}")
    return int(m.group())


def to_decimal(val: str):
    try:
        return Decimal(val.strip())
    except Exception:
        return None


def to_bool(val: str) -> bool:
    return val.strip().lower() in ('true', '1', 'yes', 'si')


# ── Construcción del item ─────────────────────────────────────────────────────

def clean_item(row: dict) -> dict:
    item = {}

    # Partition key
    raw_id = row.get('id', '').strip()
    if not raw_id:
        raise ValueError("Campo 'id' vacío")
    item['produccion_id'] = parse_produccion_id(raw_id)

    # Sort key
    clave = row.get('clave', '').strip()
    if not clave:
        raise ValueError("Campo 'clave' vacío")
    item['clave'] = clave

    # Numérico
    if row.get('cloud_cover', '').strip():
        v = to_decimal(row['cloud_cover'])
        if v is not None:
            item['cloud_cover'] = v

    # Booleanos
    for col in ('procesado', 'renderizado'):
        if col in row and row[col].strip():
            item[col] = to_bool(row[col])

    # Strings simples
    string_cols = (
        'band_blue', 'band_green', 'band_nir', 'band_red',
        'band_rededge1', 'band_rededge2', 'band_rededge3', 'band_swir16',
        'collection', 'fecha', 'folio',
        'preview_image', 'preview_json', 'preview_svg', 'scene_created',
    )
    for col in string_cols:
        val = row.get(col, '').strip()
        if val:
            item[col] = val

    # bbox y polygon: vienen como JSON serializado de DynamoDB → deserializar
    for col in ('bbox', 'polygon'):
        raw = row.get(col, '').strip()
        if not raw:
            continue
        try:
            item[col] = deserialize_dynamo_json(raw)
        except Exception as e:
            logger.warning(f"  No se pudo deserializar {col}: {e} — se guarda como string")
            item[col] = raw

    return item


# ── Importación ───────────────────────────────────────────────────────────────

def import_escenas(file_path: str, table_name: str, dry_run: bool,
                   endpoint_url: str, region: str) -> None:
    dynamodb = boto3.resource('dynamodb', endpoint_url=endpoint_url, region_name=region)
    table = dynamodb.Table(table_name)

    items = []
    read_errors = 0

    with open(file_path, newline='', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for i, row in enumerate(reader):
            try:
                items.append((i, clean_item(row)))
            except Exception as e:
                read_errors += 1
                logger.warning(f"  Fila {i} ignorada al leer: {e}")

    logger.info(f"CSV leído: {len(items)} válidas, {read_errors} ignoradas")
    logger.info(f"Tabla: {table_name} | Región: {region} | Dry-run: {dry_run}")

    success = 0
    failed = 0
    for i, item in items:
        pk = item['produccion_id']
        sk = item['clave']
        if dry_run:
            logger.info(f"  DRY-RUN {i}: produccion_id={pk} clave={sk}")
            success += 1
            continue
        try:
            table.put_item(Item=item)
            logger.info(f"  ✓ {i}: produccion_id={pk} clave={sk}")
            success += 1
        except Exception as e:
            logger.error(f"  ✗ {i}: produccion_id={pk} clave={sk} → {e}")
            failed += 1

    logger.info(f"Resultado: {success} OK | {failed} errores | {read_errors} filas ignoradas")


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Importar escenas a DynamoDB monitoring_escenas')
    parser.add_argument('--file',         required=True,  help='Ruta al CSV')
    parser.add_argument('--table',        default='monitoring_escenas', help='Nombre de la tabla DynamoDB')
    parser.add_argument('--dry-run',      action='store_true', help='Solo muestra qué se importaría')
    parser.add_argument('--endpoint-url', default='http://localhost:4566')
    parser.add_argument('--region',       default='us-east-1')
    args = parser.parse_args()

    import_escenas(args.file, args.table, args.dry_run, args.endpoint_url, args.region)
