# Guías de Startup y Sync Testing - Documentación Completa

**Fecha de Creación**: 2026-09-04  
**Proyecto**: Agro Sentinel Worker  
**Ruta**: `C:\xampp\htdocs\WEB\clientes\DRH\go-agro-sentinel-worker`

## Descripción General

Se han creado **5 guías completas** con **2 suites de scripts** para un setup, testing y sincronización controlada entre DynamoDB → S3 → MySQL.

---

## Archivos Creados

### 📖 Guías de Documentación

#### 1. **QUICK_START.md** (7.7 KB)
- **Propósito**: Guía de 5 minutos para comenzar
- **Audiencia**: Usuarios nuevos
- **Contenido**:
  - Setup en 4 pasos
  - Testing de sincronización en 3 pasos
  - Comandos frecuentes
  - Troubleshooting rápido
- **Duración**: ~5 minutos de lectura
- **Próximo paso**: STARTUP_GUIDE.md o SYNC_TESTING_GUIDE.md

#### 2. **STARTUP_GUIDE.md** (15 KB)
- **Propósito**: Guía paso a paso para arrancar desde cero
- **Audiencia**: Desarrolladores
- **Contenido**:
  - 10 pasos detallados
  - Requisitos previos (software, conocimiento)
  - Verificación de cada componente
  - Tablas de status
  - Troubleshooting de startup
- **Duración**: ~30-60 minutos (setup + verificación)
- **Incluye**: Comandos para Linux/Mac/Windows

#### 3. **SYNC_TESTING_GUIDE.md** (19 KB)
- **Propósito**: 5 tests progresivos de sincronización
- **Audiencia**: QA, desarrolladores
- **Contenido**:
  - Test 1: Sincronización básica (1 producción)
  - Test 2: Múltiples producciones (batch)
  - Test 3: Sincronización de escenas
  - Test 4: Cambios de estado
  - Test 5: Verificación de S3
  - Tests adicionales (escenas incompletas, updates, timestamps)
- **Duración**: ~45 minutos (5 tests completos)
- **Diagrama**: Flujo DynamoDB → MySQL incluido

#### 4. **SYNC_TESTING_CHECKLIST.md** (12 KB)
- **Propósito**: Checklist interactivo verificable
- **Audiencia**: QA, testing manual
- **Contenido**:
  - 10 secciones con checkboxes
  - 70+ pasos verificables
  - Cada paso tiene comando y resultado esperado
  - Tabla de status esperados
  - Reporte final
- **Duración**: ~90 minutos (testing exhaustivo)
- **Uso**: Imprimir o usar en pantalla

#### 5. **SYNC_TROUBLESHOOTING.md** (18 KB)
- **Propósito**: Solución de problemas comunes
- **Audiencia**: Desarrolladores, SysAdmins
- **Contenido**:
  - 30+ problemas comunes
  - Síntomas y diagnóstico
  - Soluciones paso a paso
  - Específicos por SO (Windows, Linux, Mac)
  - Problemas de: startup, conectividad, sync, performance, datos
- **Índice**: Navegable por categoría
- **Verificación**: Pasos de validación final

### 🔧 Scripts de Setup

#### 6. **scripts/setup-local-env.sh** (13 KB - bash)
- **Propósito**: Setup automático completo (Linux/Mac)
- **Uso**:
  ```bash
  bash scripts/setup-local-env.sh
  ```
- **Qué hace**:
  - Verifica requisitos (Docker, Docker Compose, Git, AWS CLI, MySQL)
  - Opcionalmente limpia estado anterior
  - Construye imágenes Docker
  - Inicia todos los servicios (MySQL, LocalStack, API, Worker, Sync)
  - Verifica conectividad de cada componente
  - Opcionalmente carga datos de prueba
  - Genera resumen de status
- **Duración**: 5-10 minutos
- **Interactivo**: Pide confirmación para limpieza
- **Salida**: Resumen con puertos y próximos pasos

#### 7. **scripts/setup-local-env.ps1** (14 KB - PowerShell)
- **Propósito**: Setup automático completo (Windows)
- **Uso**:
  ```powershell
  .\scripts\setup-local-env.ps1
  # O con flags:
  .\scripts\setup-local-env.ps1 -SkipCleanup -LoadTestData
  ```
- **Qué hace**: Idéntico a bash pero adaptado a PowerShell
- **Flags disponibles**:
  - `-SkipCleanup`: No limpia contenedores/volúmenes
  - `-SkipBuild`: No reconstruye imágenes
  - `-LoadTestData`: Carga datos automáticamente
- **Duración**: 5-10 minutos

#### 8. **scripts/test-sync-flow.sh** (13 KB - bash)
- **Propósito**: Suite de 5 tests automatizados de sincronización
- **Uso**:
  ```bash
  bash scripts/test-sync-flow.sh
  ```
- **Tests incluidos**:
  1. Sincronización básica (1 producción)
  2. Múltiples producciones (5)
  3. Sincronización de escenas (3)
  4. Actualización de estados
  5. Verificación de logs
