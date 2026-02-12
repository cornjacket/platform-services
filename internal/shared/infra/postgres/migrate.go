package postgres

import (
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver for database/sql
	"github.com/pressly/goose/v3"
)

// RunMigrations applies pending migrations from an embedded filesystem.
// The fsys should contain SQL files in the subdir directory (typically "migrations").
// Each service must use a unique tableName (e.g., "goose_ingestion") so that
// version tracking doesn't collide when multiple services share a database.
// Opens a temporary database/sql connection (separate from the pgxpool) because
// goose requires database/sql. The connection is closed after migration completes.
func RunMigrations(databaseURL string, fsys fs.FS, subdir, tableName string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database for migration: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(fsys)
	goose.SetTableName(tableName)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.Up(db, subdir); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}
