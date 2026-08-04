package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

func Migrate(databaseURL string) error {
	source, err := iofs.New(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		if isDirtyErr(err) {
			if fixErr := recoverDirtyMigration(m, databaseURL); fixErr != nil {
				return fmt.Errorf("migrate up: %w (recovery failed: %v)", err, fixErr)
			}
			if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
				return fmt.Errorf("migrate up after dirty recovery: %w", err)
			}
			log.Printf("database migrations recovered from dirty state")
			return nil
		}
		return fmt.Errorf("migrate up: %w", err)
	}

	if v, dirty, err := m.Version(); err == nil {
		log.Printf("database migrations at version %d (dirty=%v)", v, dirty)
	}
	return nil
}

func isDirtyErr(err error) bool {
	if err == nil {
		return false
	}
	// golang-migrate: "Dirty database version N. Fix and force version."
	return strings.Contains(err.Error(), "Dirty database version")
}

// recoverDirtyMigration clears a dirty flag when it is safe, then re-runs Up.
// Safe means the failed version's expected objects already exist (partial apply),
// or we can force back one version and re-apply an idempotent migration.
func recoverDirtyMigration(m *migrate.Migrate, databaseURL string) error {
	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return err
	}
	if !dirty {
		return nil
	}
	log.Printf("dirty migration detected at version %d; attempting recovery", version)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Version 2 introduces annotations. If the table is present, force clean at 2.
	// If not, force back to 1 so Up() can re-apply 000002 (now idempotent).
	if version == 2 {
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = 'annotations'
			)
		`).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			log.Printf("annotations table present; forcing schema_migrations clean at version 2")
			return m.Force(2)
		}
		log.Printf("annotations table missing; forcing schema_migrations back to version 1 for re-apply")
		return m.Force(1)
	}

	// Generic fallback: force clean at current version so a re-Up is a no-op when complete.
	// Prefer Force(version) only when we cannot safely step back.
	log.Printf("forcing clean at version %d (generic dirty recovery)", version)
	return m.Force(int(version))
}
