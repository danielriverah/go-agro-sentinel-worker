#!/bin/bash

# Script para ejecutar los tests de la Fase 3 del Sistema Agro Sentinel
# Uso: ./scripts/run_tests.sh [opción]

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_header() {
    echo -e "${BLUE}===================================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}===================================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

# Verificar MYSQL_TEST_DSN
check_db_env() {
    if [ -z "$MYSQL_TEST_DSN" ]; then
        print_warning "MYSQL_TEST_DSN not set - database tests will be skipped"
        echo "To enable database tests, set the environment variable:"
        echo "  export MYSQL_TEST_DSN=\"root:password@tcp(localhost:3306)/sentinel_test?parseTime=true\""
    else
        print_success "MYSQL_TEST_DSN is set: ${MYSQL_TEST_DSN:0:50}..."
    fi
}

# Ejecutar tests específicos
run_domain_tests() {
    print_header "Running Domain Tests (Structs & Validation)"
    if go test -v ./internal/domain -run "TestProduction|TestIAResult|TestBBox"; then
        print_success "Domain tests passed"
    else
        print_error "Domain tests failed"
        return 1
    fi
}

run_production_repo_tests() {
    print_header "Running ProductionRepo Tests"
    if go test -v ./internal/infrastructure/database -run "TestProductionRepo"; then
        print_success "ProductionRepo tests passed"
    else
        print_error "ProductionRepo tests failed"
        return 1
    fi
}

run_ia_result_repo_tests() {
    print_header "Running IAResultRepository Tests"
    if go test -v ./internal/infrastructure/database -run "TestIAResultRepository"; then
        print_success "IAResultRepository tests passed"
    else
        print_error "IAResultRepository tests failed"
        return 1
    fi
}

run_integration_tests() {
    print_header "Running Integration Tests"
    if go test -v ./internal/infrastructure/database -run "TestIntegration"; then
        print_success "Integration tests passed"
    else
        print_error "Integration tests failed"
        return 1
    fi
}

run_all_database_tests() {
    print_header "Running All Database Tests"
    if go test -v ./internal/infrastructure/database; then
        print_success "All database tests passed"
    else
        print_error "Some database tests failed"
        return 1
    fi
}

run_all_tests() {
    print_header "Running ALL Tests"
    if go test -v ./...; then
        print_success "All tests passed"
    else
        print_error "Some tests failed"
        return 1
    fi
}

run_with_coverage() {
    print_header "Running Tests with Coverage"
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    print_success "Coverage report generated: coverage.html"
}

run_with_race() {
    print_header "Running Tests with Race Detector"
    go test -race ./...
    print_success "Race detector passed"
}

# Main
case "${1:-all}" in
    domain)
        check_db_env
        run_domain_tests
        ;;
    production)
        check_db_env
        run_production_repo_tests
        ;;
    ia-result)
        check_db_env
        run_ia_result_repo_tests
        ;;
    integration)
        check_db_env
        run_integration_tests
        ;;
    database)
        check_db_env
        run_all_database_tests
        ;;
    coverage)
        check_db_env
        run_with_coverage
        ;;
    race)
        check_db_env
        run_with_race
        ;;
    all)
        check_db_env
        run_domain_tests && \
        run_production_repo_tests && \
        run_ia_result_repo_tests && \
        run_integration_tests
        print_success "All Phase 3 tests completed successfully!"
        ;;
    *)
        echo "Usage: $0 [domain|production|ia-result|integration|database|coverage|race|all]"
        echo ""
        echo "Options:"
        echo "  domain        - Run domain struct and validation tests"
        echo "  production    - Run ProductionRepo tests"
        echo "  ia-result     - Run IAResultRepository tests"
        echo "  integration   - Run integration tests"
        echo "  database      - Run all database tests"
        echo "  coverage      - Run tests with coverage report"
        echo "  race          - Run tests with race detector"
        echo "  all           - Run all Phase 3 tests (default)"
        exit 1
        ;;
esac

echo ""
print_success "Test execution completed"
