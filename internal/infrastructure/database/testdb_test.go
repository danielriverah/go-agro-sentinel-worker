package database

import (
	"database/sql"
	"math/rand"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// testDB opens a connection to MYSQL_TEST_DSN and runs migrations, skipping
// the calling test when that env var is not set.
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN not set, skipping database test")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.Ping(); err != nil {
		t.Fatalf("pinging test db: %v", err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	return db
}

// randomID returns a pseudo-random int64 usable as a unique produccion_id/scene id in tests.
func randomID() int64 {
	return int64(rand.Intn(1_000_000_000)) + 1
}
