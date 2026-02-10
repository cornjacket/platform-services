//go:build integration

package testutil

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultDBURL = "postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable"

// NewTestPool creates a pgxpool connection to the test Postgres instance.
// Override with INTEGRATION_DB_URL environment variable.
func NewTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("INTEGRATION_DB_URL")
	if url == "" {
		url = defaultDBURL
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("failed to create test pool: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("failed to ping test database (is docker-compose running?): %v", err)
	}

	t.Cleanup(func() { pool.Close() })
	return pool
}

// RunMigrations applies all .sql files in the given directory, sorted by filename.
func RunMigrations(t *testing.T, pool *pgxpool.Pool, migrationDir string) {
	t.Helper()

	// sorted in lexicographic order automatically
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("failed to read migration dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(migrationDir, entry.Name())
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("failed to execute %s: %v", entry.Name(), err)
		}
	}
}

// MustNewTestPool creates a pgxpool for use in TestMain (where *testing.T is unavailable).
// Calls log.Fatal on failure. Caller is responsible for closing the pool.
func MustNewTestPool() *pgxpool.Pool {
	url := os.Getenv("INTEGRATION_DB_URL")
	if url == "" {
		url = defaultDBURL
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		log.Fatalf("failed to create test pool: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		log.Fatalf("failed to ping test database (is docker-compose running?): %v", err)
	}

	return pool
}

// MustRunMigrations applies migrations for use in TestMain (where *testing.T is unavailable).
// Calls log.Fatal on any error. Expects a clean schema (call MustDropAllTables first).
func MustRunMigrations(pool *pgxpool.Pool, migrationDir string) {
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		log.Fatalf("failed to read migration dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(migrationDir, entry.Name())
		sql, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("failed to read %s: %v", path, err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			log.Fatalf("failed to execute %s: %v", entry.Name(), err)
		}
	}
}

// MustDropAllTables drops all tables in the public schema.
// Used in TestMain before MustRunMigrations to ensure a clean schema.
func MustDropAllTables(pool *pgxpool.Pool) {
	query := `DO $$ DECLARE
		r RECORD;
	BEGIN
		FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP
			EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE';
		END LOOP;
	END $$`

	if _, err := pool.Exec(context.Background(), query); err != nil {
		log.Fatalf("failed to drop tables: %v", err)
	}
}

// TruncateTables truncates the specified tables with CASCADE.
func TruncateTables(t *testing.T, pool *pgxpool.Pool, tables ...string) {
	t.Helper()

	query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", strings.Join(tables, ", "))
	_, err := pool.Exec(context.Background(), query)
	if err != nil {
		t.Fatalf("failed to truncate tables %v: %v", tables, err)
	}
}
