-- =====================================================
-- FASE 3 (C): PROCEDIMIENTOS DE ADMINISTRACIÓN DE USUARIOS
-- Sistema de Monitoreo Agro Sentinel
-- =====================================================
--
-- POR QUÉ
-- La API nunca consulta la tabla de usuarios directamente: todo pasa por
-- funciones y procedimientos MySQL (internal/auth/repo.go). Para la pantalla de
-- configuración hacen falta tres operaciones que hoy no existen.
--
-- sp_change_password no sirve para un administrador: exige la contraseña
-- actual, que quien administra no conoce.
--
-- IMPORTANTE PARA EL DBA
-- La tabla de agro_usuarios y los procedimientos existentes (fn_validate_login,
-- fn_get_usuario, sp_create_usuario, sp_change_password) viven sólo en la base
-- y no están versionados en este repositorio, así que los nombres de tabla y
-- columnas de abajo son una PLANTILLA: ajústalos a los reales. Lo que no debe
-- cambiar es el CONTRATO de salida (el JSON), porque el código Go lo parsea.
--
-- CONTRATO DE SALIDA (idéntico al de los procedimientos ya existentes)
--   { "ok": true|false, "code": "<CODIGO>", "user_id": N,
--     "username": "...", "activo": 0|1 }
--   Las funciones (fn_) devuelven ese JSON como valor de retorno.
--   Los procedimientos (sp_) lo devuelven como una única fila de un result set.
-- =====================================================

USE agro;

-- Ajusta estos nombres a los reales antes de ejecutar.
SET @tabla_usuarios = 'agro_usuarios';   -- tabla real de usuarios
SET @col_id         = 'id'; -- PK
SET @col_username   = 'username';
SET @col_activo     = 'activo';

DELIMITER $$

-- =====================================================
-- 1) fn_list_usuarios() -> JSON array
-- =====================================================
-- Devuelve la lista completa para la pantalla de administración.
-- Nunca expone hash ni salt.
DROP FUNCTION IF EXISTS fn_list_usuarios$$
CREATE FUNCTION fn_list_usuarios()
RETURNS JSON
READS SQL DATA
DETERMINISTIC
BEGIN
  DECLARE resultado JSON;

  SELECT COALESCE(
           JSON_ARRAYAGG(
             JSON_OBJECT(
               'user_id',        u.id,
               'username',       u.username,
               'activo',         u.activo,
               'fecha_creacion', DATE_FORMAT(u.fecha_creacion, '%Y-%m-%dT%H:%i:%sZ')
             )
           ),
           JSON_ARRAY()
         )
    INTO resultado
    FROM agro_usuarios u;

  RETURN JSON_OBJECT('ok', TRUE, 'code', 'OK', 'usuarios', resultado);
END$$

-- =====================================================
-- 2) sp_set_usuario_activo(user_id, activo)
-- =====================================================
-- Activa o desactiva. Las salvaguardas de "no desactivarte a ti mismo" y "no
-- dejar el sistema sin usuarios activos" se aplican también en la API, pero se
-- repiten aquí para que la base sea segura por sí sola.
DROP PROCEDURE IF EXISTS sp_set_usuario_activo$$
CREATE PROCEDURE sp_set_usuario_activo(
  IN p_user_id BIGINT,
  IN p_activo  TINYINT
)
BEGIN
  DECLARE v_existe        INT DEFAULT 0;
  DECLARE v_activos_resto INT DEFAULT 0;

  SELECT COUNT(*) INTO v_existe
    FROM agro_usuarios WHERE id = p_user_id;

  IF v_existe = 0 THEN
    SELECT JSON_OBJECT('ok', FALSE, 'code', 'USER_NOT_FOUND') AS resultado;
  ELSE
    SELECT COUNT(*) INTO v_activos_resto
      FROM agro_usuarios
     WHERE activo = 1 AND id <> p_user_id;

    IF p_activo = 0 AND v_activos_resto = 0 THEN
      -- Desactivar al último usuario activo dejaría el sistema sin acceso.
      SELECT JSON_OBJECT('ok', FALSE, 'code', 'LAST_ACTIVE_USER') AS resultado;
    ELSE
      UPDATE agro_usuarios
         SET activo = IF(p_activo = 0, 0, 1)
       WHERE id = p_user_id;

      SELECT JSON_OBJECT(
               'ok',       TRUE,
               'code',     'OK',
               'user_id',  u.id,
               'username', u.username,
               'activo',   u.activo
             ) AS resultado
        FROM agro_usuarios u
       WHERE u.id = p_user_id;
    END IF;
  END IF;
END$$

-- =====================================================
-- 3) sp_admin_reset_password(user_id, nueva)
-- =====================================================
-- Restablece sin conocer la contraseña actual. Debe generar el hash EXACTAMENTE
-- igual que sp_create_usuario y sp_change_password (mismo esquema de salt y
-- SHA2-256); si no, el usuario no podrá volver a entrar.
--
-- >>> DBA: copia aquí el mismo cálculo de hash que usan esos dos procedimientos.
DROP PROCEDURE IF EXISTS sp_admin_reset_password$$
CREATE PROCEDURE sp_admin_reset_password(
  IN p_user_id  BIGINT,
  IN p_password VARCHAR(255)
)
BEGIN
  DECLARE v_existe INT DEFAULT 0;
  DECLARE v_salt   VARCHAR(64);

  SELECT COUNT(*) INTO v_existe
    FROM agro_usuarios WHERE id = p_user_id;

  IF v_existe = 0 THEN
    SELECT JSON_OBJECT('ok', FALSE, 'code', 'USER_NOT_FOUND') AS resultado;
  ELSEIF p_password IS NULL OR CHAR_LENGTH(p_password) < 8 THEN
    SELECT JSON_OBJECT('ok', FALSE, 'code', 'PASSWORD_TOO_SHORT') AS resultado;
  ELSE
    SET v_salt = HEX(RANDOM_BYTES(16));

    UPDATE agro_usuarios
       SET salt          = v_salt,
           password_hash = SHA2(CONCAT(v_salt, p_password), 256)
     WHERE id = p_user_id;

    SELECT JSON_OBJECT(
             'ok',       TRUE,
             'code',     'OK',
             'user_id',  u.id,
             'username', u.username,
             'activo',   u.activo
           ) AS resultado
      FROM agro_usuarios u
     WHERE u.id = p_user_id;
  END IF;
END$$

DELIMITER ;

-- =====================================================
-- VERIFICACIÓN
-- =====================================================
-- SELECT fn_list_usuarios();
-- CALL sp_set_usuario_activo(1, 1);
-- Tras un reset, comprobar que el usuario puede autenticarse:
--   CALL sp_admin_reset_password(<id>, 'nueva-contrasena');
--   SELECT fn_validate_login('<username>', 'nueva-contrasena');
-- El segundo SELECT debe devolver ok = true. Si no, el cálculo del hash no
-- coincide con el de sp_create_usuario y hay que igualarlo.

-- =====================================================
-- ROLLBACK
-- =====================================================
-- DROP FUNCTION IF EXISTS fn_list_usuarios;
-- DROP PROCEDURE IF EXISTS sp_set_usuario_activo;
-- DROP PROCEDURE IF EXISTS sp_admin_reset_password;
