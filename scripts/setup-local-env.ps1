# Setup Local Environment - Agro Sentinel Worker
# Inicializa el entorno local completo: Docker, MySQL, DynamoDB, S3

param(
    [switch]$SkipCleanup = $false,
    [switch]$SkipBuild = $false,
    [switch]$LoadTestData = $false
)

# Variables de control
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$ChecksPassed = 0
$ChecksTotal = 0

# Colores
$ColorReset = "`e[0m"
$ColorRed = "`e[31m"
$ColorGreen = "`e[32m"
$ColorYellow = "`e[33m"
$ColorBlue = "`e[34m"

###############################################################################
# FUNCIONES UTILITARIAS
###############################################################################

function Write-Header {
    param([string]$Message)
    Write-Host ""
    Write-Host "═════════════════════════════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host $Message -ForegroundColor Cyan
    Write-Host "═════════════════════════════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""
}

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Blue
}

function Write-Success {
    param([string]$Message)
    Write-Host "[✓] $Message" -ForegroundColor Green
    $script:ChecksPassed++
}

function Write-Error-Custom {
    param([string]$Message)
    Write-Host "[✗] $Message" -ForegroundColor Red
}

function Write-Warning-Custom {
    param([string]$Message)
    Write-Host "[!] $Message" -ForegroundColor Yellow
}

function Check-Command {
    param(
        [string]$Command,
        [string]$Name
    )
    $script:ChecksTotal++

    try {
        $null = & $Command --version 2>&1
        Write-Success "$Name instalado"
        return $true
    }
    catch {
        Write-Error-Custom "$Name no instalado"
        return $false
    }
}

###############################################################################
# PASO 1: VERIFICAR REQUISITOS
###############################################################################

function Verify-Requirements {
    Write-Header "PASO 1: Verificar Requisitos"

    Check-Command "docker" "Docker"
    Check-Command "docker-compose" "Docker Compose"
    Check-Command "git" "Git"

    if (-not (Check-Command "aws" "AWS CLI")) {
        Write-Warning-Custom "AWS CLI no instalado. Algunas operaciones pueden fallar."
        Write-Info "Instala con: pip install awscli"
    }

    if (-not (Check-Command "mysql" "MySQL Client")) {
        Write-Warning-Custom "MySQL Client no instalado. Algunas operaciones pueden fallar."
        Write-Info "Instala con: choco install mysql-cli o descarga desde https://dev.mysql.com/"
    }

    # Verificar Docker daemon
    try {
        $null = docker ps 2>&1
        Write-Success "Docker daemon está corriendo"
    }
    catch {
        Write-Error-Custom "Docker daemon no está corriendo"
        Write-Info "Inicia Docker Desktop y vuelve a intentar"
        exit 1
    }
}

###############################################################################
# PASO 2: LIMPIAR ESTADO ANTERIOR
###############################################################################

function Cleanup-Docker {
    if ($SkipCleanup) {
        Write-Info "Limpieza saltada (opción -SkipCleanup)"
        return
    }

    Write-Header "PASO 2: Limpiar Estado Anterior"

    $response = Read-Host "¿Deseas limpiar contenedores anteriores? (s/n)"
    if ($response -ne "s") {
        Write-Info "Limpieza cancelada"
        return
    }

    Write-Info "Deteniendo contenedores..."
    & docker-compose -f "$ProjectRoot/docker-compose.yml" down 2>&1 | Out-Null

    $response = Read-Host "¿Eliminar volúmenes? (borrará datos) (s/n)"
    if ($response -eq "s") {
        & docker-compose -f "$ProjectRoot/docker-compose.yml" down -v 2>&1 | Out-Null
        Write-Success "Volúmenes eliminados"
    }
}

###############################################################################
# PASO 3: CONSTRUIR IMÁGENES
###############################################################################

function Build-Images {
    if ($SkipBuild) {
        Write-Info "Build saltado (opción -SkipBuild)"
        return
    }

    Write-Header "PASO 3: Construir Imágenes Docker"

    Write-Info "Construyendo imágenes (esto puede tomar 2-5 minutos)..."

    if (-not (& docker-compose -f "$ProjectRoot/docker-compose.yml" build 2>&1)) {
        Write-Error-Custom "Error construyendo imágenes"
        exit 1
    }

    Write-Success "Imágenes construidas correctamente"
}

