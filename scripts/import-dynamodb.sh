#!/bin/bash

#############################################################################
# DynamoDB Import Script for Agro Sentinel Worker
#
# Herramienta wrapper para importar datos a DynamoDB desde JSON/CSV
#
# Uso:
#   ./import-dynamodb.sh --table <tabla> --file <archivo> [opciones]
#
# Opciones:
#   --table              Nombre de tabla (monitoring_producciones|monitoring_escenas)
#   --file              Ruta del archivo JSON o CSV
#   --endpoint-url      URL endpoint DynamoDB (opcional)
#   --region            Región AWS (default: us-west-2)
#   --dry-run           Validar sin modificar datos
#   --batch-size        Tamaño de batch (default: 25)
#   --verbose           Mostrar más detalles
#   --help              Mostrar esta ayuda
#
#############################################################################

set -e

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Variables globales
TABLE=""
FILE=""
ENDPOINT_URL=""
REGION="us-west-2"
DRY_RUN=false
BATCH_SIZE=25
VERBOSE=false
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
PYTHON_SCRIPT="$SCRIPT_DIR/import-dynamodb.py"

# Función de ayuda
show_help() {
    cat << 'EOF'
DynamoDB Import Tool - Agro Sentinel Worker

Importa datos de archivos JSON o CSV a DynamoDB para el sistema Agro Sentinel.

SINTAXIS:
  ./import-dynamodb.sh --table <tabla> --file <archivo> [opciones]

TABLAS SOPORTADAS:
  monitoring_producciones    Tabla de producciones agrícolas
  monitoring_escenas         Tabla de escenas de satélite

OPCIONES:
  --table TABLE              Nombre de la tabla (requerido)
  --file FILE                Ruta del archivo JSON o CSV (requerido)
  --endpoint-url URL         URL endpoint DynamoDB
                            Ej: http://localhost:4566 para LocalStack
                            Dejar vacío para AWS real
  --region REGION            Región AWS (default: us-west-2)
  --dry-run                  Validar datos sin modificar DynamoDB
  --batch-size SIZE          Tamaño de batch para escritura (default: 25)
  --verbose                  Mostrar más detalles de ejecución
  --help                     Mostrar esta ayuda

EJEMPLOS:

1. Importar producciones desde JSON (AWS real):
   ./import-dynamodb.sh \\
     --table monitoring_producciones \\
     --file data/dynamodb/examples/monitoring_producciones.json

2. Importar escenas con validación (dry-run):
   ./import-dynamodb.sh \\
     --table monitoring_escenas \\
     --file data/dynamodb/examples/monitoring_escenas.json \\
     --dry-run

3. Importar desde LocalStack:
   ./import-dynamodb.sh \\
     --table monitoring_producciones \\
     --file data/dynamodb/examples/monitoring_producciones.json \\
     --endpoint-url http://localhost:4566

4. Importar con más detalles:
   ./import-dynamodb.sh \\
     --table monitoring_escenas \\
     --file data/monitoring_escenas.csv \\
     --verbose \\
     --region us-east-1

REQUISITOS:
  - Python 3.7+
  - boto3 instalado (pip install boto3)
  - Credenciales AWS configuradas (si no usa LocalStack)
  - Archivo JSON o CSV bien formado

ESTRUCTURA DE ARCHIVOS JSON:

monitoring_producciones:
  {
    "produccion_id": 12345,
    "activa": true,
    "cultivo": "maiz",
    "ciclo": "2026-A",
    "fecha_plantacion": "2026-01-15",
    "dias_produccion": 120,
    "articulo_id": 5001,
    "centro_costo_id": 101,
    "nombre_rancho": "Rancho El Remanso"
  }

monitoring_escenas:
  {
    "scene_id": "S2A_MSIL2A_20260205T135051_N0510_R024_T19HCC_20260205T135101",
    "produccion_id": 12345,
    "date": "2026-02-05",
    "cloud_cover": 12.5,
    "stac_assets": {
      "B04": {
        "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B04.jp2",
        "resolution": 10
      }
    }
  }

CÓDIGOS DE SALIDA:
  0    Importación exitosa
  1    Fallos en la importación (algunos items fallaron)
  2    Error fatal (no se ejecutó)

DOCUMENTACIÓN:
  Ver docs/DYNAMODB_IMPORT_GUIDE.md para más información

EOF
}

# Función para mostrar error
error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Función para mostrar advertencia
warning() {
    echo -e "${YELLOW}[ADVERTENCIA]${NC} $*"
}

# Función para mostrar info
info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

