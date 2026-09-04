#!/usr/bin/env python3
"""
Test suite para DynamoDB Import Tool

Ejecutar con: python3 scripts/test-import-dynamodb.py
"""

import json
import tempfile
import sys
import os
from pathlib import Path

# Agregar scripts al path
sys.path.insert(0, str(Path(__file__).parent))

from import_dynamodb import DynamoDBImporter


def test_load_json():
    """Test: Cargar datos desde JSON"""
    print("Test 1: Cargar datos desde JSON... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    data = importer.load_data('data/dynamodb/examples/monitoring_producciones.json')
    assert len(data) == 5, f"Esperados 5 items, obtenidos {len(data)}"
    assert data[0]['produccion_id'] == 12345
    print("✓")


def test_load_csv():
    """Test: Cargar datos desde CSV"""
    print("Test 2: Cargar datos desde CSV... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    data = importer.load_data('data/dynamodb/examples/monitoring_producciones.csv')
    assert len(data) == 5, f"Esperados 5 items, obtenidos {len(data)}"
    assert data[0]['produccion_id'] == '12345'  # CSV devuelve strings
    print("✓")


def test_validate_valid_produccion():
    """Test: Validar producción válida"""
    print("Test 3: Validar producción válida... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    item = {
        'produccion_id': 12345,
        'activa': True,
        'cultivo': 'maiz',
        'ciclo': '2026-A',
        'fecha_plantacion': '2026-01-15',
        'dias_produccion': 120,
        'articulo_id': 5001,
        'centro_costo_id': 101,
        'nombre_rancho': 'Rancho Test'
    }

    valid, errors = importer.validate_item(item, 0)
    assert valid, f"Item debe ser válido: {errors}"
    print("✓")


def test_validate_invalid_ciclo():
    """Test: Rechazar ciclo inválido"""
    print("Test 4: Rechazar ciclo inválido... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    item = {
        'produccion_id': 12345,
        'activa': True,
        'cultivo': 'maiz',
        'ciclo': '2026-C',  # INVÁLIDO
        'dias_produccion': 120,
        'articulo_id': 5001,
        'centro_costo_id': 101,
    }

    valid, errors = importer.validate_item(item, 0)
    assert not valid, "Item debe ser inválido"
    assert any('ciclo' in str(e).lower() for e in errors), "Error debe mencionar ciclo"
    print("✓")


def test_validate_invalid_dias():
    """Test: Rechazar dias_produccion fuera de rango"""
    print("Test 5: Rechazar dias_produccion inválido... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    item = {
        'produccion_id': 12345,
        'activa': True,
        'cultivo': 'maiz',
        'ciclo': '2026-A',
        'dias_produccion': 300,  # > 200, INVÁLIDO
        'articulo_id': 5001,
        'centro_costo_id': 101,
    }

    valid, errors = importer.validate_item(item, 0)
    assert not valid, "Item debe ser inválido"
    assert any('dias_produccion' in str(e).lower() for e in errors)
    print("✓")


def test_validate_missing_required_field():
    """Test: Rechazar campo requerido faltante"""
    print("Test 6: Rechazar campo requerido faltante... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    item = {
        'produccion_id': 12345,
        'activa': True,
        'cultivo': 'maiz',
        # Falta 'ciclo'
        'dias_produccion': 120,
        'articulo_id': 5001,
        'centro_costo_id': 101,
    }

    valid, errors = importer.validate_item(item, 0)
    assert not valid, "Item debe ser inválido"
    assert any('ciclo' in str(e).lower() for e in errors)
    print("✓")


def test_validate_valid_escena():
    """Test: Validar escena válida"""
    print("Test 7: Validar escena válida... ", end="")

    importer = DynamoDBImporter(
        'monitoring_escenas',
        dry_run=True
    )

    item = {
        'scene_id': 'S2A_MSIL2A_20260205T135051_N0510_R024_T19HCC_20260205T135101',
        'produccion_id': 12345,
        'date': '2026-02-05',
        'cloud_cover': 12.5,
        'stac_assets': {
            'B04': {
                'href': 'https://example.com/B04.jp2',
                'resolution': 10
            }
        }
    }

    valid, errors = importer.validate_item(item, 0)
    assert valid, f"Item debe ser válido: {errors}"
    print("✓")


def test_validate_cloud_cover_range():
    """Test: Validar rango de cloud_cover"""
    print("Test 8: Validar rango de cloud_cover... ", end="")

    importer = DynamoDBImporter(
        'monitoring_escenas',
        dry_run=True
    )

    item = {
        'scene_id': 'S2A_MSIL2A_20260205T135051_N0510_R024_T19HCC_20260205T135101',
        'produccion_id': 12345,
        'date': '2026-02-05',
        'cloud_cover': 150.0,  # > 100, INVÁLIDO
    }

    valid, errors = importer.validate_item(item, 0)
    assert not valid, "Item debe ser inválido"
    assert any('cloud_cover' in str(e).lower() for e in errors)
    print("✓")


def test_convert_types():
    """Test: Convertir tipos de datos"""
    print("Test 9: Convertir tipos de datos... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    item = {
        'produccion_id': '12345',  # string -> int
        'activa': 'true',          # string -> bool
        'cultivo': 'maiz',
        'ciclo': '2026-A',
        'dias_produccion': '120',  # string -> int
        'articulo_id': '5001',
        'centro_costo_id': '101',
    }

    converted = importer.convert_types(item)
    assert isinstance(converted['produccion_id'], int)
    assert isinstance(converted['activa'], bool)
    assert isinstance(converted['dias_produccion'], int)
    assert converted['produccion_id'] == 12345
    assert converted['activa'] is True
    print("✓")


def test_schema_compatibility():
    """Test: Esquemas de tabla conocidas"""
    print("Test 10: Esquemas de tablas... ", end="")

    # Verificar que las tablas existen en el importer
    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    assert 'monitoring_producciones' in importer.TABLE_SCHEMAS
    assert 'monitoring_escenas' in importer.TABLE_SCHEMAS

    schema = importer.TABLE_SCHEMAS['monitoring_producciones']
    assert 'required_fields' in schema
    assert 'optional_fields' in schema
    assert 'field_types' in schema
    assert 'partition_key' in schema

    print("✓")


def test_date_validation():
    """Test: Validación de fechas"""
    print("Test 11: Validación de fechas... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    # Fechas válidas
    assert importer._is_valid_date('2026-01-15')
    assert importer._is_valid_date('2000-12-31')

    # Fechas inválidas
    assert not importer._is_valid_date('2026-13-01')
    assert not importer._is_valid_date('2026-01-32')
    assert not importer._is_valid_date('2026/01/15')
    assert not importer._is_valid_date('15-01-2026')

    print("✓")


def test_ciclo_validation():
    """Test: Validación de ciclos"""
    print("Test 12: Validación de ciclos... ", end="")

    importer = DynamoDBImporter(
        'monitoring_producciones',
        dry_run=True
    )

    # Ciclos válidos
    assert importer._is_valid_ciclo('2026-A')
    assert importer._is_valid_ciclo('2025-B')
    assert importer._is_valid_ciclo('2100-A')

    # Ciclos inválidos
    assert not importer._is_valid_ciclo('2026-C')
    assert not importer._is_valid_ciclo('2026')
    assert not importer._is_valid_ciclo('A-2026')
    assert not importer._is_valid_ciclo('2026-AB')

    print("✓")


def test_invalid_table_name():
    """Test: Rechazar tabla desconocida"""
    print("Test 13: Rechazar tabla desconocida... ", end="")

    try:
        importer = DynamoDBImporter('tabla_inexistente', dry_run=True)
        assert False, "Debe lanzar ValueError"
    except ValueError as e:
        assert 'tabla desconocida' in str(e).lower()
    print("✓")


def run_all_tests():
    """Ejecutar todos los tests"""
    print("\n" + "=" * 60)
    print("DynamoDB Import Tool - Test Suite")
    print("=" * 60 + "\n")

    tests = [
        test_load_json,
        test_load_csv,
        test_validate_valid_produccion,
        test_validate_invalid_ciclo,
        test_validate_invalid_dias,
        test_validate_missing_required_field,
        test_validate_valid_escena,
        test_validate_cloud_cover_range,
        test_convert_types,
        test_schema_compatibility,
        test_date_validation,
        test_ciclo_validation,
        test_invalid_table_name,
    ]

    passed = 0
    failed = 0

    for test in tests:
        try:
            test()
            passed += 1
        except Exception as e:
            print(f"✗ ERROR: {e}")
            failed += 1

    print("\n" + "=" * 60)
    print(f"Resultados: {passed} pasados, {failed} fallidos")
    print("=" * 60)

    return 0 if failed == 0 else 1


if __name__ == '__main__':
    sys.exit(run_all_tests())
