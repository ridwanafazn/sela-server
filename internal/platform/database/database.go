// Package database owns PostgreSQL connectivity for the API and the
// migration runner.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/ridwanafazn/sela-server/internal/config"
)

// DB bundles the GORM handle and the underlying database/sql pool.
type DB struct {
	Gorm *gorm.DB
	SQL  *sql.DB
}

// Open creates the application connection pool, verifies connectivity, and
// applies pool limits. The application must use this pooled connection for
// ordinary traffic.
func Open(ctx context.Context, cfg config.Database) (*DB, error) {
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN: cfg.URL,
		// Neon pools application traffic through PgBouncer in transaction
		// mode. Simple protocol avoids prepared-statement failures across
		// pooled backends. Revisit after measuring performance.
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		// GORM's logger interpolates SQL parameters. Journal and message
		// content must never reach logs, so query logging stays disabled.
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get database pool: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	db := &DB{Gorm: gormDB, SQL: sqlDB}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()
	if err := db.Ping(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

// OpenSQL opens a single connection pool for migrations or administrative
// work. Use the direct, non-pooled database URL.
func OpenSQL(dsn string) (*sql.DB, error) {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open migration connection: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return sqlDB, nil
}

// Ping verifies that the database is reachable.
func (d *DB) Ping(ctx context.Context) error {
	return d.SQL.PingContext(ctx)
}

// Close releases the connection pool.
func (d *DB) Close() error {
	return d.SQL.Close()
}
