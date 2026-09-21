#!/bin/bash
set -e

echo "=== Inicializando recursos en Localstack ==="

# S3 bucket
awslocal s3 mb s3://agro-sentinel-bucket 2>/dev/null || echo "Bucket ya existe"

# SQS queue
awslocal sqs create-queue --queue-name agro-sentinel-jobs 2>/dev/null || echo "Cola ya existe"

# DynamoDB — tabla producciones
awslocal dynamodb create-table \
  --table-name monitoring_producciones \
  --attribute-definitions AttributeName=produccion_id,AttributeType=N \
  --key-schema AttributeName=produccion_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST 2>/dev/null || echo "Tabla monitoring_producciones ya existe"

# DynamoDB — tabla escenas (sort key: clave = scene name)
awslocal dynamodb create-table \
  --table-name monitoring_escenas \
  --attribute-definitions \
    AttributeName=produccion_id,AttributeType=N \
    AttributeName=clave,AttributeType=S \
  --key-schema \
    AttributeName=produccion_id,KeyType=HASH \
    AttributeName=clave,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST 2>/dev/null || echo "Tabla monitoring_escenas ya existe"

echo "=== Localstack listo ==="
echo "  S3:       s3://agro-sentinel-bucket"
echo "  SQS:      http://localhost:4566/000000000000/agro-sentinel-jobs"
echo "  DynamoDB:  monitoring_producciones, monitoring_escenas"
