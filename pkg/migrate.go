package pkg

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationStatus is emitted to the frontend to show progress.
type MigrationStatus struct {
	Running     bool   `json:"running"`
	Current     int    `json:"current"`
	Total       int    `json:"total"`
	Description string `json:"description"`
}

// MigrationResult describes what happened when migrations ran.
type MigrationResult struct {
	Applied int // number of migrations actually applied
	Total   int // total migration files available
}

// RunMigrations runs all pending migrations one step at a time,
// calling onProgress for each step so the frontend can show a progress bar.
// Returns the count of applied migrations. If no migrations are pending, returns 0.
func RunMigrations(databaseURL string, onProgress func(MigrationStatus)) (*MigrationResult, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to open migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}
	defer m.Close()

	// Count total available migrations by scanning the embedded FS.
	totalMigrations := countMigrationFiles()

	// Get current version to determine pending count.
	currentVersion, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return nil, fmt.Errorf("failed to get migration version: %w", err)
	}
	if dirty {
		return nil, fmt.Errorf("database is in a dirty migration state at version %d — fix manually", currentVersion)
	}

	startVersion := int(currentVersion) // 0 if ErrNilVersion (no migrations run yet)
	pending := totalMigrations - startVersion

	if pending <= 0 {
		return &MigrationResult{Applied: 0, Total: totalMigrations}, nil
	}

	// Notify that migrations are starting.
	if onProgress != nil {
		onProgress(MigrationStatus{
			Running:     true,
			Current:     0,
			Total:       pending,
			Description: fmt.Sprintf("Running %d migration(s)…", pending),
		})
	}

	// Apply one step at a time so we can report progress.
	applied := 0
	for {
		err := m.Steps(1)
		if errors.Is(err, migrate.ErrNoChange) || errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("migration step failed: %w", err)
		}

		applied++
		if onProgress != nil {
			ver, _, _ := m.Version()
			desc := fmt.Sprintf("Applied migration %d of %d (v%d)", applied, pending, ver)
			onProgress(MigrationStatus{
				Running:     true,
				Current:     applied,
				Total:       pending,
				Description: desc,
			})
		}
	}

	// Signal completion.
	if onProgress != nil {
		onProgress(MigrationStatus{
			Running:     false,
			Current:     applied,
			Total:       pending,
			Description: fmt.Sprintf("Migrations complete — %d applied", applied),
		})
	}

	return &MigrationResult{Applied: applied, Total: totalMigrations}, nil
}

// countMigrationFiles counts the number of *.up.sql files in the embedded migrations dir.
func countMigrationFiles() int {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			count++
		}
	}
	return count
}