###############################################################################
# PASO 4: INICIAR SERVICIOS
###############################################################################

function Start-Services {
    Write-Header "PASO 4: Iniciar Servicios Docker"

    Write-Info "Iniciando MySQL..."
    & docker-compose -f "$ProjectRoot/docker-compose.yml" up -d mysql | Out-Null

    Write-Info "Esperando MySQL health check (hasta 30 segundos)..."
    $healthy = $false
    for ($i = 0; $i -lt 30; $i++) {
        $output = & docker-compose -f "$ProjectRoot/docker-compose.yml" ps mysql 2>&1
        if ($output -match "healthy") {
            Write-Success "MySQL está healthy"
            $healthy = $true
            break
        }
        Write-Host -NoNewline "."
        Start-Sleep -Seconds 1
    }
    Write-Host ""

    if (-not $healthy) {
        Write-Warning-Custom "MySQL no alcanzó estado healthy en 30 segundos"
    }

    Write-Info "Iniciando LocalStack..."
    & docker-compose -f "$ProjectRoot/docker-compose.yml" up -d localstack | Out-Null
    Start-Sleep -Seconds 5
    Write-Success "LocalStack iniciado"

    Write-Info "Iniciando API..."
    & docker-compose -f "$ProjectRoot/docker-compose.yml" up -d api | Out-Null
    Start-Sleep -Seconds 2
    Write-Success "API iniciado"

    Write-Info "Iniciando Worker..."
    & docker-compose -f "$ProjectRoot/docker-compose.yml" up -d worker | Out-Null
    Start-Sleep -Seconds 2
    Write-Success "Worker iniciado"

    Write-Info "Iniciando Sync Service..."
    & docker-compose -f "$ProjectRoot/docker-compose.yml" up -d sync | Out-Null
    Start-Sleep -Seconds 2
    Write-Success "Sync Service iniciado"

    Write-Info "Verificando estado de todos los servicios..."
    & docker-compose -f "$ProjectRoot/docker-compose.yml" ps
}

###############################################################################
# PASO 5: VERIFICAR TABLAS MySQL
###############################################################################

function Verify-MySQL-Tables {
    Write-Header "PASO 5: Verificar Tablas MySQL"

    Start-Sleep -Seconds 3

    Write-Info "Verificando conexión a MySQL..."
    try {
        $output = & mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SELECT VERSION();" 2>&1
        Write-Success "MySQL accesible"
    }
    catch {
        Write-Error-Custom "No se puede conectar a MySQL"
        Write-Info "Error: $_"
        return
    }

    Write-Info "Verificando tablas..."
    try {
        $output = & mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SHOW TABLES;" 2>&1
        $tableCount = $output.Count - 1
        if ($tableCount -gt 0) {
            Write-Success "MySQL contiene $tableCount tablas"
        }
        else {
            Write-Warning-Custom "No se encontraron tablas"
        }
    }
    catch {
        Write-Warning-Custom "No se pudo contar tablas"
    }
}

###############################################################################
# PASO 6: VERIFICAR DYNAMODB
###############################################################################

function Verify-DynamoDB {
    Write-Header "PASO 6: Verificar DynamoDB (LocalStack)"

    Write-Info "Verificando tablas DynamoDB..."

    try {
        $output = & aws dynamodb list-tables `
            --endpoint-url http://localhost:4566 `
            --region us-east-1 2>&1 | ConvertFrom-Json

        if ($output.TableNames -match "monitoring") {
            Write-Success "Encontradas tablas DynamoDB"
        }
        else {
            Write-Warning-Custom "No se encontraron tablas DynamoDB"
        }
    }
    catch {
        Write-Warning-Custom "No se puede conectar a DynamoDB (AWS CLI puede no estar instalado)"
    }
}

###############################################################################
# PASO 7: VERIFICAR S3
###############################################################################

function Verify-S3 {
    Write-Header "PASO 7: Verificar S3 (LocalStack)"

    Write-Info "Verificando buckets S3..."

    try {
        $output = & aws s3 ls `
            --endpoint-url http://localhost:4566 `
            --region us-east-1 2>&1

        if ($output -match "agro-sentinel-bucket") {
            Write-Success "Bucket S3 encontrado (agro-sentinel-bucket)"
        }
        else {
            Write-Warning-Custom "Bucket S3 no encontrado"
        }
    }
    catch {
        Write-Warning-Custom "No se puede conectar a S3 (AWS CLI puede no estar instalado)"
    }
}

