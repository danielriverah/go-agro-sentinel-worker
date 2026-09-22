-- Phase 5: Sistema de permisos, roles y políticas IAM
-- Aplicar manualmente por el DBA.

-- 1. Catálogo de acciones
CREATE TABLE IF NOT EXISTS auth_permisos (
  permiso_id     INT AUTO_INCREMENT PRIMARY KEY,
  clave          VARCHAR(50)  NOT NULL UNIQUE,
  modulo         VARCHAR(30)  NOT NULL,
  descripcion    VARCHAR(120) NOT NULL,
  scope          ENUM('global','rancho') NOT NULL DEFAULT 'rancho',
  fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 2. Roles
CREATE TABLE IF NOT EXISTS auth_roles (
  rol_id         INT AUTO_INCREMENT PRIMARY KEY,
  nombre         VARCHAR(50)  NOT NULL UNIQUE,
  descripcion    VARCHAR(200),
  es_sistema     TINYINT(1)   NOT NULL DEFAULT 0,
  fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 3. Permisos de cada rol
CREATE TABLE IF NOT EXISTS auth_rol_permisos (
  rol_id      INT NOT NULL,
  permiso_id  INT NOT NULL,
  PRIMARY KEY (rol_id, permiso_id),
  KEY idx_permiso (permiso_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 4. Asignación de roles con scope por rancho
CREATE TABLE IF NOT EXISTS auth_usuario_roles (
  usuario_rol_id  BIGINT AUTO_INCREMENT PRIMARY KEY,
  usuario_id      BIGINT       NOT NULL,
  rol_id          INT          NOT NULL,
  centro_costo_id BIGINT       NULL,
  asignado_por    BIGINT       NOT NULL,
  fecha_creacion  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_usr_rol_cc (usuario_id, rol_id, centro_costo_id),
  KEY idx_usuario (usuario_id),
  KEY idx_centro_costo (centro_costo_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 5. Permisos directos (excepciones)
CREATE TABLE IF NOT EXISTS auth_usuario_permisos (
  usuario_permiso_id BIGINT AUTO_INCREMENT PRIMARY KEY,
  usuario_id         BIGINT    NOT NULL,
  permiso_id         INT       NOT NULL,
  centro_costo_id    BIGINT    NULL,
  asignado_por       BIGINT    NOT NULL,
  fecha_creacion     DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_usr_perm_cc (usuario_id, permiso_id, centro_costo_id),
  KEY idx_usuario (usuario_id),
  KEY idx_centro_costo (centro_costo_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 6. Catálogo de permisos (18 acciones)
INSERT INTO auth_permisos (clave, modulo, descripcion, scope) VALUES
  ('producciones.ver',       'producciones', 'Ver lista y detalle de producciones',   'rancho'),
  ('producciones.editar',    'producciones', 'Editar polígono, fases, ia_auto',       'rancho'),
  ('producciones.bloquear',  'producciones', 'Marcar/desmarcar posible cosecha',      'rancho'),
  ('monitoreo.eliminar',     'monitoreo',    'Borrar monitoreo (irreversible)',        'rancho'),
  ('monitoreo.worker',       'monitoreo',    'Disparar worker por producción',         'rancho'),
  ('alertas.ver',            'alertas',      'Ver alertas del rancho',                 'rancho'),
  ('alertas.gestionar',      'alertas',      'Marcar vista/resuelta',                  'rancho'),
  ('fases.ver',              'fases',        'Ver fases de cultivo',                   'rancho'),
  ('fases.editar',           'fases',        'Crear/editar/copiar fases',              'rancho'),
  ('escenas.ver',            'escenas',      'Ver escenas e índices',                  'rancho'),
  ('escenas.analizar',       'escenas',      'Disparar análisis IA',                   'rancho'),
  ('usuarios.ver',           'admin',        'Listar usuarios',                        'global'),
  ('usuarios.administrar',   'admin',        'Activar/desactivar, reset password',     'global'),
  ('roles.administrar',      'admin',        'Crear/editar roles y asignar permisos',  'global'),
  ('sync.ver',               'sistema',      'Ver estado del sync',                    'global'),
  ('sync.ejecutar',          'sistema',      'Disparar sincronización',                'global'),
  ('worker.ver',             'sistema',      'Ver estado global del worker',           'global'),
  ('worker.controlar',       'sistema',      'Cancelar, desbloquear worker',           'global');

-- 7. Roles seed
INSERT INTO auth_roles (nombre, descripcion, es_sistema) VALUES
  ('Superadmin', 'Acceso total al sistema', 1),
  ('Supervisor', 'Ve todo, gestiona alertas/fases, no borra monitoreo', 1),
  ('Operador',   'Solo lectura de producciones, alertas, fases y escenas', 1);

-- 8. Permisos del Superadmin: todos
INSERT INTO auth_rol_permisos (rol_id, permiso_id)
SELECT (SELECT rol_id FROM auth_roles WHERE nombre = 'Superadmin'), permiso_id
FROM auth_permisos;

-- 9. Permisos del Supervisor (12 de 18)
INSERT INTO auth_rol_permisos (rol_id, permiso_id)
SELECT (SELECT rol_id FROM auth_roles WHERE nombre = 'Supervisor'), permiso_id
FROM auth_permisos
WHERE clave IN (
  'producciones.ver', 'producciones.editar', 'producciones.bloquear',
  'monitoreo.worker',
  'alertas.ver', 'alertas.gestionar',
  'fases.ver', 'fases.editar',
  'escenas.ver', 'escenas.analizar',
  'sync.ver', 'worker.ver'
);

-- 10. Permisos del Operador (4 de 18)
INSERT INTO auth_rol_permisos (rol_id, permiso_id)
SELECT (SELECT rol_id FROM auth_roles WHERE nombre = 'Operador'), permiso_id
FROM auth_permisos
WHERE clave IN (
  'producciones.ver', 'alertas.ver', 'fases.ver', 'escenas.ver'
);
