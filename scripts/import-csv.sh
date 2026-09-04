#!/bin/bash

# =====================================================
# SCRIPT: Utilidad para importar datos desde CSV
# Sistema Agro Sentinel
# =====================================================
# Descripción: Script bash para facilitar la importación
# de archivos CSV a la base de datos MySQL
#
# Uso: ./import-csv.sh [tabla] [archivo.csv]
# Ejemplo: ./import-csv.sh articulos data/csv/examples/articulos.csv
# =====================================================

set -e  # Salir si hay error

# =====================================================
# COLORES PARA SALIDA
# =====================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# =====================================================
# CONFIGURACIÓN DE CONEXIÓN A BASE DE DATOS
# =====================================================
DB_HOST="${DB_HOST:-localhost}"
DB_USER="${DB_USER:-admin}"
DB_PASS="${DB_PASS:-admin}"
DB_NAME="${DB_NAME:-agro}"
DB_PORT="${DB_PORT:-3306}"

# =====================================================
# FUNCIONES
# =====================================================

# Función para imprimir mensajes
print_message() {
    local level=$1
    local message=$2

    case $level in
        "info")
            echo -e "${BLUE}[INFO]${NC} $message"
            ;;
        "success")
            echo -e "${GREEN}[OK]${NC} $message"
            ;;
        "warning")
            echo -e "${YELLOW}[WARN]${NC} $message"
            ;;
        "error")
            echo -e "${RED}[ERROR]${NC} $message"
            ;;
    esac
}

# Función para mostrar el uso del script
show_usage() {
    cat << EOF
${BLUE}=== UTILIDAD DE IMPORTACIÓN CSV - AGRO SENTINEL ===${NC}

${YELLOW}Uso:${NC}
    $0 [COMANDO] [OPCIONES]

${YELLOW}Comandos:${NC}
    import <tabla> <archivo>       Importar CSV a una tabla específica
    import-all                      Importar todos los CSV de ejemplo
    list-tables                     Listar tablas disponibles
    validate <archivo>              Validar estructura de un CSV
    backup                          Crear backup de las tablas
    restore <backup_file>           Restaurar desde un backup
    help                            Mostrar esta ayuda

${YELLOW}Tablas soportadas:${NC}
    - articulos
    - centros_costos
    - producciones
    - zonas_producciones
    - asignaciones_zonas_producciones
    - s3_monitoring_escena_ia_resumen

${YELLOW}Ejemplos:${NC}
    $0 import articulos data/csv/examples/articulos.csv
    $0 import-all
    $0 validate data/csv/examples/articulos.csv
    $0 backup

${YELLOW}Variables de entorno:${NC}
    DB_HOST         Host de MySQL (default: localhost)
    DB_USER         Usuario MySQL (default: admin)
    DB_PASS         Contraseña MySQL (default: admin)
    DB_NAME         Base de datos (default: agro)
    DB_PORT         Puerto MySQL (default: 3306)

EOF
}

# Función para verificar conexión a base de datos
check_db_connection() {
    print_message "info" "Verificando conexión a la base de datos..."

    if ! mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" "$DB_NAME" -e "SELECT 1;" > /dev/null 2>&1; then
        print_message "error" "No se puede conectar a la base de datos"
        print_message "error" "Host: $DB_HOST, Usuario: $DB_USER, BD: $DB_NAME"
        return 1
    fi

    print_message "success" "Conexión a la base de datos verificada"
    return 0
}

# Función para verificar si un archivo existe
check_file_exists() {
    local file=$1
    if [ ! -f "$file" ]; then
        print_message "error" "El archivo no existe: $file"
        return 1
    fi
    print_message "success" "Archivo encontrado: $file"
    return 0
}

# Función para importar una tabla
import_table() {
    local tabla=$1
    local archivo=$2

    print_message "info" "Iniciando importación de tabla: $tabla"
    print_message "info" "Archivo: $archivo"

    # Verificar archivo
    if ! check_file_exists "$archivo"; then
        return 1
    fi

    # Verificar conexión
    if ! check_db_connection; then
        return 1
    fi

    # Preparar SQL según la tabla
    local sql_file="/tmp/import_${tabla}_$$.sql"

    case $tabla in
        "articulos")
            cat > "$sql_file" << 'EOF'
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '__FILE__'
INTO TABLE articulos
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(articulo_id, nombre, variedad);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM articulos;
EOF
            ;;
        "centros_costos")
            cat > "$sql_file" << 'EOF'
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '__FILE__'
INTO TABLE centros_costos
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(centro_costo_id, nombre);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM centros_costos;
EOF
            ;;
        "producciones")
            cat > "$sql_file" << 'EOF'
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '__FILE__'
INTO TABLE producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(folio, articulo_id, fecha, hora, fecha_cierre, hora_cierre, usuario, estatus, aplicado, cantidad, centro_costo_id, monitoring);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM producciones;
EOF
            ;;
        "zonas_producciones")
            cat > "$sql_file" << 'EOF'
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '__FILE__'
INTO TABLE zonas_producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(zona_produccion_id, nombre, nombre_corto, estatus, area, centro_costo_id, poligono);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM zonas_producciones;
EOF
            ;;
        "asignaciones_zonas_producciones")
            cat > "$sql_file" << 'EOF'
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '__FILE__'
INTO TABLE asignaciones_zonas_producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(asignacion_zona_prod_id, produccion_id, zona_produccion_id, tipo_asignacion, area, poligono);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM asignaciones_zonas_producciones;
EOF
            ;;
        "s3_monitoring_escena_ia_resumen")
            cat > "$sql_file" << 'EOF'
