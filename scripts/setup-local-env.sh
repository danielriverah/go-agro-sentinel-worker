#!/bin/bash

###############################################################################
# Setup Local Environment - Agro Sentinel Worker
# Inicializa el entorno local completo: Docker, MySQL, DynamoDB, S3
###############################################################################

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
CHECKS_PASSED=0
CHECKS_TOTAL=0

###############################################################################
# FUNCIONES UTILITARIAS
###############################################################################

log_header() {
    echo -e "\n${BLUE}═════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}═════════════════════════════════════════════════════════════════${NC}\n"
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
    ((CHECKS_PASSED++))
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

check_command() {
    local cmd=$1
    local name=$2
    ((CHECKS_TOTAL++))

    if command -v "$cmd" &> /dev/null; then
        log_success "$name instalado"
        return 0
    else
        log_error "$name no instalado"
        return 1
    fi
}

###############################################################################
# PASO 1: VERIFICAR REQUISITOS
###############################################################################

verify_requirements() {
    log_header "PASO 1: Verificar Requisitos"

    check_command "docker" "Docker"
    check_command "docker-compose" "Docker Compose"
    check_command "git" "Git"

    if ! check_command "aws" "AWS CLI"; then
        log_warning "AWS CLI no instalado. Algunas operaciones pueden fallar."
        log_info "Instala con: pip install awscli"
    fi

    if ! check_command "mysql" "MySQL Client"; then
        log_warning "MySQL Client no instalado. Algunas operaciones pueden fallar."
        log_info "Instala con: apt-get install mysql-client (Linux) o brew install mysql-client (Mac)"
    fi

    # Verificar Docker daemon
    if ! docker ps > /dev/null 2>&1; then
        log_error "Docker daemon no está corriendo"
        log_info "Inicia Docker Desktop y vuelve a intentar"
        exit 1
    fi

    log_success "Docker daemon está corriendo"
}

###############################################################################
# PASO 2: LIMPIAR ESTADO ANTERIOR
###############################################################################

cleanup_docker() {
    log_header "PASO 2: Limpiar Estado Anterior (Opcional)"

    read -p "¿Deseas limpiar contenedores/volúmenes anteriores? (s/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Ss]$ ]]; then
        log_info "Deteniendo contenedores..."
        docker-compose -f "$PROJECT_ROOT/docker-compose.yml" down 2>/dev/null || true

        log_info "¿Eliminar volúmenes? (borrará datos) (s/n):"
        read -n 1 -r
        echo
        if [[ $REPLY =~ ^[Ss]$ ]]; then
            docker-compose -f "$PROJECT_ROOT/docker-compose.yml" down -v 2>/dev/null || true
            log_success "Volúmenes eliminados"
        fi
    else
        log_info "Limpieza cancelada"
    fi
}

###############################################################################
# PASO 3: CONSTRUIR IMÁGENES
###############################################################################

build_images() {
    log_header "PASO 3: Construir Imágenes Docker"

    log_info "Construyendo imágenes (esto puede tomar 2-5 minutos)..."

    if docker-compose -f "$PROJECT_ROOT/docker-compose.yml" build; then
        log_success "Imágenes construidas correctamente"
    else
        log_error "Error construyendo imágenes"
        exit 1
    fi
}

###############################################################################
# PASO 4: INICIAR SERVICIOS
###############################################################################

start_services() {
    log_header "PASO 4: Iniciar Servicios Docker"

    log_info "Iniciando MySQL..."
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" up -d mysql

    log_info "Esperando MySQL health check (hasta 30 segundos)..."
    for i in {1..30}; do
        if docker-compose -f "$PROJECT_ROOT/docker-compose.yml" ps mysql | grep -q "healthy"; then
            log_success "MySQL está healthy"
            break
        fi
        echo -n "."
        sleep 1
    done

    log_info "Iniciando LocalStack..."
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" up -d localstack
    sleep 5  # LocalStack tarda un poco en iniciarse
    log_success "LocalStack iniciado"

    log_info "Iniciando API..."
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" up -d api
    sleep 2
    log_success "API iniciado"

    log_info "Iniciando Worker..."
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" up -d worker
    sleep 2
    log_success "Worker iniciado"

    log_info "Iniciando Sync Service..."
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" up -d sync
    sleep 2
    log_success "Sync Service iniciado"

    log_info "Verificando estado de todos los servicios..."
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" ps
}

###############################################################################
# PASO 5: VERIFICAR TABLAS MySQL
###############################################################################

verify_mysql_tables() {
    log_header "PASO 5: Verificar Tablas MySQL"

    # Esperar a que MySQL esté listo
    sleep 3

    log_info "Verificando conexión a MySQL..."
    if mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SELECT VERSION();" 2>/dev/null; then
        log_success "MySQL accesible"
    else
        log_error "No se puede conectar a MySQL"
        return 1
    fi

    log_info "Verificando tablas..."
    TABLES=$(mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SHOW TABLES;" 2>/dev/null | wc -l)

    if [ "$TABLES" -gt 1 ]; then
        log_success "MySQL contiene $((TABLES-1)) tablas"
    else
        log_warning "No se encontraron tablas. Las tablas pueden estar en creación..."
    fi
}

###############################################################################
# PASO 6: VERIFICAR DYNAMODB
###############################################################################

verify_dynamodb() {
    log_header "PASO 6: Verificar DynamoDB (LocalStack)"

    log_info "Verificando tablas DynamoDB..."

    if TABLES=$(aws dynamodb list-tables \
        --endpoint-url http://localhost:4566 \
        --region us-east-1 2>/dev/null); then

        TABLE_COUNT=$(echo "$TABLES" | grep -o "monitoring" | wc -l)

        if [ "$TABLE_COUNT" -ge 2 ]; then
            log_success "Encontradas tablas DynamoDB (monitoring_producciones, monitoring_escenas)"
        else
            log_warning "No se encontraron todas las tablas DynamoDB. Pueden estar en creación..."
        fi
    else
        log_warning "No se puede conectar a DynamoDB (AWS CLI puede no estar instalado)"
    fi
}

###############################################################################
# PASO 7: VERIFICAR S3
###############################################################################

verify_s3() {
    log_header "PASO 7: Verificar S3 (LocalStack)"

    log_info "Verificando buckets S3..."

    if BUCKETS=$(aws s3 ls \
        --endpoint-url http://localhost:4566 \
        --region us-east-1 2>/dev/null); then

        if echo "$BUCKETS" | grep -q "agro-sentinel-bucket"; then
            log_success "Bucket S3 encontrado (agro-sentinel-bucket)"
        else
            log_warning "Bucket S3 no encontrado"
        fi
    else
        log_warning "No se puede conectar a S3 (AWS CLI puede no estar instalado)"
    fi
}

###############################################################################
# PASO 8: CARGAR DATOS DE PRUEBA
###############################################################################

load_test_data() {
    log_header "PASO 8: Cargar Datos de Prueba"

    read -p "¿Deseas cargar datos de prueba? (s/n): " -n 1 -r
    echo

    if [[ ! $REPLY =~ ^[Ss]$ ]]; then
        log_info "Carga de datos cancelada"
        return
    fi

    log_info "Insertando datos en MySQL..."

    mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF 2>/dev/null || log_warning "Error insertando datos (pueden ya existir)"
INSERT IGNORE INTO articulos (articulo_id, nombre, variedad)
VALUES
  (1, 'Tomate', 'Roma'),
  (2, 'Pepino', 'Japonés'),
  (3, 'Lechuga', 'Romana');

INSERT IGNORE INTO centros_costos (centro_costo_id, nombre)
VALUES
  (1, 'Rancho A'),
  (2, 'Rancho B');

INSERT IGNORE INTO producciones (produccion_id, folio, articulo_id, centro_costo_id, fecha)
VALUES
  (1, 'PROD-001', 1, 1, CURDATE()),
  (2, 'PROD-002', 2, 2, CURDATE());
EOF

    log_success "Datos insertados en MySQL"

    log_info "Insertando datos en DynamoDB (si AWS CLI está disponible)..."

    if command -v aws &> /dev/null; then
        aws dynamodb put-item \
            --table-name monitoring_producciones \
            --item '{
              "produccion_id": {"N": "1"},
              "folio": {"S": "PROD-001"},
              "status": {"S": "monitoring"},
              "cantidad_escenas": {"N": "0"}
            }' \
            --endpoint-url http://localhost:4566 \
            --region us-east-1 2>/dev/null && log_success "Datos insertados en DynamoDB" || log_warning "Error insertando en DynamoDB"
    else
        log_warning "AWS CLI no disponible, se salteó inserción en DynamoDB"
    fi
}

###############################################################################
# PASO 9: VERIFICAR CONECTIVIDAD
###############################################################################

verify_connectivity() {
    log_header "PASO 9: Verificar Conectividad de Servicios"

    log_info "Verificando API health endpoint..."
    if HEALTH=$(curl -s http://localhost:8088/health 2>/dev/null); then
        if echo "$HEALTH" | grep -q "healthy"; then
            log_success "API respondiendo correctamente"
        else
            log_warning "API respondiendo pero respuesta inesperada: $HEALTH"
        fi
    else
        log_warning "No se pudo alcanzar API en http://localhost:8088/health"
    fi
}

###############################################################################
# PASO 10: RESUMEN
###############################################################################

print_summary() {
    log_header "RESUMEN DE SETUP"

    log_success "Setup completado"
    log_success "$CHECKS_PASSED de $CHECKS_TOTAL verificaciones pasadas"

    echo -e "\n${GREEN}Servicios disponibles:${NC}"
    echo -e "  ${GREEN}MySQL${NC}       → Host: 127.0.0.1, Puerto: 3309, User: root, Pass: root"
    echo -e "  ${GREEN}LocalStack${NC}  → http://localhost:4566 (S3, DynamoDB, SQS)"
    echo -e "  ${GREEN}API${NC}         → http://localhost:8088"
    echo -e "  ${GREEN}Worker${NC}      → Procesando jobs de SQS"
    echo -e "  ${GREEN}Sync${NC}        → Sincronizando DynamoDB → MySQL"

    echo -e "\n${GREEN}Próximos pasos:${NC}"
    echo -e "  1. Revisar logs: ${YELLOW}docker-compose logs -f${NC}"
    echo -e "  2. Testing: ${YELLOW}bash scripts/test-sync-flow.sh${NC}"
    echo -e "  3. Guía: ${YELLOW}cat SYNC_TESTING_GUIDE.md${NC}"

    echo -e "\n${BLUE}═════════════════════════════════════════════════════════════════${NC}\n"
}

###############################################################################
# MAIN
###############################################################################

main() {
    log_header "SETUP LOCAL ENVIRONMENT - AGRO SENTINEL WORKER"

    verify_requirements
    cleanup_docker
    build_images
    start_services

    # Dar tiempo a los servicios a iniciarse completamente
    log_info "Esperando a que los servicios se estabilicen (30 segundos)..."
    for i in {1..30}; do
        echo -n "."
        sleep 1
    done
    echo ""

    verify_mysql_tables
    verify_dynamodb
    verify_s3
    load_test_data
    verify_connectivity
    print_summary
}

# Ejecutar main
main "$@"
