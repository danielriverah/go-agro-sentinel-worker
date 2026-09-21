#!/bin/bash

###############################################################################
# Test Sync Flow - Agro Sentinel Worker
# Prueba flujo completo de sincronización DynamoDB → MySQL
###############################################################################

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Contadores
TESTS_PASSED=0
TESTS_TOTAL=0

###############################################################################
# FUNCIONES UTILITARIAS
###############################################################################

log_header() {
    echo -e "\n${BLUE}═════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}═════════════════════════════════════════════════════════════════${NC}\n"
}

log_test() {
    echo -e "\n${YELLOW}[TEST $((TESTS_TOTAL + 1))]${NC} $1"
    ((TESTS_TOTAL++))
}

log_step() {
    echo -e "${BLUE}  ├─${NC} $1"
}

log_success() {
    echo -e "${GREEN}  └─ ✓${NC} $1"
    ((TESTS_PASSED++))
}

log_error() {
    echo -e "${RED}  └─ ✗${NC} $1"
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

###############################################################################
# TEST 1: SYNC BASICO
###############################################################################

test_basic_sync() {
    log_test "Sincronización Básica (1 Producción)"

    log_step "Insertar producción en DynamoDB (ID: 501)"

    aws dynamodb put-item \
        --table-name monitoring_producciones \
        --item '{
          "produccion_id": {"N": "501"},
          "folio": {"S": "TEST-SYNC-BASIC-501"},
          "articulo_id": {"N": "1"},
          "centro_costo_id": {"N": "1"},
          "status": {"S": "monitoring"},
          "fecha": {"S": "2026-09-04"},
          "fecha_creacion": {"S": "2026-09-04T12:00:00Z"},
          "fecha_actualizacion": {"S": "2026-09-04T12:00:00Z"},
          "ultima_escena": {"S": ""},
          "cantidad_escenas": {"N": "0"}
        }' \
        --endpoint-url http://localhost:4566 \
        --region us-east-1 2>/dev/null

    log_step "Reiniciar sync service"
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" restart sync > /dev/null 2>&1
    sleep 3

    log_step "Verificar en MySQL"
    RESULT=$(mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
        "SELECT COUNT(*) FROM monitoreo_produccion_temporal WHERE folio = 'TEST-SYNC-BASIC-501';" 2>/dev/null | tail -1)

    if [ "$RESULT" = "1" ]; then
        log_success "Producción sincronizada correctamente"
        return 0
    else
        log_error "Producción no encontrada en MySQL (Resultado: $RESULT)"
        return 1
    fi
}

###############################################################################
# TEST 2: MULTIPLES PRODUCCIONES
###############################################################################

test_multiple_sync() {
    log_test "Múltiples Producciones (Batch Sync)"

    log_step "Insertar 5 producciones en DynamoDB"

    for i in {1..5}; do
        PROD_ID=$((510 + i))
        aws dynamodb put-item \
            --table-name monitoring_producciones \
            --item "{
              \"produccion_id\": {\"N\": \"$PROD_ID\"},
              \"folio\": {\"S\": \"TEST-BATCH-$PROD_ID\"},
              \"articulo_id\": {\"N\": \"$((i % 3 + 1))\"},
              \"centro_costo_id\": {\"N\": \"$((i % 2 + 1))\"},
              \"status\": {\"S\": \"monitoring\"},
              \"fecha\": {\"S\": \"2026-09-04\"},
              \"fecha_creacion\": {\"S\": \"2026-09-04T12:00:00Z\"},
              \"fecha_actualizacion\": {\"S\": \"2026-09-04T12:00:00Z\"},
              \"ultima_escena\": {\"S\": \"\"},
              \"cantidad_escenas\": {\"N\": \"0\"}
            }" \
            --endpoint-url http://localhost:4566 \
            --region us-east-1 2>/dev/null
        echo -n "."
    done
    echo ""

    log_step "Reiniciar sync service"
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" restart sync > /dev/null 2>&1
    sleep 3

    log_step "Verificar en MySQL"
    RESULT=$(mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
        "SELECT COUNT(*) FROM monitoreo_produccion_temporal WHERE folio LIKE 'TEST-BATCH-%';" 2>/dev/null | tail -1)

    if [ "$RESULT" -ge "5" ]; then
        log_success "$RESULT producciones sincronizadas"
        return 0
    else
        log_error "Solo se sincronizaron $RESULT de 5 producciones"
        return 1
    fi
}

###############################################################################
# TEST 3: ESCENAS
###############################################################################

test_scenes_sync() {
    log_test "Sincronización de Escenas"

    log_step "Insertar escenas en DynamoDB para producción 501"

    for i in {1..3}; do
        SCENE_ID="SCENE-501-$(printf '%03d' $i)"
        aws dynamodb put-item \
            --table-name monitoring_escenas \
            --item "{
              \"produccion_id\": {\"N\": \"501\"},
              \"scene_id\": {\"S\": \"$SCENE_ID\"},
              \"satellite\": {\"S\": \"Sentinel-2\"},
              \"fecha_captura\": {\"S\": \"2026-09-04T$(printf '%02d' $((10 + i*2))):00:00Z\"},
              \"fecha_procesamiento\": {\"S\": \"2026-09-04T12:00:00Z\"},
              \"cloud_coverage\": {\"N\": \"$((5 + i * 3))\"},
              \"s3_key\": {\"S\": \"scenes/501/$SCENE_ID.tif\"},
              \"status\": {\"S\": \"processed\"}
            }" \
            --endpoint-url http://localhost:4566 \
            --region us-east-1 2>/dev/null
        echo -n "."
    done
    echo ""

    log_step "Reiniciar sync service"
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" restart sync > /dev/null 2>&1
    sleep 3

    log_step "Verificar en MySQL"
    RESULT=$(mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
        "SELECT COUNT(*) FROM monitoreo_escenas WHERE produccion_id = 501;" 2>/dev/null | tail -1)

    if [ "$RESULT" -ge "3" ]; then
        log_success "$RESULT escenas sincronizadas"
        return 0
    else
        log_error "Solo se sincronizaron $RESULT de 3 escenas"
        return 1
    fi
}

