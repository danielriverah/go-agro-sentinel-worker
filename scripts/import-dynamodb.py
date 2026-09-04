#!/usr/bin/env python3
"""
DynamoDB Data Importer for Agro Sentinel Worker

Importa datos de archivos JSON o CSV a DynamoDB. Soporta:
- Lectura desde archivos JSON o CSV
- Validación de esquema según tabla
- Inserción en DynamoDB (AWS o LocalStack)
- Modo dry-run (sin modificar datos)
- Manejo de errores y reintentos
"""

import json
import csv
import sys
import argparse
import logging
from pathlib import Path
from typing import Dict, List, Any, Optional, Tuple
from datetime import datetime
import os

import boto3
from botocore.exceptions import ClientError, BotoCoreError

# Configurar logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class DynamoDBImporter:
    """Importador de datos a DynamoDB para Agro Sentinel Worker"""

    # Esquemas de validación por tabla
    TABLE_SCHEMAS = {
        'monitoring_producciones': {
            'required_fields': [
                'produccion_id',
                'activa',
                'cultivo',
                'ciclo',
                'dias_produccion',
                'articulo_id',
                'centro_costo_id'
            ],
            'optional_fields': [
                'fecha_plantacion',
                'nombre_rancho'
            ],
            'field_types': {
                'produccion_id': 'number',
                'activa': 'boolean',
                'cultivo': 'string',
                'ciclo': 'string',
                'fecha_plantacion': 'string',
                'dias_produccion': 'number',
                'articulo_id': 'number',
                'centro_costo_id': 'number',
                'nombre_rancho': 'string'
            },
            'partition_key': 'produccion_id',
            'sort_key': None
        },
        'monitoring_escenas': {
            'required_fields': [
                'scene_id',
                'produccion_id',
                'date',
                'cloud_cover'
            ],
            'optional_fields': [
                'stac_assets'
            ],
            'field_types': {
                'scene_id': 'string',
                'produccion_id': 'number',
                'date': 'string',
                'cloud_cover': 'number',
                'stac_assets': 'map'
            },
            'partition_key': 'scene_id',
            'sort_key': None
        }
    }

    def __init__(self, table_name: str, endpoint_url: Optional[str] = None,
                 region: str = 'us-west-2', dry_run: bool = False):
        """
        Inicializar el importador.

        Args:
            table_name: Nombre de la tabla DynamoDB
            endpoint_url: URL del endpoint DynamoDB (None para AWS real)
            region: Región AWS
            dry_run: Si True, no modifica datos
        """
        self.table_name = table_name
        self.endpoint_url = endpoint_url
        self.region = region
        self.dry_run = dry_run

        # Validar que tabla sea conocida
        if table_name not in self.TABLE_SCHEMAS:
            raise ValueError(
                f"Tabla desconocida: {table_name}. "
                f"Tablas soportadas: {list(self.TABLE_SCHEMAS.keys())}"
            )

        self.schema = self.TABLE_SCHEMAS[table_name]

        # Crear cliente DynamoDB
        try:
            if endpoint_url:
                logger.info(f"Usando endpoint personalizado: {endpoint_url}")
                self.dynamodb = boto3.resource(
                    'dynamodb',
                    region_name=region,
                    endpoint_url=endpoint_url,
                    aws_access_key_id=os.getenv('AWS_ACCESS_KEY_ID', 'test'),
                    aws_secret_access_key=os.getenv('AWS_SECRET_ACCESS_KEY', 'test')
                )
            else:
                logger.info(f"Usando AWS región: {region}")
                self.dynamodb = boto3.resource(
                    'dynamodb',
                    region_name=region
                )
            self.table = self.dynamodb.Table(table_name)
            logger.info(f"Conectado a tabla: {table_name}")
        except ClientError as e:
            logger.error(f"Error conectando a DynamoDB: {e}")
            raise

    def load_data(self, file_path: str) -> List[Dict[str, Any]]:
        """
        Cargar datos desde archivo JSON o CSV.

        Args:
            file_path: Ruta del archivo

        Returns:
            Lista de diccionarios con los datos

        Raises:
            FileNotFoundError: Si el archivo no existe
            ValueError: Si el formato es inválido
        """
        path = Path(file_path)

        if not path.exists():
            raise FileNotFoundError(f"Archivo no encontrado: {file_path}")

        logger.info(f"Cargando datos desde: {file_path}")

        if path.suffix.lower() == '.json':
            return self._load_json(file_path)
        elif path.suffix.lower() == '.csv':
            return self._load_csv(file_path)
        else:
            raise ValueError(
                f"Formato no soportado: {path.suffix}. "
                f"Use .json o .csv"
            )

    def _load_json(self, file_path: str) -> List[Dict[str, Any]]:
        """Cargar datos desde archivo JSON"""
        try:
            with open(file_path, 'r', encoding='utf-8') as f:
                data = json.load(f)

            if not isinstance(data, list):
                raise ValueError("JSON debe contener un array de objetos")

            logger.info(f"Cargados {len(data)} items desde JSON")
            return data
        except json.JSONDecodeError as e:
            raise ValueError(f"Error parseando JSON: {e}")

    def _load_csv(self, file_path: str) -> List[Dict[str, Any]]:
        """Cargar datos desde archivo CSV"""
        try:
            data = []
            with open(file_path, 'r', encoding='utf-8') as f:
                reader = csv.DictReader(f)
                if reader.fieldnames is None:
                    raise ValueError("CSV vacío o sin encabezados")

                for row in reader:
                    data.append(row)

            logger.info(f"Cargados {len(data)} items desde CSV")
            return data
        except Exception as e:
            raise ValueError(f"Error leyendo CSV: {e}")

    def validate_item(self, item: Dict[str, Any], index: int) -> Tuple[bool, List[str]]:
        """
        Validar un item contra el esquema de la tabla.

        Args:
            item: Diccionario con los datos del item
            index: Índice del item (para mensajes de error)

        Returns:
            Tupla (válido: bool, errores: list[str])
        """
        errors = []

        # Validar campos requeridos
        for field in self.schema['required_fields']:
            if field not in item or item[field] == '' or item[field] is None:
                errors.append(
                    f"Item {index}: Campo requerido faltante o vacío: {field}"
                )

        # Validar tipos de datos
        for field, value in item.items():
            if field not in self.schema['field_types']:
                logger.warning(
                    f"Item {index}: Campo no reconocido en esquema: {field}"
                )
                continue

            expected_type = self.schema['field_types'][field]
            error = self._validate_field_type(field, value, expected_type, index)
            if error:
                errors.append(error)

        # Validaciones específicas
        if self.table_name == 'monitoring_producciones':
            errors.extend(
                self._validate_produccion_specific(item, index)
            )
        elif self.table_name == 'monitoring_escenas':
            errors.extend(
                self._validate_escena_specific(item, index)
            )

        return len(errors) == 0, errors

    def _validate_field_type(self, field: str, value: Any,
                           expected_type: str, index: int) -> Optional[str]:
        """Validar tipo de un campo específico"""
        if value is None or value == '':
            # Los campos opcionales pueden ser None/vacíos
            if field in self.schema['optional_fields']:
                return None
            return None  # Ya validado en required_fields

        try:
            if expected_type == 'number':
                float(value)
            elif expected_type == 'boolean':
                if not isinstance(value, bool):
                    if isinstance(value, str):
                        if value.lower() not in ['true', 'false']:
                            return (
                                f"Item {index}: Campo '{field}' debe ser boolean, "
                                f"recibido: {value}"
                            )
            elif expected_type == 'string':
                if not isinstance(value, str):
                    return (
                        f"Item {index}: Campo '{field}' debe ser string, "
                        f"recibido: {type(value).__name__}"
                    )
            elif expected_type == 'map':
                if not isinstance(value, dict):
                    return (
                        f"Item {index}: Campo '{field}' debe ser map/dict, "
                        f"recibido: {type(value).__name__}"
                    )
        except (ValueError, TypeError):
            return (
                f"Item {index}: Campo '{field}' tipo inválido para {expected_type}"
            )

        return None

    def _validate_produccion_specific(self, item: Dict[str, Any],
                                     index: int) -> List[str]:
        """Validaciones específicas para produccion"""
        errors = []

        # Validar ciclo formato
        if 'ciclo' in item and item['ciclo']:
            if not self._is_valid_ciclo(item['ciclo']):
                errors.append(
                    f"Item {index}: Ciclo debe ser formato YYYY-[A|B], "
                    f"recibido: {item['ciclo']}"
                )

        # Validar fecha si existe
        if 'fecha_plantacion' in item and item['fecha_plantacion']:
            if not self._is_valid_date(item['fecha_plantacion']):
                errors.append(
                    f"Item {index}: fecha_plantacion debe ser YYYY-MM-DD, "
                    f"recibido: {item['fecha_plantacion']}"
                )

        # Validar dias_produccion rango
        if 'dias_produccion' in item:
            try:
                dias = int(item['dias_produccion'])
                if dias < 60 or dias > 200:
                    errors.append(
                        f"Item {index}: dias_produccion debe estar entre 60-200, "
                        f"recibido: {dias}"
                    )
            except ValueError:
                errors.append(
                    f"Item {index}: dias_produccion debe ser número"
                )

        # Validar produccion_id es positivo
        if 'produccion_id' in item:
            try:
                pid = int(item['produccion_id'])
                if pid <= 0:
                    errors.append(
                        f"Item {index}: produccion_id debe ser > 0"
                    )
            except ValueError:
                errors.append(
                    f"Item {index}: produccion_id debe ser número entero"
                )

        return errors

    def _validate_escena_specific(self, item: Dict[str, Any],
                                 index: int) -> List[str]:
        """Validaciones específicas para escena"""
        errors = []

        # Validar date formato
        if 'date' in item and item['date']:
            if not self._is_valid_date(item['date']):
                errors.append(
                    f"Item {index}: date debe ser YYYY-MM-DD, "
                    f"recibido: {item['date']}"
                )

        # Validar cloud_cover rango
        if 'cloud_cover' in item:
            try:
                cc = float(item['cloud_cover'])
                if cc < 0 or cc > 100:
                    errors.append(
                        f"Item {index}: cloud_cover debe estar entre 0-100, "
                        f"recibido: {cc}"
                    )
            except ValueError:
                errors.append(
                    f"Item {index}: cloud_cover debe ser número"
                )

        # Validar produccion_id es positivo
        if 'produccion_id' in item:
            try:
                pid = int(item['produccion_id'])
                if pid <= 0:
                    errors.append(
                        f"Item {index}: produccion_id debe ser > 0"
                    )
            except ValueError:
                errors.append(
                    f"Item {index}: produccion_id debe ser número entero"
                )

        # Validar scene_id no esté vacío
        if 'scene_id' in item:
            if not item['scene_id'] or not str(item['scene_id']).strip():
                errors.append(
                    f"Item {index}: scene_id no puede estar vacío"
                )

        # Validar stac_assets si existe
        if 'stac_assets' in item and item['stac_assets']:
            if not isinstance(item['stac_assets'], dict):
                errors.append(
                    f"Item {index}: stac_assets debe ser un diccionario"
                )
            else:
                for band, asset in item['stac_assets'].items():
                    if not isinstance(asset, dict):
                        errors.append(
                            f"Item {index}: stac_assets.{band} debe ser un diccionario"
                        )
                    elif 'href' not in asset or 'resolution' not in asset:
                        errors.append(
                            f"Item {index}: stac_assets.{band} debe tener 'href' y 'resolution'"
                        )

        return errors

    def _is_valid_date(self, date_str: str) -> bool:
        """Validar formato de fecha YYYY-MM-DD"""
        try:
            datetime.strptime(date_str, '%Y-%m-%d')
            return True
        except (ValueError, TypeError):
            return False

    def _is_valid_ciclo(self, ciclo_str: str) -> bool:
        """Validar formato de ciclo YYYY-[A|B]"""
        parts = str(ciclo_str).split('-')
        if len(parts) != 2:
            return False
        try:
            int(parts[0])
            return parts[1] in ['A', 'B']
        except ValueError:
            return False

    def convert_types(self, item: Dict[str, Any]) -> Dict[str, Any]:
        """
        Convertir tipos de datos según esquema.

        Convierte strings a números, booleanos, etc.
        """
        converted = {}

        for field, value in item.items():
            if value is None or value == '':
                # Omitir campos vacíos (DynamoDB no almacena NULL)
                if field not in self.schema['required_fields']:
                    continue

            expected_type = self.schema['field_types'].get(field)

            if expected_type == 'number':
                try:
                    # Intentar int primero, luego float
                    if '.' in str(value):
                        converted[field] = float(value)
                    else:
                        converted[field] = int(value)
                except (ValueError, TypeError):
                    converted[field] = value
            elif expected_type == 'boolean':
                if isinstance(value, bool):
                    converted[field] = value
                elif isinstance(value, str):
                    converted[field] = value.lower() in ['true', '1', 'yes']
                else:
                    converted[field] = bool(value)
            else:
                converted[field] = value

        return converted

    def import_items(self, items: List[Dict[str, Any]],
                    batch_size: int = 25) -> Dict[str, Any]:
        """
        Importar items a DynamoDB.

        Args:
            items: Lista de items a importar
            batch_size: Tamaño de batch para escritura

        Returns:
            Diccionario con estadísticas de la importación
        """
        stats = {
            'total': len(items),
            'success': 0,
            'failed': 0,
            'skipped': 0,
            'errors': [],
            'start_time': datetime.now().isoformat(),
            'end_time': None
        }

        logger.info(f"Iniciando importación de {len(items)} items")
        if self.dry_run:
            logger.warning("MODO DRY-RUN: No se modificarán datos")

        # Validar todos los items primero
        logger.info("Validando items...")
        valid_items = []
        for index, item in enumerate(items):
            valid, errors = self.validate_item(item, index)
            if not valid:
                for error in errors:
                    logger.error(error)
                    stats['errors'].append(error)
                stats['failed'] += 1
            else:
                valid_items.append(item)

        if stats['failed'] > 0:
            logger.warning(f"Se encontraron {stats['failed']} items inválidos")

        logger.info(f"Validados: {stats['success'] + len(valid_items)}, "
                   f"Inválidos: {stats['failed']}")

        # Convertir tipos
        logger.info("Convirtiendo tipos de datos...")
        converted_items = [self.convert_types(item) for item in valid_items]

        # Importar en batches
        logger.info(f"Importando {len(converted_items)} items válidos...")

        if not self.dry_run:
            try:
                # Usar batch_write_item para mejor performance
                with self.table.batch_writer(batch_size=batch_size) as batch:
                    for item in converted_items:
                        batch.put_item(Item=item)
                        stats['success'] += 1
            except ClientError as e:
                logger.error(f"Error escribiendo a DynamoDB: {e}")
                stats['failed'] = len(converted_items)
                stats['errors'].append(str(e))
        else:
            # En dry-run, solo simular
            logger.info("[DRY-RUN] Se escribirían los siguientes items:")
            for i, item in enumerate(converted_items[:5]):
                logger.info(f"  Item {i+1}: {json.dumps(item, indent=2, default=str)}")
            if len(converted_items) > 5:
                logger.info(f"  ... y {len(converted_items) - 5} items más")
            stats['success'] = len(converted_items)

        stats['end_time'] = datetime.now().isoformat()

        return stats

    def print_summary(self, stats: Dict[str, Any]) -> None:
        """Imprimir resumen de importación"""
        logger.info("=" * 70)
        logger.info("RESUMEN DE IMPORTACIÓN")
        logger.info("=" * 70)
        logger.info(f"Tabla: {self.table_name}")
        logger.info(f"Total items: {stats['total']}")
        logger.info(f"Items exitosos: {stats['success']}")
        logger.info(f"Items fallidos: {stats['failed']}")
        logger.info(f"Items saltados: {stats['skipped']}")

        if stats['errors']:
            logger.error("Errores encontrados:")
            for error in stats['errors'][:10]:  # Mostrar primeros 10
                logger.error(f"  - {error}")
            if len(stats['errors']) > 10:
                logger.error(f"  ... y {len(stats['errors']) - 10} errores más")

        logger.info(f"Inicio: {stats['start_time']}")
        logger.info(f"Fin: {stats['end_time']}")
        logger.info("=" * 70)


