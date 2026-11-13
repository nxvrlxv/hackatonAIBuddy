package migrator

import (
	"net/url"
	"testing"
)

func TestMigrationURLPinsBookkeepingSchema(t *testing.T) {
	result, err := migrationURL("postgres://buddy:example@localhost/buddy?sslmode=disable&search_path=buddy,public")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("x-migrations-table") != `"public"."schema_migrations"` ||
		parsed.Query().Get("x-migrations-table-quoted") != "true" {
		t.Fatal("migration table must be schema-qualified")
	}
	if parsed.Query().Get("sslmode") != "disable" || parsed.Query().Get("search_path") != "buddy,public" {
		t.Fatal("connection options must be preserved")
	}
}
