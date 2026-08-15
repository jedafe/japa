package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

// TestListMigrationFilesSkipsDownMigrations is the AUDIT-BE-003 regression
// anchor: the runner once globbed every .sql file and applied .down.sql
// rollbacks forward (they sort before .up.sql), dropping data. Forward runs
// must include .up.sql and legacy plain .sql files only.
func TestListMigrationFilesSkipsDownMigrations(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{
		"000001_first.up.sql",
		"000001_first.down.sql",
		"000002_legacy_plain.sql",
		"000003_second.up.sql",
		"000003_second.down.sql",
		"README.md",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatalf("writing fixture %s: %v", name, err)
		}
	}

	if err := os.Mkdir(filepath.Join(dir, "archive"), 0o750); err != nil {
		t.Fatalf("creating fixture subdirectory: %v", err)
	}

	files, err := ListMigrationFiles(dir)
	if err != nil {
		t.Fatalf("ListMigrationFiles: %v", err)
	}

	want := []string{
		filepath.Join(dir, "000001_first.up.sql"),
		filepath.Join(dir, "000002_legacy_plain.sql"),
		filepath.Join(dir, "000003_second.up.sql"),
	}

	if len(files) != len(want) {
		t.Fatalf("got %d files %v, want %d %v", len(files), files, len(want), want)
	}

	for i, f := range files {
		if f != want[i] {
			t.Errorf("files[%d] = %q, want %q (order must be sorted, .down.sql excluded)", i, f, want[i])
		}
	}
}