###############################################################################
# TEST 4: CAMBIOS DE ESTADO
###############################################################################

test_status_updates() {
    log_test "Actualización de Estados"

    log_step "Cambiar estado en DynamoDB"
    aws dynamodb update-item \
        --table-name monitoring_producciones \
        --key '{"produccion_id": {"N": "501"}}' \
        --update-expression "SET #s = :status" \
        --expression-attribute-names '{"#s": "status"}' \
        --expression-attribute-values '{":status": {"S": "completed"}}' \
        --endpoint-url http://localhost:4566 \
        --region us-east-1 2>/dev/null

    log_step "Reiniciar sync service"
    docker-compose -f "$PROJECT_ROOT/docker-compose.yml" restart sync > /dev/null 2>&1
    sleep 3

    log_step "Verificar estado en MySQL"
    RESULT=$(mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
        "SELECT status FROM monitoreo_produccion_temporal WHERE produccion_id = 501;" 2>/dev/null | tail -1)

    if [ "$RESULT" = "completed" ]; then
        log_success "Estado actualizado a '$RESULT'"
        return 0
    else
        log_error "Estado no se actualizó correctamente (Resultado: $RESULT)"
        return 1
    fi
}

###############################################################################
# TEST 5: VERIFICACION DE LOGS
###############################################################################

test_logs_verification() {
    log_test "Verificación de Logs del Sync Service"

    log_step "Verificar logs del sync"
    LOGS=$(docker-compose -f "$PROJECT_ROOT/docker-compose.yml" logs sync 2>/dev/null | tail -20)

    if echo "$LOGS" | grep -qi "error"; then
        log_error "Se encontraron errores en logs del sync"
        echo "$LOGS" | grep -i error
        return 1
    fi

    log_step "Buscar mensajes de sincronización"
    if echo "$LOGS" | grep -qi "sync\|processing"; then
        log_success "Logs de sincronización encontrados"
        return 0
    else
        log_error "No se encontraron mensajes de sincronización en logs"
        return 1
    fi
}

###############################################################################
# UTILIDADES DE VERIFICACION
###############################################################################

verify_services_running() {
    log_info "Verificando que los servicios están corriendo..."

    # Verificar MySQL
    if ! mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SELECT 1;" 2>/dev/null > /dev/null; then
        log_error "MySQL no está respondiendo"
        return 1
    fi

    # Verificar LocalStack
    if ! curl -s http://localhost:4566/health > /dev/null 2>&1; then
        log_error "LocalStack no está respondiendo"
        return 1
    fi

    # Verificar Docker containers
    if ! docker-compose -f "$PROJECT_ROOT/docker-compose.yml" ps | grep -q "sync"; then
        log_error "Sync service no está corriendo"
        return 1
    fi

    log_info "Todos los servicios están activos"
    return 0
}

cleanup_test_data() {
    log_info "Limpiando datos de prueba..."

    # Limpiar producciones de prueba en MySQL
    mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
        "DELETE FROM monitoreo_produccion_temporal WHERE folio LIKE 'TEST-%';" 2>/dev/null || true

    # Limpiar escenas de prueba en MySQL
    mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
        "DELETE FROM monitoreo_escenas WHERE produccion_id >= 500;" 2>/dev/null || true

    # Limpiar producciones de prueba en DynamoDB
    for i in 501 502 503 504 505 506; do
        aws dynamodb delete-item \
            --table-name monitoring_producciones \
            --key "{\"produccion_id\": {\"N\": \"$i\"}}" \
            --endpoint-url http://localhost:4566 \
            --region us-east-1 2>/dev/null || true
    done

    log_info "Datos de prueba limpiados"
}

###############################################################################
# MAIN
###############################################################################

main() {
    log_header "TEST SYNC FLOW - AGRO SENTINEL WORKER"

    # Verificar servicios
    if ! verify_services_running; then
        log_error "Los servicios no están activos. Ejecuta:"
        log_info "  docker-compose up -d"
        exit 1
    fi

    # Ejecutar tests
    test_basic_sync || true
    test_multiple_sync || true
    test_scenes_sync || true
    test_status_updates || true
    test_logs_verification || true

    # Resumen
    log_header "RESUMEN DE TESTING"
    echo -e "${GREEN}Tests Pasados: $TESTS_PASSED/$TESTS_TOTAL${NC}"

    if [ $TESTS_PASSED -eq $TESTS_TOTAL ]; then
        echo -e "${GREEN}═════════════════════════════════════════════════════════════════${NC}"
        echo -e "${GREEN}RESULTADO: TODOS LOS TESTS PASARON${NC}"
        echo -e "${GREEN}═════════════════════════════════════════════════════════════════${NC}"
    else
        TESTS_FAILED=$((TESTS_TOTAL - TESTS_PASSED))
        echo -e "${YELLOW}═════════════════════════════════════════════════════════════════${NC}"
        echo -e "${YELLOW}RESULTADO: $TESTS_FAILED de $TESTS_TOTAL tests FALLARON${NC}"
        echo -e "${YELLOW}═════════════════════════════════════════════════════════════════${NC}"
    fi

    # Limpiar datos
    echo -e "\n"
    read -p "¿Deseas limpiar los datos de prueba? (s/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Ss]$ ]]; then
        cleanup_test_data
    fi

    echo -e "\n"
}

# Ejecutar main
main "$@"
