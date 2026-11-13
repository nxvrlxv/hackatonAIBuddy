package migrator

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/nxvrlxv/ai-buddy/backend/migrations"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Run applies pending embedded up migrations. An up-to-date database is a success.
// Only the database URL is needed; application/LLM settings are not required.
func Run(dbURL string) (resultErr error) {
	if strings.TrimSpace(dbURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	dbURL, err := migrationURL(dbURL)
	if err != nil {
		return err
	}
	driver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", driver, dbURL)
	if err != nil {
		_ = driver.Close()
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := m.Close()
		resultErr = errors.Join(resultErr, sourceErr, databaseErr)
	}()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// migrationURL pins bookkeeping to public even after schema "$user" appears.
func migrationURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return "", errors.New("DATABASE_URL must be a valid PostgreSQL URL")
	}
	query := parsed.Query()
	query.Set("x-migrations-table", `"public"."schema_migrations"`)
	query.Set("x-migrations-table-quoted", "true")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