# Función para mostrar debug
debug() {
    if [ "$VERBOSE" = true ]; then
        echo -e "${BLUE}[DEBUG]${NC} $*"
    fi
}

# Parsear argumentos
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --table)
                TABLE="$2"
                shift 2
                ;;
            --file)
                FILE="$2"
                shift 2
                ;;
            --endpoint-url)
                ENDPOINT_URL="$2"
                shift 2
                ;;
            --region)
                REGION="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --batch-size)
                BATCH_SIZE="$2"
                shift 2
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            --help)
                show_help
                exit 0
                ;;
            *)
                error "Opción desconocida: $1"
                exit 2
                ;;
        esac
    done
}

# Validar argumentos
validate_args() {
    if [ -z "$TABLE" ]; then
        error "La opción --table es requerida"
        exit 2
    fi

    if [ -z "$FILE" ]; then
        error "La opción --file es requerida"
        exit 2
    fi

    case "$TABLE" in
        monitoring_producciones|monitoring_escenas)
            debug "Tabla válida: $TABLE"
            ;;
        *)
            error "Tabla desconocida: $TABLE"
            error "Tablas soportadas: monitoring_producciones, monitoring_escenas"
            exit 2
            ;;
    esac

    if [ ! -f "$FILE" ]; then
        error "Archivo no encontrado: $FILE"
        exit 2
    fi

    debug "Archivo encontrado: $FILE"
}

# Validar requisitos
check_requirements() {
    debug "Verificando requisitos..."

    # Verificar Python
    if ! command -v python3 &> /dev/null; then
        error "Python 3 no está instalado"
        echo "Por favor instale Python 3.7 o superior"
        exit 2
    fi

    PYTHON_VERSION=$(python3 --version 2>&1 | awk '{print $2}')
    info "Python encontrado: $PYTHON_VERSION"

    # Verificar boto3
    if ! python3 -c "import boto3" 2>/dev/null; then
        error "boto3 no está instalado"
        echo "Instálelo con: pip install boto3"
        exit 2
    fi

    debug "boto3 encontrado"

    # Verificar script Python
    if [ ! -f "$PYTHON_SCRIPT" ]; then
        error "Script Python no encontrado: $PYTHON_SCRIPT"
        exit 2
    fi

    debug "Script Python encontrado: $PYTHON_SCRIPT"
}

# Función principal
main() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║       DynamoDB Import Tool - Agro Sentinel Worker            ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo ""

    # Parsear y validar argumentos
    parse_args "$@"
    validate_args
    check_requirements

    # Mostrar configuración
    echo "Configuración:"
    echo "  Tabla: $TABLE"
    echo "  Archivo: $FILE"
    echo "  Región: $REGION"
    if [ -n "$ENDPOINT_URL" ]; then
        echo "  Endpoint: $ENDPOINT_URL"
    else
        echo "  Endpoint: AWS real (us-west-2)"
    fi
    echo "  Batch size: $BATCH_SIZE"
    if [ "$DRY_RUN" = true ]; then
        echo "  Modo: DRY-RUN (sin modificar datos)"
    else
        echo "  Modo: REAL (modificará DynamoDB)"
    fi
    echo ""

    # Confirmar si no es dry-run
    if [ "$DRY_RUN" = false ]; then
        warning "Esta operación modificará datos en DynamoDB"
        read -p "¿Continuar? (s/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Ss]$ ]]; then
            error "Operación cancelada por el usuario"
            exit 0
        fi
    fi

    # Construir comando Python
    PYTHON_CMD="python3 \"$PYTHON_SCRIPT\" --table \"$TABLE\" --file \"$FILE\" --region \"$REGION\" --batch-size $BATCH_SIZE"

    if [ -n "$ENDPOINT_URL" ]; then
        PYTHON_CMD="$PYTHON_CMD --endpoint-url \"$ENDPOINT_URL\""
    fi

    if [ "$DRY_RUN" = true ]; then
        PYTHON_CMD="$PYTHON_CMD --dry-run"
    fi

    if [ "$VERBOSE" = true ]; then
        PYTHON_CMD="$PYTHON_CMD --verbose"
    fi

    debug "Ejecutando: $PYTHON_CMD"

    # Ejecutar
    echo "Iniciando importación..."
    echo ""

    if eval "$PYTHON_CMD"; then
        echo ""
        info "Importación completada exitosamente"
        return 0
    else
        EXIT_CODE=$?
        echo ""
        error "Importación fallida con código: $EXIT_CODE"
        return $EXIT_CODE
    fi
}

# Ejecutar
main "$@"
exit $?