def main():
    """Función principal"""
    parser = argparse.ArgumentParser(
        description='Importar datos a DynamoDB para Agro Sentinel Worker',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Ejemplos de uso:

  # Importar producciones desde JSON
  python import-dynamodb.py \\
    --table monitoring_producciones \\
    --file data/dynamodb/examples/monitoring_producciones.json

  # Importar escenas con dry-run
  python import-dynamodb.py \\
    --table monitoring_escenas \\
    --file data/dynamodb/examples/monitoring_escenas.json \\
    --dry-run

  # Importar desde LocalStack
  python import-dynamodb.py \\
    --table monitoring_producciones \\
    --file data/dynamodb/examples/monitoring_producciones.json \\
    --endpoint-url http://localhost:4566

  # Importar desde CSV
  python import-dynamodb.py \\
    --table monitoring_escenas \\
    --file data/monitoring_escenas.csv \\
    --region us-west-2
        """
    )

    parser.add_argument(
        '--table',
        required=True,
        choices=['monitoring_producciones', 'monitoring_escenas'],
        help='Nombre de la tabla DynamoDB'
    )

    parser.add_argument(
        '--file',
        required=True,
        help='Ruta del archivo JSON o CSV con los datos'
    )

    parser.add_argument(
        '--endpoint-url',
        default=None,
        help='URL del endpoint DynamoDB (dejar vacío para AWS real, '
             'usar http://localhost:4566 para LocalStack)'
    )

    parser.add_argument(
        '--region',
        default='us-west-2',
        help='Región AWS (default: us-west-2)'
    )

    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Validar datos sin modificar DynamoDB'
    )

    parser.add_argument(
        '--batch-size',
        type=int,
        default=25,
        help='Tamaño de batch para escritura (default: 25)'
    )

    parser.add_argument(
        '--verbose',
        action='store_true',
        help='Mostrar más detalles'
    )

    args = parser.parse_args()

    # Ajustar nivel de logging
    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    try:
        # Crear importador
        importer = DynamoDBImporter(
            table_name=args.table,
            endpoint_url=args.endpoint_url,
            region=args.region,
            dry_run=args.dry_run
        )

        # Cargar datos
        items = importer.load_data(args.file)

        # Importar
        stats = importer.import_items(items, batch_size=args.batch_size)

        # Mostrar resumen
        importer.print_summary(stats)

        # Retornar código de salida apropiado
        sys.exit(0 if stats['failed'] == 0 else 1)

    except Exception as e:
        logger.error(f"Error fatal: {e}")
        if args.verbose:
            import traceback
            traceback.print_exc()
        sys.exit(2)


if __name__ == '__main__':
    main()
