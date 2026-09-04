package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// MigrateUp applies all pending migrations.
func (db *Database) MigrateUp(ctx context.Context) error {
	return db.runMigrations(ctx, "up")
}

// MigrateDown rolls back the last applied migration.
func (db *Database) MigrateDown(ctx context.Context) error {
	return db.runMigrations(ctx, "down")
}

func (db *Database) runMigrations(ctx context.Context, direction string) error {
	// Create tracking table if not exists
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	files, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var upFiles []string
	var downFiles []string
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".up.sql") {
			upFiles = append(upFiles, f.Name())
		} else if strings.HasSuffix(f.Name(), ".down.sql") {
			downFiles = append(downFiles, f.Name())
		}
	}
	sort.Strings(upFiles)
	// For down, we sort in reverse (handled during execution)
	sort.Sort(sort.Reverse(sort.StringSlice(downFiles)))

	return db.RunInTx(ctx, func(tx *sql.Tx) error {
		var currentVersion int
		err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
		if err != nil {
			return err
		}

		if direction == "up" {
			for _, f := range upFiles {
				version := extractVersion(f)
				if version > currentVersion {
					slog.Info("Applying migration", "file", f)
					content, err := migrationFiles.ReadFile("migrations/" + f)
					if err != nil {
						return err
					}
					if _, err := tx.ExecContext(ctx, string(content)); err != nil {
						return fmt.Errorf("failed to apply %s: %w", f, err)
					}
					if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
						return err
					}
				}
			}
		} else if direction == "down" {
			for _, f := range downFiles {
				version := extractVersion(f)
				if version == currentVersion {
					slog.Info("Rolling back migration", "file", f)
					content, err := migrationFiles.ReadFile("migrations/" + f)
					if err != nil {
						return err
					}
					if _, err := tx.ExecContext(ctx, string(content)); err != nil {
						return fmt.Errorf("failed to apply %s: %w", f, err)
					}
					if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
						return err
					}
					break // Only rollback one version at a time
				}
			}
		}
		return nil
	})
}

func extractVersion(filename string) int {
	var version int
	fmt.Sscanf(filename, "%d_", &version)
	return version
}
