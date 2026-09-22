# Sistema de Permisos, Roles y Políticas — Agro Sentinel

**Fecha**: 2026-09-22
**Estado**: Aprobado por el usuario, pendiente de implementación

## Resumen

Sistema de control de acceso basado en roles (RBAC) con permisos directos opcionales, inspirado en AWS IAM pero simplificado: solo Allow (sin Deny), con scope por rancho (`centro_costo_id`) para acciones per-recurso y scope global para acciones administrativas.

Reemplaza los mecanismos actuales (`RequireAdmin`, `RequireUserIn`, `AUTH_DELETE_ALLOWED_USER_IDS`) con un modelo unificado que filtra tanto las acciones como la visibilidad de datos por rancho.

## Decisiones de diseño

| Decisión | Elección | Alternativa descartada |
|---|---|---|
| Efecto de permisos | Solo Allow (sin permiso = denegado) | Allow + Deny (más complejo, sin beneficio real aquí) |
| Scope de recursos | Dos niveles: global + rancho | Todo por rancho con comodín `*` (menos preciso) |
| Granularidad de recursos | Nivel rancho (`centro_costo_id`) | Nivel producción individual (complejidad innecesaria) |
| Asignación | Roles + permisos directos (excepciones) | Solo roles (inflexible) o solo permisos (tedioso) |
| Bootstrap | Primer usuario = Superadmin automático | Seed por env var o script del DBA |
| Caché de permisos | En memoria del servidor, TTL = sesión JWT | En el JWT (payload grande, cambios no surten efecto) |

## Restricciones del proyecto

- **La tabla de usuarios es `agro_usuarios` con PK `id`** (no `usuario_id`).
- **Auth usa procedimientos MySQL** — nunca queries directos a `agro_usuarios`.
- **`RunMigrations` es un no-op** — el DBA aplica todo cambio de esquema.
- **Degradación obligatoria** — si las tablas de permisos no existen, el sistema opera en modo permisivo (comportamiento actual).

---

## Catálogo de acciones

Las acciones se organizan en módulos (para la UI) y tienen un scope que define si se aplican globalmente o por rancho.

| Módulo | Permiso (clave) | Scope | Descripción |
|---|---|---|---|
| producciones | `producciones.ver` | rancho | Ver lista y detalle de producciones |
| producciones | `producciones.editar` | rancho | Editar polígono, fases, ia_auto |
| producciones | `producciones.bloquear` | rancho | Marcar/desmarcar posible cosecha |
| monitoreo | `monitoreo.eliminar` | rancho | Borrar monitoreo (irreversible) |
| monitoreo | `monitoreo.worker` | rancho | Disparar worker por producción |
| alertas | `alertas.ver` | rancho | Ver alertas del rancho |
| alertas | `alertas.gestionar` | rancho | Marcar vista/resuelta |
| fases | `fases.ver` | rancho | Ver fases de cultivo |
| fases | `fases.editar` | rancho | Crear/editar/copiar fases |
| escenas | `escenas.ver` | rancho | Ver escenas e índices |
| escenas | `escenas.analizar` | rancho | Disparar análisis IA |
| admin | `usuarios.ver` | global | Listar usuarios |
| admin | `usuarios.administrar` | global | Activar/desactivar, reset password |
| admin | `roles.administrar` | global | Crear/editar roles y asignar permisos |
| sistema | `sync.ver` | global | Ver estado del sync |
| sistema | `sync.ejecutar` | global | Disparar sincronización |
| sistema | `worker.ver` | global | Ver estado global del worker |
| sistema | `worker.controlar` | global | Cancelar, desbloquear worker |

Total: 18 permisos (11 per-rancho, 7 globales).

---

## Estructura de tablas

### `auth_permisos` — Catálogo de acciones

