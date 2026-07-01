package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func RunMigrations(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	migrations, err := LoadMigrations()
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if err := runMigration(ctx, db, migration); err != nil {
			return err
		}
	}

	return nil
}

func LoadMigrations() ([]Migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := migrationVersion(entry.Name())
		if err != nil {
			return nil, err
		}

		path := "migrations/" + entry.Name()
		contents, err := migrationFiles.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    entry.Name(),
			SQL:     string(contents),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func runMigration(ctx context.Context, db *sql.DB, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.Name, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	var applied bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)
	`, migration.Version).Scan(&applied); err != nil {
		return fmt.Errorf("check migration %s: %w", migration.Name, err)
	}

	if applied {
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.Name, err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name) VALUES ($1, $2)
	`, migration.Version, migration.Name); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.Name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.Name, err)
	}

	return nil
}

func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %s must start with a numeric version", name)
	}

	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration %s has invalid version: %w", name, err)
	}
	return version, nil
}
