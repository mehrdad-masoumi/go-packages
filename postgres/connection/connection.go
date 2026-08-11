package connection

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

var (
	registerOnce sync.Once
	driverName   = "postgres"
)

type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

func Open(ctx context.Context, cfg Config) (*sqlx.DB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errors.New("postgres: DSN is required")
	}
	driver, err := instrumentedDriver()
	if err != nil {
		return nil, err
	}
	db, err := sqlx.Open(driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 25
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 5
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = 5 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return db, nil
}

func EnsureDatabase(ctx context.Context, dsn string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	dbName := extractDBName(dsn)
	if dbName == "" {
		return errors.New("postgres ensure database: no dbname found in DSN")
	}
	maintDSN := replaceDBName(dsn, "postgres")
	conn, err := sql.Open("postgres", maintDSN)
	if err != nil {
		return fmt.Errorf("postgres maintenance open: %w", err)
	}
	defer conn.Close()
	var exists bool
	if err := conn.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists); err != nil {
		return fmt.Errorf("postgres check database: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(dbName)); err != nil {
		return fmt.Errorf("postgres create database: %w", err)
	}
	return nil
}

func instrumentedDriver() (string, error) {
	registerOnce.Do(func() {
		name, err := otelsql.Register("postgres",
			otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
			otelsql.WithSpanOptions(otelsql.SpanOptions{DisableErrSkip: true}),
		)
		if err != nil {
			// Observability must not make PostgreSQL unavailable. Fall back to lib/pq.
			driverName = "postgres"
			return
		}
		driverName = name
	})
	return driverName, nil
}

func extractDBName(dsn string) string {
	for _, part := range strings.Fields(dsn) {
		if strings.HasPrefix(part, "dbname=") {
			return strings.TrimPrefix(part, "dbname=")
		}
	}
	return ""
}

func replaceDBName(dsn, name string) string {
	parts := strings.Fields(dsn)
	for i, part := range parts {
		if strings.HasPrefix(part, "dbname=") {
			parts[i] = "dbname=" + name
		}
	}
	return strings.Join(parts, " ")
}
