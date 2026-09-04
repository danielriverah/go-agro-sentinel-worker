# =====================================================
# SCRIPT: Utilidad para importar datos desde CSV
# Sistema Agro Sentinel - PowerShell Edition
# =====================================================
# Descripción: Script PowerShell para facilitar la importación
# de archivos CSV a la base de datos MySQL
#
# Uso: .\import-csv.ps1 -Comando Import -Tabla articulos -Archivo data/csv/examples/articulos.csv
# Ejemplos:
#   .\import-csv.ps1 -Comando Help
#   .\import-csv.ps1 -Comando ImportAll
#   .\import-csv.ps1 -Comando ListTables
# =====================================================

param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("Import", "ImportAll", "ListTables", "Validate", "Backup", "Help")]
    [string]$Comando,

    [Parameter(Mandatory=$false)]
    [string]$Tabla,

    [Parameter(Mandatory=$false)]
    [string]$Archivo,

    [Parameter(Mandatory=$false)]
    [string]$DBHost = "localhost",

    [Parameter(Mandatory=$false)]
    [string]$DBUser = "admin",

    [Parameter(Mandatory=$false)]
    [string]$DBPass = "admin",

    [Parameter(Mandatory=$false)]
    [string]$DBName = "agro",

    [Parameter(Mandatory=$false)]
    [int]$DBPort = 3306
)

# =====================================================
# FUNCIONES
# =====================================================

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "[OK] $Message" -ForegroundColor Green
}

function Write-Warning {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
}

function Write-Error {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

function Show-Help {
    $helpText = @"
=== UTILIDAD DE IMPORTACIÓN CSV - AGRO SENTINEL ===

Uso:
    .\import-csv.ps1 -Comando <comando> [opciones]

Comandos:
    Import      Importar CSV a una tabla específica
    ImportAll   Importar todos los CSV de ejemplo
    ListTables  Listar tablas disponibles
    Validate    Validar estructura de un CSV
    Backup      Crear backup de las tablas
    Help        Mostrar esta ayuda

Parámetros comunes:
    -Comando    Comando a ejecutar (requerido)
    -Tabla      Tabla de destino (para comando Import)
    -Archivo    Ruta del archivo CSV
    -DBHost     Host de MySQL (default: localhost)
    -DBUser     Usuario MySQL (default: admin)
    -DBPass     Contraseña MySQL (default: admin)
    -DBName     Base de datos (default: agro)
    -DBPort     Puerto MySQL (default: 3306)

Tablas soportadas:
    - articulos
    - centros_costos
    - producciones
    - zonas_producciones
    - asignaciones_zonas_producciones
    - s3_monitoring_escena_ia_resumen

Ejemplos:
    .\import-csv.ps1 -Comando ListTables
    .\import-csv.ps1 -Comando ImportAll
    .\import-csv.ps1 -Comando Import -Tabla articulos -Archivo data/csv/examples/articulos.csv
    .\import-csv.ps1 -Comando Validate -Archivo data/csv/examples/articulos.csv
    .\import-csv.ps1 -Comando Backup -DBHost 192.168.1.100 -DBUser admin -DBPass pass123

"@
    Write-Host $helpText
}

function Test-DBConnection {
    param(
        [string]$Host,
        [string]$User,
        [string]$Password,
        [string]$Database,
        [int]$Port
    )

    Write-Info "Verificando conexión a la base de datos..."

    try {
        $connectionString = "Server=$Host;Uid=$User;Pwd=$Password;Database=$Database;Port=$Port;"
        $connection = New-Object MySql.Data.MySqlClient.MySqlConnection
        $connection.ConnectionString = $connectionString
        $connection.Open()
        $connection.Close()
        Write-Success "Conexión verificada"
        return $true
    }
    catch {
        Write-Error "No se puede conectar: $($_.Exception.Message)"
        Write-Error "Host: $Host, Usuario: $User, BD: $Database, Puerto: $Port"
        return $false
    }
}

function Test-FileExists {
    param([string]$FilePath)

    if (-Not (Test-Path $FilePath)) {
        Write-Error "El archivo no existe: $FilePath"
        return $false
    }
    Write-Success "Archivo encontrado: $FilePath"
    return $true
}

function Import-Table {
    param(
        [string]$TableName,
        [string]$FilePath,
        [string]$Host,
        [string]$User,
        [string]$Password,
        [string]$Database,
        [int]$Port
    )

    Write-Info "Iniciando importación de tabla: $TableName"
    Write-Info "Archivo: $FilePath"

    if (-Not (Test-FileExists $FilePath)) {
        return $false
    }

    if (-Not (Test-DBConnection $Host $User $Password $Database $Port)) {
        return $false
    }

    # Obtener ruta absoluta
    $absolutePath = (Resolve-Path $FilePath).Path
    $absolutePath = $absolutePath -replace '\\', '/'  # Convertir a forward slashes para MySQL

    # Crear el comando SQL según la tabla
    $sqlCommand = switch ($TableName) {
        "articulos" {
            @"
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '$absolutePath'
INTO TABLE articulos
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(articulo_id, nombre, variedad);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM articulos;
"@
        }
        "centros_costos" {
            @"
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '$absolutePath'
INTO TABLE centros_costos
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(centro_costo_id, nombre);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM centros_costos;
"@
        }
        "producciones" {
            @"
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '$absolutePath'
INTO TABLE producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(folio, articulo_id, fecha, hora, fecha_cierre, hora_cierre, usuario, estatus, aplicado, cantidad, centro_costo_id, monitoring);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM producciones;
"@
        }
        "zonas_producciones" {
            @"
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '$absolutePath'
INTO TABLE zonas_producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(zona_produccion_id, nombre, nombre_corto, estatus, area, centro_costo_id, poligono);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM zonas_producciones;
"@
        }
        "asignaciones_zonas_producciones" {
            @"
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '$absolutePath'
INTO TABLE asignaciones_zonas_producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(asignacion_zona_prod_id, produccion_id, zona_produccion_id, tipo_asignacion, area, poligono);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM asignaciones_zonas_producciones;
"@
        }
        "s3_monitoring_escena_ia_resumen" {
            @"
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '$absolutePath'
INTO TABLE s3_monitoring_escena_ia_resumen
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(s3_monitoring_escena_id, estado_clave, estado_general, riesgo_nivel, riesgo_motivo, fecha_analisis, json_original);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM s3_monitoring_escena_ia_resumen;
"@
        }
        default {
            Write-Error "Tabla no soportada: $TableName"
            return $false
        }
    }

    # Guardar SQL en archivo temporal
    $tempSql = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "import_$TableName.sql")
    $sqlCommand | Out-File -FilePath $tempSql -Encoding UTF8 -Force

    try {
        Write-Info "Ejecutando importación..."
        $output = & mysql -h $Host -P $Port -u $User -p"$Password" $Database --local-infile=1 < $tempSql

        if ($LASTEXITCODE -eq 0) {
            Write-Success "Importación de $TableName completada"
            if ($output) {
                Write-Host $output
            }
            Remove-Item $tempSql -Force
            return $true
        }
        else {
            Write-Error "Error durante la importación"
            Remove-Item $tempSql -Force
            return $false
        }
    }
    catch {
        Write-Error "Excepción: $($_.Exception.Message)"
        Remove-Item $tempSql -Force -ErrorAction SilentlyContinue
        return $false
    }
}