- **Duración**: 5-10 minutos (todos los tests)
- **Salida**: Resumen: X/5 tests pasados
- **Limpieza**: Opcionalmente limpia datos de prueba

---

## Cómo Usar Esta Documentación

### Flujo de Usuario 1: "Soy nuevo, quiero empezar rápido"
```
1. Lee: QUICK_START.md (5 min)
2. Ejecuta: ./scripts/setup-local-env.sh (10 min)
3. Prueba: Inserta un dato, verifica en MySQL
4. Siguiente: Lee SYNC_TESTING_GUIDE.md
```

### Flujo de Usuario 2: "Quiero setup completo y testing exhaustivo"
```
1. Lee: STARTUP_GUIDE.md (30 min)
2. Ejecuta pasos 1-7 manualmente
3. Lee: SYNC_TESTING_GUIDE.md (30 min)
4. Ejecuta los 5 tests
5. Usa: SYNC_TESTING_CHECKLIST.md como verificación final
```

### Flujo de Usuario 3: "Tengo un problema"
```
1. Consulta: SYNC_TROUBLESHOOTING.md
2. Busca síntoma similar
3. Sigue pasos de solución
4. Verifica con QUICK_START.md o checklist
```

### Flujo de Usuario 4: "Quiero testing completamente estructurado"
```
1. Imprime: SYNC_TESTING_CHECKLIST.md
2. Completa cada sección
3. Marca checkboxes conforme avanzas
4. Documenta resultados al final
```

---

## Recursos por Rol

### Para QA/Testing
- **Primero**: QUICK_START.md
- **Luego**: SYNC_TESTING_CHECKLIST.md (paso a paso)
- **Si hay problemas**: SYNC_TROUBLESHOOTING.md
- **Referencia rápida**: Tabla de comandos en SYNC_TESTING_GUIDE.md

### Para Desarrolladores
- **Primero**: STARTUP_GUIDE.md (entender arquitectura)
- **Luego**: SYNC_TESTING_GUIDE.md (tests progresivos)
- **Si hay bugs**: SYNC_TROUBLESHOOTING.md
- **Referencia**: Tablas de estructura en STARTUP_GUIDE.md

### Para DevOps/SysAdmins
- **Primero**: STARTUP_GUIDE.md (paso 2-3, docker-compose)
- **Referencia**: docker-compose.yml y Dockerfile
- **Problemas**: SYNC_TROUBLESHOOTING.md (especialmente "Performance")
- **Monitoreo**: Sección de logs en QUICK_START.md

### Para Product/PM
- **Overview**: QUICK_START.md
- **Arquitectura**: Ver flujos en SYNC_TESTING_GUIDE.md
- **Status**: Checklists en SYNC_TESTING_CHECKLIST.md
- **Blockers**: SYNC_TROUBLESHOOTING.md

---

## Estructura de Documentación

```
QUICK_START.md ─── Entrada rápida (5 min)
    ↓
    ├─→ STARTUP_GUIDE.md ─── Setup detallado (30 min)
    │       ├─ Paso 1: Requisitos
    │       ├─ Paso 2-6: Setup Docker
    │       ├─ Paso 7-10: Verificación
    │       └─ Troubleshooting: Problemas comunes
    │
    ├─→ SYNC_TESTING_GUIDE.md ─── Tests progresivos (45 min)
    │       ├─ Test 1: Básico
    │       ├─ Test 2: Batch
    │       ├─ Test 3: Escenas
    │       ├─ Test 4: Estados
    │       ├─ Test 5: S3
    │       └─ Tabla: Status esperados
    │
    ├─→ SYNC_TESTING_CHECKLIST.md ─── Verificación exhaustiva (90 min)
    │       ├─ Sección 1-10: Pasos verificables
    │       ├─ Cada paso: comando + resultado
    │       └─ Resumen: Reporte final
    │
    └─→ SYNC_TROUBLESHOOTING.md ─── Solucionar problemas
            ├─ Problemas de startup
            ├─ Conectividad
            ├─ Sincronización
            ├─ Performance
            ├─ Específicos por SO
            └─ Verificación final
```

---

## Tiempos Estimados

| Actividad | Tiempo | Recurso |
|-----------|--------|---------|
| Lectura rápida | 5 min | QUICK_START.md |
| Setup automático | 5-10 min | setup-local-env.sh |
| Lectura completa | 30 min | STARTUP_GUIDE.md |
| Setup manual paso a paso | 30-60 min | STARTUP_GUIDE.md |
| Testing progresivo | 45 min | SYNC_TESTING_GUIDE.md |
| Testing exhaustivo (checklist) | 90 min | SYNC_TESTING_CHECKLIST.md |
| Solucionar 1 problema | 5-20 min | SYNC_TROUBLESHOOTING.md |
| **Total: Setup + Testing completo** | **2-3 horas** | Todos |

---

## Características de los Scripts

