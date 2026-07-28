package db

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
)

type Options struct {
	MaxOpenConns int
	MaxIdleConns int
}

func Connect(dsn string, opts ...Options) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	opt := Options{MaxOpenConns: 25, MaxIdleConns: 5}
	if len(opts) > 0 {
		if opts[0].MaxOpenConns > 0 {
			opt.MaxOpenConns = opts[0].MaxOpenConns
		}
		if opts[0].MaxIdleConns > 0 {
			opt.MaxIdleConns = opts[0].MaxIdleConns
		}
	}
	db.SetMaxOpenConns(opt.MaxOpenConns)
	db.SetMaxIdleConns(opt.MaxIdleConns)
	return db, nil
}

func RunMigrations(db *sqlx.DB, dir string) error {
	migrations := &migrate.FileMigrationSource{Dir: dir}
	n, err := migrate.Exec(db.DB, "postgres", migrations, migrate.Up)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if n > 0 {
		fmt.Printf("applied %d migration(s)\n", n)
	}
	return nil
}
