#!/usr/bin/env python3
import json
import csv
import boto3
import logging
import re

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

def convert_types(item, table_name):
    """Convierte tipos de datos según lo esperado"""
    
    # Para monitoring_producciones
    if table_name == 'monitoring_producciones':
        if 'produccion_id' in item and isinstance(item['produccion_id'], str):
            try:
                item['produccion_id'] = int(item['produccion_id'])
            except:
                item['produccion_id'] = float(item['produccion_id'])
        
        if 'dias_max_monitoreo' in item and isinstance(item['dias_max_monitoreo'], str):
            try:
                item['dias_max_monitoreo'] = int(item['dias_max_monitoreo'])
            except:
                pass
    
    # Para monitoring_escenas: mapear keys correctamente
    if table_name == 'monitoring_escenas':
        # Mapear 'id' a 'produccion_id' (extraer número de "PROD#2044" → 2044)
        if 'id' in item and 'produccion_id' not in item:
            id_val = item.pop('id')
            try:
                # Extraer número de PROD#2044
                num = int(re.search(r'\d+', id_val).group())
                item['produccion_id'] = num
            except:
                item['produccion_id'] = id_val
        
        # Mapear 'clave' a 'scene_id'
        if 'clave' in item and 'scene_id' not in item:
            item['scene_id'] = item.pop('clave')
    
    return item

def load_data(file_path, format_type='json', table_name='monitoring_producciones'):
    if format_type == 'csv':
        items = []
        with open(file_path, 'r', encoding='utf-8') as f:
            reader = csv.DictReader(f)
            for row in reader:
                # Convertir JSON strings a dicts
                for key in ['bbox', 'polygon', 'pbox']:
                    if key in row and isinstance(row[key], str):
                        try:
                            row[key] = json.loads(row[key])
                        except:
                            pass
                
                row = convert_types(row, table_name)
                items.append(row)
        return items
    else:
        with open(file_path, 'r', encoding='utf-8') as f:
            items = json.load(f)
            return [convert_types(item, table_name) for item in items]

def import_dynamodb(table_name, file_path, format_type='json', dry_run=False, endpoint_url='http://localhost:4566', region='us-east-1'):
    dynamodb = boto3.resource('dynamodb', endpoint_url=endpoint_url, region_name=region)
    table = dynamodb.Table(table_name)
    
    items = load_data(file_path, format_type, table_name)
    logging.info(f"Cargados {len(items)} items")
    logging.info(f"Tabla: {table_name}, Región: {region}")
    
    success_count = 0
    for i, item in enumerate(items):
        if not dry_run:
            try:
                table.put_item(Item=item)
                if table_name == 'monitoring_producciones':
                    pk = item.get('produccion_id')
                    logging.info(f"✓ Item {i}: {pk}")
                else:
                    pk = item.get('produccion_id')
                    sk = item.get('scene_id')
                    logging.info(f"✓ Item {i}: PK={pk}, SK={sk}")
                success_count += 1
            except Exception as e:
                logging.error(f"✗ Item {i}: {str(e)}")
        else:
            logging.info(f"DRY-RUN: Item {i}")
            success_count += 1
    
    logging.info(f"✓ {success_count}/{len(items)} items procesados")

if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--table', required=True, choices=['monitoring_producciones', 'monitoring_escenas'])
    parser.add_argument('--file', required=True)
    parser.add_argument('--format', default='json', choices=['json', 'csv'])
    parser.add_argument('--dry-run', action='store_true')
    parser.add_argument('--endpoint-url', default='http://localhost:4566')
    parser.add_argument('--region', default='us-east-1')
    args = parser.parse_args()
    
    import_dynamodb(args.table, args.file, args.format, args.dry_run, args.endpoint_url, args.region)