function Import-All {
    Write-Info "Importando todos los archivos CSV..."

    $tablas = @(
        "articulos",
        "centros_costos",
        "zonas_producciones",
        "producciones",
        "asignaciones_zonas_producciones",
        "s3_monitoring_escena_ia_resumen"
    )

    $baseDir = "data/csv/examples"
    $count = 0

    foreach ($tabla in $tablas) {
        $archivo = "$baseDir\$tabla.csv"
        Write-Info "Procesando: $tabla"

        if (Import-Table $tabla $archivo $DBHost $DBUser $DBPass $DBName $DBPort) {
            $count++
        }
        else {
            Write-Warning "Error importando $tabla"
        }

        Write-Host ""
    }

    Write-Success "Importación completada: $count/$($tablas.Count) tablas"
}

function List-Tables {
    Write-Info "Listando tablas disponibles..."

    if (-Not (Test-DBConnection $DBHost $DBUser $DBPass $DBName $DBPort)) {
        return
    }

    $query = "SELECT TABLE_NAME as 'Tabla', TABLE_ROWS as 'Registros' FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA='$DBName' ORDER BY TABLE_NAME;"

    $tempSql = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "list_tables.sql")
    $query | Out-File -FilePath $tempSql -Encoding UTF8 -Force

    try {
        $output = & mysql -h $DBHost -P $DBPort -u $DBUser -p"$DBPass" $DBName < $tempSql
        Write-Host $output
        Remove-Item $tempSql -Force
    }
    catch {
        Write-Error "Error: $($_.Exception.Message)"
        Remove-Item $tempSql -Force -ErrorAction SilentlyContinue
    }
}

function Validate-CSV {
    param([string]$FilePath)

    Write-Info "Validando archivo CSV: $FilePath"

    if (-Not (Test-FileExists $FilePath)) {
        return
    }

    $content = Get-Content $FilePath
    $totalLines = @($content).Count
    $dataLines = $totalLines - 1

    Write-Success "Archivo válido"
    Write-Host "  Línea de encabezado: 1"
    Write-Host "  Líneas de datos: $dataLines"
    Write-Host "  Total de líneas: $totalLines"
    Write-Host ""

    Write-Info "Primeras 3 líneas del archivo:"
    $content | Select-Object -First 3 | ForEach-Object {
        Write-Host "  $_"
    }
}

function Backup-Database {
    Write-Info "Creando backup de la base de datos..."

    $backupDate = Get-Date -Format "yyyyMMdd_HHmmss"
    $backupDir = "backups"
    $backupFile = Join-Path $backupDir "agro_backup_$backupDate.sql"

    if (-Not (Test-Path $backupDir)) {
        New-Item -ItemType Directory -Path $backupDir -Force | Out-Null
    }

    try {
        & mysqldump -h $DBHost -P $DBPort -u $DBUser -p"$DBPass" $DBName | Out-File -FilePath $backupFile -Encoding UTF8 -Force

        if ($LASTEXITCODE -eq 0) {
            Write-Success "Backup creado: $backupFile"
        }
        else {
            Write-Error "Error al crear el backup"
        }
    }
    catch {
        Write-Error "Excepción: $($_.Exception.Message)"
    }
}

# =====================================================
# MAIN
# =====================================================

switch ($Comando) {
    "Help" {
        Show-Help
    }
    "Import" {
        if (-Not $Tabla -or -Not $Archivo) {
            Write-Error "Uso: .\import-csv.ps1 -Comando Import -Tabla <tabla> -Archivo <archivo>"
            exit 1
        }
        Import-Table $Tabla $Archivo $DBHost $DBUser $DBPass $DBName $DBPort
    }
    "ImportAll" {
        Import-All
    }
    "ListTables" {
        List-Tables
    }
    "Validate" {
        if (-Not $Archivo) {
            Write-Error "Uso: .\import-csv.ps1 -Comando Validate -Archivo <archivo>"
            exit 1
        }
        Validate-CSV $Archivo
    }
    "Backup" {
        Backup-Database
    }
    default {
        Write-Error "Comando desconocido: $Comando"
        Show-Help
        exit 1
    }
}

exit 0
