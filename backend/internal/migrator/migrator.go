package migrator

import (
	"errors"
	"fmt"

	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/nxvrlxv/ai-buddy/backend/migrations"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func Run(dbURL string) error {
	driver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrate instance error: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config loader error: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", driver, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("migrate instance error: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration error: %w", err)
	}

	defer m.Close()

	return nil
}
