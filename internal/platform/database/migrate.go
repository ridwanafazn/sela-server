package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ridwanafazn/sela-server/migrations"
)

const migrationsTable = "schema_migrations"

// newMigrator opens a dedicated connection for the migration run. The
// golang-migrate driver holds a single session-level advisory lock, so it
// must not share the application pool. The returned cleanup closes the
// migrator and its connection.
func newMigrator(dsn string) (*migrate.Migrate, func(), error) {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open migration connection: %w", err)
	}
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("load embedded migrations: %w", err)
	}

	driver, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{
		MigrationsTable: migrationsTable,
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("initialize migration driver: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		_ = driver.Close()
		return nil, nil, fmt.Errorf("initialize migrator: %w", err)
	}

	cleanup := func() {
		_, _ = migrator.Close()
		_ = sqlDB.Close()
	}

	return migrator, cleanup, nil
}

// MigrateUp applies every pending migration.
func MigrateUp(dsn string) error {
	migrator, cleanup, err := newMigrator(dsn)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

// MigrateDown rolls back every applied migration. Use only against a
// disposable database.
func MigrateDown(dsn string) error {
	migrator, cleanup, err := newMigrator(dsn)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := migrator.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("roll back migrations: %w", err)
	}

	return nil
}

// MigrateSteps applies n migrations. A negative n rolls back.
func MigrateSteps(dsn string, n int) error {
	migrator, cleanup, err := newMigrator(dsn)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := migrator.Steps(n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate %d steps: %w", n, err)
	}

	return nil
}

// MigrateForce marks the database as being at version without running
// migrations. It is a recovery tool for a dirty migration state.
func MigrateForce(dsn string, version int) error {
	migrator, cleanup, err := newMigrator(dsn)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := migrator.Force(version); err != nil {
		return fmt.Errorf("force migration version: %w", err)
	}

	return nil
}

// MigrationVersion reads the applied migration version. It reports zero when
// the migrations table does not exist yet.
func MigrationVersion(ctx context.Context, sqlDB *sql.DB) (uint, bool, error) {
	var (
		version int64
		dirty   bool
	)

	err := sqlDB.QueryRowContext(ctx, fmt.Sprintf("SELECT version, dirty FROM %s LIMIT 1", migrationsTable)).
		Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" { // undefined_table
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read migration version: %w", err)
	}

	if version < 0 {
		return 0, false, fmt.Errorf("invalid migration version %d", version)
	}

	return uint(version), dirty, nil
}
