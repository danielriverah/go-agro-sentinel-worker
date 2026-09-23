package database

import (
	"context"
	"database/sql"
	"time"
)

// El DBA aplica los cambios de esquema por separado (RunMigrations es un
// no-op), así que el binario puede desplegarse antes que la tabla o columna
// que necesita. Estos sondeos permiten degradar en lugar de fallar en cada
// consulta: se ejecutan una sola vez, al construir cada repositorio.

// schemaProbeTimeout acota el sondeo para que un catálogo lento no retrase el
// arranque del servicio.
const schemaProbeTimeout = 5 * time.Second

func contextWithShortTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), schemaProbeTimeout)
}

// columnExists indica si la columna está presente en la base actual.
// Ante cualquier error responde false: degradar es preferible a fallar.
func columnExists(db *sql.DB, table, column string) bool {
	ctx, cancel := contextWithShortTimeout()
	defer cancel()

	const q = `
SELECT 1 FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?
LIMIT 1`

	var one int
	return db.QueryRowContext(ctx, q, table, column).Scan(&one) == nil
}

// tableExists indica si la tabla está presente en la base actual.
func tableExists(db *sql.DB, table string) bool {
	ctx, cancel := contextWithShortTimeout()
	defer cancel()

	const q = `
SELECT 1 FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?
LIMIT 1`

	var one int
	return db.QueryRowContext(ctx, q, table).Scan(&one) == nil
}