```sql
CREATE TABLE auth_permisos (
  permiso_id     INT AUTO_INCREMENT PRIMARY KEY,
  clave          VARCHAR(50)  NOT NULL UNIQUE,
  modulo         VARCHAR(30)  NOT NULL,
  descripcion    VARCHAR(120) NOT NULL,
  scope          ENUM('global','rancho') NOT NULL DEFAULT 'rancho',
  fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Tabla de referencia, solo se modifica al agregar funcionalidad nueva. Se puebla con los 18 permisos del catálogo.

### `auth_roles` — Roles predefinidos y personalizados

```sql
CREATE TABLE auth_roles (
  rol_id         INT AUTO_INCREMENT PRIMARY KEY,
  nombre         VARCHAR(50)  NOT NULL UNIQUE,
  descripcion    VARCHAR(200),
  es_sistema     TINYINT(1)   NOT NULL DEFAULT 0,
  fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

`es_sistema = 1` protege de eliminación (Superadmin, Supervisor, Operador).

### `auth_rol_permisos` — Permisos de cada rol

```sql
CREATE TABLE auth_rol_permisos (
  rol_id      INT NOT NULL,
  permiso_id  INT NOT NULL,
  PRIMARY KEY (rol_id, permiso_id),
  KEY idx_permiso (permiso_id)
);
```

### `auth_usuario_roles` — Asignación de roles con scope

```sql
CREATE TABLE auth_usuario_roles (
  usuario_rol_id  BIGINT AUTO_INCREMENT PRIMARY KEY,
  usuario_id      BIGINT       NOT NULL,   -- FK lógica a agro_usuarios.id
  rol_id          INT          NOT NULL,
  centro_costo_id BIGINT       NULL,       -- NULL = aplica a todos los ranchos
  asignado_por    BIGINT       NOT NULL,   -- agro_usuarios.id de quien asignó
  fecha_creacion  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_usr_rol_cc (usuario_id, rol_id, centro_costo_id),
  KEY idx_usuario (usuario_id),
  KEY idx_centro_costo (centro_costo_id)
);
```

`centro_costo_id = NULL` equivale a `Resource: "*"` en AWS IAM: el rol aplica a todos los ranchos.

### `auth_usuario_permisos` — Permisos directos (excepciones)

```sql
CREATE TABLE auth_usuario_permisos (
  usuario_permiso_id BIGINT AUTO_INCREMENT PRIMARY KEY,
  usuario_id         BIGINT    NOT NULL,   -- FK lógica a agro_usuarios.id
  permiso_id         INT       NOT NULL,
  centro_costo_id    BIGINT    NULL,       -- NULL = global
  asignado_por       BIGINT    NOT NULL,
  fecha_creacion     DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_usr_perm_cc (usuario_id, permiso_id, centro_costo_id),
  KEY idx_usuario (usuario_id),
  KEY idx_centro_costo (centro_costo_id)
);
```

Para asignar permisos sueltos sin crear un rol. Ejemplo: "usuario 7 puede `monitoreo.eliminar` solo en rancho 15".

---

## Roles seed

| Rol | es_sistema | Permisos incluidos |
|---|---|---|
| **Superadmin** | 1 | Todos (18/18) |
| **Supervisor** | 1 | `producciones.*`, `alertas.*`, `fases.*`, `escenas.*`, `monitoreo.worker` (12/18) |
| **Operador** | 1 | `producciones.ver`, `alertas.ver`, `fases.ver`, `escenas.ver` (4/18) |

El Superadmin siempre puede todo. Los otros dos son plantillas de partida; el admin puede crear roles personalizados.

---

## Bootstrap del primer usuario

Al arrancar la API, si la tabla `auth_usuario_roles` existe pero está vacía:

1. Obtener el usuario activo con `id` más bajo de `agro_usuarios` (vía un procedimiento o función MySQL por la convención del proyecto).
2. Asignar el rol Superadmin con `centro_costo_id = NULL`.
3. Registrar en log: `"bootstrap: usuario %d asignado como Superadmin"`.

Esto se ejecuta una sola vez. Si ya hay al menos una asignación, no se toca.

---

## Evaluación de permisos

### Algoritmo

Para verificar si el usuario `U` puede ejecutar la acción `A` sobre el recurso `R` (un `centro_costo_id`, o evaluación global):

```
permisos_efectivos = (
    -- De roles asignados
    SELECT p.clave
    FROM auth_usuario_roles ur
    JOIN auth_rol_permisos rp ON ur.rol_id = rp.rol_id
    JOIN auth_permisos p ON rp.permiso_id = p.permiso_id
    WHERE ur.usuario_id = U
      AND (ur.centro_costo_id = R OR ur.centro_costo_id IS NULL)

    UNION

    -- De permisos directos
    SELECT p.clave
    FROM auth_usuario_permisos up
    JOIN auth_permisos p ON up.permiso_id = p.permiso_id
    WHERE up.usuario_id = U
      AND (up.centro_costo_id = R OR up.centro_costo_id IS NULL)
)

SI A ∈ permisos_efectivos → PERMITIR
SI NO → DENEGAR (403)
```

Para acciones globales (`scope = 'global'`), `R` no aplica; la condición es `centro_costo_id IS NULL`.

### Caché en memoria

Los permisos se consultan a la BD una vez (al primer request autenticado del usuario) y se cachean en memoria por el TTL del JWT (8 horas). Un mapa `sync.Map` keyed por `usuario_id` almacena la estructura de permisos.

Un endpoint `POST /api/v1/auth/refrescar-permisos` permite forzar la recarga sin re-login, para que los cambios surtan efecto inmediatamente.

---

## Middleware

### `RequirePermission(permiso string)`

Reemplaza `RequireAdmin` y `RequireUserIn`. Firma:

```go
func RequirePermission(permiso string) func(http.Handler) http.Handler
```

**Para rutas que operan sobre una producción** (ej: `DELETE /api/v1/producciones/{id}/monitoreo`):
1. Extrae `{id}` de la ruta → obtiene la producción → resuelve su `centro_costo_id`
2. Evalúa `monitoreo.eliminar` contra ese `centro_costo_id`

**Para rutas globales** (ej: `GET /api/v1/admin/usuarios`):
1. Evalúa `usuarios.ver` sin recurso (scope global)

**Para listados filtrados** (ej: `GET /api/v1/producciones`):
1. El middleware no bloquea
2. El handler consulta los `centro_costo_id` donde el usuario tiene `producciones.ver`
3. Si tiene scope `NULL`, no filtra; si tiene lista explícita, `WHERE centro_costo_id IN (...)`
4. Lo mismo para `GET /api/v1/alertas` con `alertas.ver`

### Migración de middlewares existentes

| Ruta actual | Middleware actual | Nuevo |
|---|---|---|
| `GET /api/v1/admin/usuarios` | `RequireAdmin` | `RequirePermission("usuarios.ver")` |
| `PUT /api/v1/admin/usuarios/{id}/activo` | `RequireAdmin` | `RequirePermission("usuarios.administrar")` |
| `PUT /api/v1/admin/usuarios/{id}/password` | `RequireAdmin` | `RequirePermission("usuarios.administrar")` |
| `DELETE /api/v1/producciones/{id}/monitoreo` | `RequireUserIn(...)` | `RequirePermission("monitoreo.eliminar")` |
| `POST /api/v1/producciones/{id}/bloquear` | (solo JWT) | `RequirePermission("producciones.bloquear")` |
| `POST /api/v1/producciones/{id}/desbloquear` | (solo JWT) | `RequirePermission("producciones.bloquear")` |
| `PUT /api/v1/producciones/{id}/fases` | (solo JWT) | `RequirePermission("fases.editar")` |
| `PUT /api/v1/producciones/{id}/poligono` | (solo JWT) | `RequirePermission("producciones.editar")` |
| `POST /api/v1/escenas/{id}/analizar` | (solo JWT) | `RequirePermission("escenas.analizar")` |
| `POST /api/v1/sync/trigger` | (solo JWT) | `RequirePermission("sync.ejecutar")` |
| `POST /api/v1/worker/cancel` | (solo JWT) | `RequirePermission("worker.controlar")` |
| `POST /api/v1/worker/unlock` | (solo JWT) | `RequirePermission("worker.controlar")` |
| `POST /api/v1/worker/run` | (solo JWT) | `RequirePermission("worker.controlar")` |
| `POST /api/v1/worker/run-production/{id}` | (solo JWT) | `RequirePermission("monitoreo.worker")` |

Las rutas de solo lectura (`GET /api/v1/producciones`, `GET /api/v1/escenas/{id}`, etc.) pasan por el filtrado de listados, no por un middleware bloqueante.

---

## API de administración de roles y permisos

### Endpoints nuevos

| Método | Ruta | Permiso | Descripción |
|---|---|---|---|
| `GET` | `/api/v1/admin/roles` | `roles.administrar` | Listar roles con sus permisos |
| `POST` | `/api/v1/admin/roles` | `roles.administrar` | Crear rol |
| `PUT` | `/api/v1/admin/roles/{id}` | `roles.administrar` | Editar rol (nombre, permisos) |
| `DELETE` | `/api/v1/admin/roles/{id}` | `roles.administrar` | Eliminar rol (no `es_sistema`) |
| `GET` | `/api/v1/admin/permisos` | `roles.administrar` | Catálogo de permisos disponibles |
| `GET` | `/api/v1/admin/usuarios/{id}/permisos` | `roles.administrar` | Permisos efectivos de un usuario |
| `PUT` | `/api/v1/admin/usuarios/{id}/roles` | `roles.administrar` | Asignar roles (reemplazo completo) |
| `PUT` | `/api/v1/admin/usuarios/{id}/permisos-directos` | `roles.administrar` | Asignar permisos directos |
| `GET` | `/api/v1/auth/permisos` | (cualquier sesión) | Mis permisos efectivos |
| `POST` | `/api/v1/auth/refrescar-permisos` | (cualquier sesión) | Forzar recarga de caché |

### Formato de respuesta: `GET /api/v1/auth/permisos`

```json
{
  "global": ["usuarios.ver", "sync.ver", "worker.ver"],
  "por_rancho": {
    "42": ["producciones.ver", "producciones.editar", "alertas.ver", "alertas.gestionar"],
    "15": ["producciones.ver"]
  },
  "ranchos_todos": false,
  "degraded": false
}
```

- `ranchos_todos: true` cuando tiene un rol con `centro_costo_id = NULL` que incluye permisos per-rancho.
- `degraded: true` cuando las tablas de permisos no existen (modo permisivo).

### Formato de asignación: `PUT /api/v1/admin/usuarios/{id}/roles`

```json
{
  "asignaciones": [
    { "rol_id": 2, "centro_costo_id": 42 },
    { "rol_id": 3, "centro_costo_id": null }
  ]
}
```

Reemplaza el conjunto completo de asignaciones de roles del usuario. Lo que no esté en la lista se quita, lo nuevo se agrega.

### Salvaguardas

- No se puede quitar el último usuario con rol Superadmin.
- No se puede eliminar un rol con `es_sistema = 1`.
- No te puedes quitar permisos a ti mismo (previene quedarse fuera).
- Los `centro_costo_id` se validan contra la tabla `centros_costos` del ERP.

---

## Degradación

### Si las tablas de permisos no existen

Al construir el repositorio de permisos, se prueba `SELECT 1 FROM information_schema.tables WHERE table_name = 'auth_permisos'`.

Si no existe:
- `RequirePermission` deja pasar todo (comportamiento actual: sesión válida = acceso total).
- `GET /api/v1/auth/permisos` retorna `{"degraded": true}`.
- El frontend no filtra nada.
- Log una vez: `"auth_permisos table not found, running in permissive mode"`.

### Transición de `AUTH_DELETE_ALLOWED_USER_IDS`

Se mantiene temporalmente como fallback:
- Si las tablas de permisos **no existen** Y la env var está configurada → se usa la env var (comportamiento actual).
- Si las tablas de permisos **existen** → se usa el sistema de permisos, la env var se ignora.
- Log de advertencia si ambos están configurados.

---

## Frontend

### Navegación condicional

El frontend consume `GET /api/v1/auth/permisos` al iniciar sesión y almacena los permisos en el store de auth.

Items del menú se muestran/ocultan según:
- **Alertas**: visible si tiene `alertas.ver` en al menos un rancho.
- **Configuración**: visible si tiene cualquier permiso del módulo `admin`.
- **Botón borrar monitoreo**: habilitado si tiene `monitoreo.eliminar` en el rancho de esa producción.
- **Botón bloquear/desbloquear**: habilitado si tiene `producciones.bloquear` en ese rancho.
- **Botón analizar IA**: habilitado si tiene `escenas.analizar` en ese rancho.

### Vista de administración de permisos

En `ConfiguracionView.vue`, pestaña "Roles y Permisos" (visible solo con `roles.administrar`):

**Panel de roles**: Lista de roles, click para ver/editar permisos (checkboxes agrupados por módulo).

**Panel de usuario seleccionado**:
1. **Roles asignados**: chips `Rol @ Rancho`. Botón para agregar con selector de rol + selector de rancho (o "Todos los ranchos").
2. **Permisos directos**: chips `Permiso @ Rancho`.
3. **Permisos efectivos** (solo lectura): tabla colapsable por módulo y rancho, con indicador de origen.

---

## Scripts SQL para el DBA

Se entregará un archivo `scripts/phase5-auth-permisos-roles.sql` con:

1. `CREATE TABLE` de las 5 tablas.
2. `INSERT` de los 18 permisos del catálogo.
3. `INSERT` de los 3 roles seed (Superadmin, Supervisor, Operador) con `es_sistema = 1`.
4. `INSERT` de los permisos de cada rol seed en `auth_rol_permisos`.

El bootstrap del primer Superadmin se hace desde el código Go al arrancar, no desde SQL.

---

## Pruebas

- Evaluación de permisos: usuario con rol Supervisor en rancho A no puede borrar en rancho A (no tiene `monitoreo.eliminar`), pero sí puede ver alertas.
- Scope global: usuario con `usuarios.administrar` puede listar usuarios sin importar rancho.
- Scope NULL vs específico: usuario con rol Operador `centro_costo_id = NULL` ve producciones de todos los ranchos; otro con Operador `centro_costo_id = 42` solo ve las del rancho 42.
- Permisos directos: usuario sin rol que tiene `monitoreo.eliminar` directo en rancho 15 puede borrar ahí y solo ahí.
- Bootstrap: tabla vacía → primer usuario recibe Superadmin.
- Último Superadmin protegido: no se puede quitar.
- Degradación: sin tablas → modo permisivo, todo funciona como hoy.
- Transición: env var configurada + tablas existentes → se ignora la env var.
- Filtrado de listados: producciones solo muestra ranchos autorizados.

---

## Fuera de alcance

- Deny explícito (permisos negativos).
- Permisos a nivel de producción individual.
- Grupos de usuarios.
- Permisos temporales (con expiración).
- Auditoría de cambios de permisos (log de quién cambió qué).
- OAuth/SSO.
