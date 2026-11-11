package migrations

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/stub"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func TestEverySQLFileHasValidNameAndCounterpart(t *testing.T) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	driver, err := iofs.New(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parsed, err := source.DefaultParse(entry.Name())
		if err != nil {
			t.Fatalf("migration would be silently ignored: %s: %v", entry.Name(), err)
		}
		for _, direction := range []source.Direction{source.Up, source.Down} {
			var reader io.ReadCloser
			if direction == source.Up {
				reader, _, err = driver.ReadUp(parsed.Version)
			} else {
				reader, _, err = driver.ReadDown(parsed.Version)
			}
			if err != nil {
				t.Fatalf("missing %s for version %d: %v", direction, parsed.Version, err)
			}
			body, readErr := io.ReadAll(reader)
			reader.Close()
			if readErr != nil || len(strings.TrimSpace(string(body))) == 0 {
				t.Fatalf("empty or unreadable migration %s", entry.Name())
			}
		}
	}
}

func TestEmbeddedMigrationSequence(t *testing.T) {
	driver, err := iofs.New(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	database, err := stub.WithInstance(nil, &stub.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithInstance("iofs", driver, "stub", database)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	version, dirty, err := m.Version()
	if err != nil || version != 3 || dirty {
		t.Fatalf("expected clean version 3: %d %v %v", version, dirty, err)
	}
	if err := m.Up(); !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("repeat up: %v", err)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	version, dirty, err = m.Version()
	if err != nil || version != 2 || dirty {
		t.Fatalf("rollback: %d %v %v", version, dirty, err)
	}
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
}