SET GLOBAL local_infile = 1;
LOAD DATA LOCAL INFILE '__FILE__'
INTO TABLE s3_monitoring_escena_ia_resumen
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(s3_monitoring_escena_id, estado_clave, estado_general, riesgo_nivel, riesgo_motivo, fecha_analisis, json_original);
SELECT CONCAT('Registros importados: ', COUNT(*)) FROM s3_monitoring_escena_ia_resumen;
EOF
            ;;
        *)
            print_message "error" "Tabla no soportada: $tabla"
            return 1
            ;;
    esac

    # Reemplazar ruta del archivo
    sed -i "s|__FILE__|$(pwd)/$archivo|g" "$sql_file"

    # Ejecutar importación
    print_message "info" "Ejecutando importación..."

    if mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" "$DB_NAME" \
        --local-infile=1 < "$sql_file"; then
        print_message "success" "Importación de $tabla completada exitosamente"
        rm "$sql_file"
        return 0
    else
        print_message "error" "Error durante la importación de $tabla"
        rm "$sql_file"
        return 1
    fi
}

# Función para importar todos los CSV
import_all() {
    print_message "info" "Importando todos los archivos CSV..."

    local tablas=("articulos" "centros_costos" "zonas_producciones" "producciones" "asignaciones_zonas_producciones" "s3_monitoring_escena_ia_resumen")
    local base_dir="data/csv/examples"

    local count=0
    for tabla in "${tablas[@]}"; do
        local archivo="$base_dir/${tabla}.csv"
        print_message "info" "Procesando: $tabla"

        if import_table "$tabla" "$archivo"; then
            ((count++))
        else
            print_message "warning" "Error importando $tabla"
        fi

        echo ""
    done

    print_message "success" "Importación completada: $count/${#tablas[@]} tablas"
}

# Función para listar tablas
list_tables() {
    print_message "info" "Tablas disponibles en la base de datos:"

    if ! check_db_connection; then
        return 1
    fi

    mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" "$DB_NAME" -e \
        "SELECT TABLE_NAME as 'Tabla', TABLE_ROWS as 'Registros' FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA='$DB_NAME' ORDER BY TABLE_NAME;"
}

# Función para validar CSV
validate_csv() {
    local archivo=$1

    print_message "info" "Validando archivo CSV: $archivo"

    if ! check_file_exists "$archivo"; then
        return 1
    fi

    # Contar líneas
    local total_lines=$(wc -l < "$archivo")
    local data_lines=$((total_lines - 1))

    print_message "success" "Archivo válido"
    echo "  Línea de encabezado: 1"
    echo "  Líneas de datos: $data_lines"
    echo "  Total de líneas: $total_lines"

    # Mostrar primeras líneas
    echo ""
    print_message "info" "Primeras 3 líneas del archivo:"
    head -3 "$archivo" | sed 's/^/  /'
}

# Función para crear backup
backup_database() {
    local backup_date=$(date +%Y%m%d_%H%M%S)
    local backup_file="backups/agro_backup_${backup_date}.sql"

    print_message "info" "Creando backup de la base de datos..."

    # Crear directorio si no existe
    mkdir -p backups

    if mysqldump -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" "$DB_NAME" > "$backup_file"; then
        print_message "success" "Backup creado: $backup_file"
        return 0
    else
        print_message "error" "Error al crear el backup"
        return 1
    fi
}

# =====================================================
# MAIN - Procesamiento de comandos
# =====================================================

if [ $# -eq 0 ]; then
    show_usage
    exit 0
fi

case "$1" in
    "import")
        if [ $# -lt 3 ]; then
            print_message "error" "Uso: $0 import <tabla> <archivo.csv>"
            exit 1
        fi
        import_table "$2" "$3"
        ;;
    "import-all")
        import_all
        ;;
    "list-tables")
        list_tables
        ;;
    "validate")
        if [ $# -lt 2 ]; then
            print_message "error" "Uso: $0 validate <archivo.csv>"
            exit 1
        fi
        validate_csv "$2"
        ;;
    "backup")
        backup_database
        ;;
    "restore")
        if [ $# -lt 2 ]; then
            print_message "error" "Uso: $0 restore <backup_file>"
            exit 1
        fi
        print_message "warning" "Restaurar borrará la base de datos actual. Continuar? (s/n)"
        read -r response
        if [ "$response" = "s" ]; then
            mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" "$DB_NAME" < "$2"
            print_message "success" "Base de datos restaurada"
        else
            print_message "warning" "Operación cancelada"
        fi
        ;;
    "help")
        show_usage
        ;;
    *)
        print_message "error" "Comando desconocido: $1"
        show_usage
        exit 1
        ;;
esac

exit 0