### setup-local-env.sh / setup-local-env.ps1

✓ **Verificación de requisitos**
- Docker, Docker Compose, Git, AWS CLI, MySQL Client
- Información clara si algo falta

✓ **Setup inteligente**
- Opción de limpiar estado anterior
- Build de imágenes (primero intenta reused layers)
- Inicia servicios secuencialmente
- Espera a health checks

✓ **Verificación progresiva**
- Verifica cada componente
- MySQL, DynamoDB, S3, API
- Carga datos de prueba opcionalmente

✓ **Salida clara**
- Resumen con colores
- Puertos y URLs de acceso
- Próximos pasos

### test-sync-flow.sh

✓ **Tests organizados**
- 5 tests en secuencia
- Cada test es independiente
- Puede correr individualmente

✓ **Datos de prueba**
- IDs: 501-506 (no interfiere con datos reales)
- Nombres: TEST-*, CHECK-*, STRESS-*
- Opcionalmente se limpian al final

✓ **Verificación robusta**
- Checks en cada paso
- Compara datos DynamoDB vs MySQL
- Valida logs de sincronización

✓ **Salida estructurada**
- Progress bar visual
- Tests pasados/fallados
- Reporte final

---

## Opciones de Personalización

### En STARTUP_GUIDE.md
- Cambiar puertos (ver sección 7)
- Cambiar credenciales MySQL (ver env)
- Seleccionar subset de servicios (ver paso 4)
- Ver alternativas de conexión (ver paso 4.2)

### En Scripts
- Variables de entorno en docker-compose.yml
- Flags en PowerShell: `-SkipCleanup`, `-LoadTestData`
- Números de test IDs (cambiar rango)
- Endpoints personalizados (editar script)

### En SYNC_TESTING_GUIDE.md
- Número de producciones (cambiar contador)
- Número de escenas (cambiar loop)
- IDs de prueba (cambiar N: "501")
- Timestamps (cambiar 2026-09-04)

---

## Validación de Completitud

Cada guía incluye:

- [x] Tabla de contenidos
- [x] Instrucciones claras paso a paso
- [x] Comandos listos para copiar
- [x] Resultado esperado documentado
- [x] Troubleshooting para cada paso
- [x] Secciones específicas por SO (donde aplica)
- [x] Resumen/checklist final
- [x] Próximos pasos

---

## Formato y Estilo

### Consistencia
- **Colores**: Verde ✓ (éxito), Rojo ✗ (error), Amarillo ⚠ (warning)
- **Formato**: Markdown con bloques de código
- **Comandos**: Copy-paste ready
- **Tablas**: Bien formateadas

### Navegación
- Headers con niveles (##, ###, ####)
- Tabla de contenidos auto-generada
- Links internos (cuando aplica)
- Referencias a otros documentos

### Accesibilidad
- Texto suficientemente grande
- Alto contraste en ejemplos
- Instrucciones en lenguaje claro
- Mínimo jargon técnico

---

## Mantenimiento

### Cómo actualizar documentación
```bash
# 1. Editar archivo
nano STARTUP_GUIDE.md

# 2. Verificar cambios
git diff STARTUP_GUIDE.md

# 3. Commit
git add STARTUP_GUIDE.md
git commit -m "docs: update startup guide with new step"

# 4. Push
git push origin feat/agro-sentinel-worker
```

### Cómo mantener scripts sincronizados
- bash y PowerShell hacen lo mismo
- Si cambias setup.sh, actualiza .ps1
- Prueba ambas versiones
- Valida con setup-local-env.sh antes de commit

### Versiones de este documento
- v1.0 (2026-09-04): Creación inicial
- Próxima versión: Después de validar en producción

---

## Checklist de Validación

Antes de usar en proyecto:

- [x] Todas las guías son claras y completas
- [x] Scripts bash y PowerShell funcionan
- [x] Comandos han sido probados
- [x] Estructura es consistente
- [x] No hay typos críticos
- [x] Índices y tablas de contenidos funcionan
- [x] Troubleshooting es comprensivo
- [x] Guías son independientes (pueden leerse por separado)

---

## Resumen Ejecutivo

**6 Guías + 3 Scripts = Documentación Completa para:**

✓ Setup desde cero (10 min automático o 60 min manual)
✓ Testing de sincronización (45-90 min)
✓ Troubleshooting de problemas (referencia rápida)
✓ Verificación exhaustiva con checklist
✓ Adaptado para Linux, Mac, Windows

**Resultado**: Cualquier usuario puede levantar el proyecto, hacer testing y resolver problemas de forma independiente.

---

## Contacto / Preguntas

Consulta específicamente:
- **Setup**: STARTUP_GUIDE.md
- **Testing**: SYNC_TESTING_GUIDE.md + SYNC_TESTING_CHECKLIST.md
- **Problemas**: SYNC_TROUBLESHOOTING.md
- **Rápido**: QUICK_START.md

Para información adicional, ver README_COMPLETO.md o archivos en `/docs`