###############################################################################
# PASO 8: CARGAR DATOS DE PRUEBA
###############################################################################

function Load-TestData {
    if (-not $LoadTestData) {
        $response = Read-Host "¿Deseas cargar datos de prueba? (s/n)"
        if ($response -ne "s") {
            Write-Info "Carga de datos cancelada"
            return
        }
    }

    Write-Header "PASO 8: Cargar Datos de Prueba"

    Write-Info "Insertando datos en MySQL..."

    $sqlScript = @"
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
"@

    try {
        $sqlScript | & mysql -h 127.0.0.1 -P 3309 -u root -proot agro 2>&1 | Out-Null
        Write-Success "Datos insertados en MySQL"
    }
    catch {
        Write-Warning-Custom "Error insertando datos (pueden ya existir): $_"
    }

    Write-Info "Insertando datos en DynamoDB (si AWS CLI está disponible)..."

    try {
        $itemJson = @{
            "produccion_id" = @{ "N" = "1" }
            "folio"         = @{ "S" = "PROD-001" }
            "status"        = @{ "S" = "monitoring" }
            "cantidad_escenas" = @{ "N" = "0" }
        } | ConvertTo-Json -Depth 10

        & aws dynamodb put-item `
            --table-name monitoring_producciones `
            --item $itemJson `
            --endpoint-url http://localhost:4566 `
            --region us-east-1 2>&1 | Out-Null

        Write-Success "Datos insertados en DynamoDB"
    }
    catch {
        Write-Warning-Custom "Error insertando en DynamoDB: $_"
    }
}

###############################################################################
# PASO 9: VERIFICAR CONECTIVIDAD
###############################################################################

function Verify-Connectivity {
    Write-Header "PASO 9: Verificar Conectividad de Servicios"

    Write-Info "Verificando API health endpoint..."
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8088/health" -TimeoutSec 5 -ErrorAction Stop
        if ($response.Content -match "healthy") {
            Write-Success "API respondiendo correctamente"
        }
        else {
            Write-Warning-Custom "API respondiendo pero respuesta inesperada"
        }
    }
    catch {
        Write-Warning-Custom "No se pudo alcanzar API en http://localhost:8088/health"
    }
}

###############################################################################
# PASO 10: RESUMEN
###############################################################################

function Print-Summary {
    Write-Header "RESUMEN DE SETUP"

    Write-Success "Setup completado"
    Write-Host "Servicios disponibles:" -ForegroundColor Green
    Write-Host "  MySQL       → Host: 127.0.0.1, Puerto: 3309, User: root, Pass: root"
    Write-Host "  LocalStack  → http://localhost:4566 (S3, DynamoDB, SQS)"
    Write-Host "  API         → http://localhost:8088"
    Write-Host "  Worker      → Procesando jobs de SQS"
    Write-Host "  Sync        → Sincronizando DynamoDB → MySQL"

    Write-Host ""
    Write-Host "Próximos pasos:" -ForegroundColor Green
    Write-Host "  1. Revisar logs: docker-compose logs -f"
    Write-Host "  2. Testing: .\scripts\test-sync-flow.ps1"
    Write-Host "  3. Guía: Get-Content SYNC_TESTING_GUIDE.md"

    Write-Host ""
    Write-Host "═════════════════════════════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""
}

###############################################################################
# MAIN
###############################################################################

function Main {
    Write-Header "SETUP LOCAL ENVIRONMENT - AGRO SENTINEL WORKER"

    Verify-Requirements
    Cleanup-Docker
    Build-Images
    Start-Services

    # Dar tiempo a los servicios a iniciarse completamente
    Write-Info "Esperando a que los servicios se estabilicen (30 segundos)..."
    for ($i = 0; $i -lt 30; $i++) {
        Write-Host -NoNewline "."
        Start-Sleep -Seconds 1
    }
    Write-Host ""

    Verify-MySQL-Tables
    Verify-DynamoDB
    Verify-S3
    Load-TestData
    Verify-Connectivity
    Print-Summary
}

# Ejecutar main
Main
