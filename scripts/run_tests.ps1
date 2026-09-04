#!/usr/bin/env pwsh
# Script para ejecutar los tests de la Fase 3 del Sistema Agro Sentinel
# Uso: .\scripts\run_tests.ps1 [-TestType <domain|production|ia-result|integration|database|coverage|all>]

param(
    [ValidateSet('domain', 'production', 'ia-result', 'integration', 'database', 'coverage', 'race', 'all')]
    [string]$TestType = 'all'
)

$ErrorActionPreference = 'Stop'

# Determinar el directorio raíz del proyecto
$ProjectRoot = Split-Path -Parent $PSScriptRoot

Set-Location $ProjectRoot

# Funciones de utilidad
function Write-Header {
    param([string]$Message)
    Write-Host ""
    Write-Host "===================================================" -ForegroundColor Blue
    Write-Host $Message -ForegroundColor Blue
    Write-Host "===================================================" -ForegroundColor Blue
    Write-Host ""
}

function Write-Success {
    param([string]$Message)
    Write-Host "✓ $Message" -ForegroundColor Green
}

function Write-Error {
    param([string]$Message)
    Write-Host "✗ $Message" -ForegroundColor Red
}

function Write-Warning {
    param([string]$Message)
    Write-Host "⚠ $Message" -ForegroundColor Yellow
}

# Verificar MYSQL_TEST_DSN
function Check-DbEnv {
    if ([string]::IsNullOrEmpty($env:MYSQL_TEST_DSN)) {
        Write-Warning "MYSQL_TEST_DSN not set - database tests will be skipped"
        Write-Host "To enable database tests, set the environment variable:"
        Write-Host "  `$env:MYSQL_TEST_DSN = 'root:password@tcp(localhost:3306)/sentinel_test?parseTime=true'"
    }
    else {
        Write-Success "MYSQL_TEST_DSN is set: $($env:MYSQL_TEST_DSN.Substring(0, [Math]::Min(50, $env:MYSQL_TEST_DSN.Length)))..."
    }
}

# Ejecutar tests específicos
function Run-DomainTests {
    Write-Header "Running Domain Tests (Structs & Validation)"

    $result = & go test -v ./internal/domain -run "TestProduction|TestIAResult|TestBBox"

    if ($LASTEXITCODE -eq 0) {
        Write-Success "Domain tests passed"
        return $true
    }
    else {
        Write-Error "Domain tests failed"
        return $false
    }
}

function Run-ProductionRepoTests {
    Write-Header "Running ProductionRepo Tests"

    $result = & go test -v ./internal/infrastructure/database -run "TestProductionRepo"

    if ($LASTEXITCODE -eq 0) {
        Write-Success "ProductionRepo tests passed"
        return $true
    }
    else {
        Write-Error "ProductionRepo tests failed"
        return $false
    }
}

function Run-IAResultRepoTests {
    Write-Header "Running IAResultRepository Tests"

    $result = & go test -v ./internal/infrastructure/database -run "TestIAResultRepository"

    if ($LASTEXITCODE -eq 0) {
        Write-Success "IAResultRepository tests passed"
        return $true
    }
    else {
        Write-Error "IAResultRepository tests failed"
        return $false
    }
}

function Run-IntegrationTests {
    Write-Header "Running Integration Tests"

    $result = & go test -v ./internal/infrastructure/database -run "TestIntegration"

    if ($LASTEXITCODE -eq 0) {
        Write-Success "Integration tests passed"
        return $true
    }
    else {
        Write-Error "Integration tests failed"
        return $false
    }
}

function Run-AllDatabaseTests {
    Write-Header "Running All Database Tests"

    $result = & go test -v ./internal/infrastructure/database

    if ($LASTEXITCODE -eq 0) {
        Write-Success "All database tests passed"
        return $true
    }
    else {
        Write-Error "Some database tests failed"
        return $false
    }
}

function Run-AllTests {
    Write-Header "Running ALL Tests"

    $result = & go test -v ./...

    if ($LASTEXITCODE -eq 0) {
        Write-Success "All tests passed"
        return $true
    }
    else {
        Write-Error "Some tests failed"
        return $false
    }
}

function Run-WithCoverage {
    Write-Header "Running Tests with Coverage"

    & go test -coverprofile=coverage.out ./...
    & go tool cover -html=coverage.out -o coverage.html

    Write-Success "Coverage report generated: coverage.html"
}

function Run-WithRace {
    Write-Header "Running Tests with Race Detector"

    $result = & go test -race ./...

    if ($LASTEXITCODE -eq 0) {
        Write-Success "Race detector passed"
        return $true
    }
    else {
        Write-Error "Race detector found issues"
        return $false
    }
}

# Main
Write-Host "Phase 3 Test Runner - Sistema Agro Sentinel" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan

Check-DbEnv

$success = $true

switch ($TestType) {
    'domain' {
        $success = Run-DomainTests
    }
    'production' {
        $success = Run-ProductionRepoTests
    }
    'ia-result' {
        $success = Run-IAResultRepoTests
    }
    'integration' {
        $success = Run-IntegrationTests
    }
    'database' {
        $success = Run-AllDatabaseTests
    }
    'coverage' {
        Run-WithCoverage
    }
    'race' {
        $success = Run-WithRace
    }
    'all' {
        $success = $true
        $success = (Run-DomainTests) -and $success
        $success = (Run-ProductionRepoTests) -and $success
        $success = (Run-IAResultRepoTests) -and $success
        $success = (Run-IntegrationTests) -and $success

        if ($success) {
            Write-Success "All Phase 3 tests completed successfully!"
        }
    }
}

Write-Host ""
if ($success) {
    Write-Success "Test execution completed"
    exit 0
}
else {
    Write-Error "Test execution had failures"
    exit 1
}
