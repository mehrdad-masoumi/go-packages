package migration

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	migrate "github.com/rubenv/sql-migrate"
)

func Run(db *sqlx.DB, dir string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("migration: db is nil")
	}
	source := &migrate.FileMigrationSource{Dir: dir}
	n, err := migrate.Exec(db.DB, "postgres", source, migrate.Up)
	if err != nil {
		return 0, fmt.Errorf("migration: %w", err)
	}
	return n, nil
}
