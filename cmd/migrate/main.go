// Command migrate applies the embedded SQL migrations to PostgreSQL.
//
// Usage:
//
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down [-all | -steps N]
//	go run ./cmd/migrate version
//	go run ./cmd/migrate force <version>
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

const usage = `usage:
  migrate up
  migrate down [-all | -steps N]
  migrate version
  migrate force <version>`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}

	_ = godotenv.Load()

	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL_UNPOOLED"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		return errors.New("DATABASE_URL_UNPOOLED or DATABASE_URL is required")
	}

	sqlDB, err := database.OpenSQL(dsn)
	if err != nil {
		return err
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	pingCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	switch args[0] {
	case "up":
		if err := database.MigrateUp(dsn); err != nil {
			return err
		}
		return printVersion(sqlDB)

	case "down":
		flags := flag.NewFlagSet("down", flag.ContinueOnError)
		all := flags.Bool("all", false, "roll back every applied migration")
		steps := flags.Int("steps", 1, "number of migrations to roll back")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *all {
			if err := database.MigrateDown(dsn); err != nil {
				return err
			}
			return printVersion(sqlDB)
		}
		if *steps < 1 {
			return errors.New("steps must be at least 1")
		}
		if err := database.MigrateSteps(dsn, -*steps); err != nil {
			return err
		}
		return printVersion(sqlDB)

	case "version":
		return printVersion(sqlDB)

	case "force":
		if len(args) < 2 {
			return errors.New("force requires a version: migrate force <version>")
		}
		value, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid version %q: %w", args[1], err)
		}
		if err := database.MigrateForce(dsn, value); err != nil {
			return err
		}
		return printVersion(sqlDB)

	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

func printVersion(sqlDB *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	version, dirty, err := database.MigrationVersion(ctx, sqlDB)
	if err != nil {
		return err
	}

	fmt.Printf("version=%d dirty=%t\n", version, dirty)
	return nil
}
