package database

import "testing"

func TestMigrateRejectsNilDatabase(t *testing.T) {
	if err := Migrate(nil); err == nil {
		t.Fatal("expected nil database error")
	}
}
