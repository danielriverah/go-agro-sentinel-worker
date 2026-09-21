package database

import "database/sql"

// RunMigrations is intentionally a no-op.
// All schema changes are applied by the database administrator.
func RunMigrations(_ *sql.DB) error {
	return nil
}
