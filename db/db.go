package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
)

type Options struct {
	MaxOpenConns int
	MaxIdleConns int
}

// EnsureDBExists connects to the "postgres" maintenance database and creates
// the target database if it does not already exist. The DSN must be a
// key=value libpq connection string containing a "dbname=..." parameter.
func EnsureDBExists(dsn string) error {
	dbName := extractDBName(dsn)
	if dbName == "" {
		return fmt.Errorf("ensure db: no dbname found in DSN")
	}

	maintDSN := replaceDBName(dsn, "postgres")
	conn, err := sql.Open("postgres", maintDSN)
	if err != nil {
		return fmt.Errorf("ensure db: open maintenance connection: %w", err)
	}
	defer conn.Close()

	var exists bool
	err = conn.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ensure db: check existence: %w", err)
	}
	if exists {
		return nil
	}

	// Database names cannot be parameterised in CREATE DATABASE; use QuoteIdentifier.
	_, err = conn.Exec("CREATE DATABASE " + pq.QuoteIdentifier(dbName))
	if err != nil {
		return fmt.Errorf("ensure db: create: %w", err)
	}
	fmt.Printf("created database %q\n", dbName)
	return nil
}

func extractDBName(dsn string) string {
	for _, part := range strings.Fields(dsn) {
		if strings.HasPrefix(part, "dbname=") {
			return strings.TrimPrefix(part, "dbname=")
		}
	}
	return ""
}

func replaceDBName(dsn, newDB string) string {
	var parts []string
	for _, part := range strings.Fields(dsn) {
		if strings.HasPrefix(part, "dbname=") {
			parts = append(parts, "dbname="+newDB)
		} else {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
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
