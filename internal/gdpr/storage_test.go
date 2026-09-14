package gdpr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestLocalStorageRoundTrip verifies that Store writes a file to disk and
// GetPath resolves the same file back, closing the GAP-GDPR-A loop:
// ProcessDataExport persists the JSON, GetMyDataExportDownload serves it.
func TestLocalStorageRoundTrip(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	ls := NewLocalStorage(tmp)

	requestID := pgtype.UUID{Bytes: [16]byte{4, 5, 6}, Valid: true}
	data := []byte(`{"user":"test","export":"data"}`)

	// Store the export file.
	url, size, err := ls.Store(t.Context(), requestID, "application/json", data)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if size != int64(len(data)) {
		t.Errorf("size = %d, want %d", size, len(data))
	}

	if !strings.HasPrefix(url, "gdpr-local://") {
		t.Errorf("url = %q, want gdpr-local:// prefix", url)
	}

	// Resolve the path back.
	path, err := ls.GetPath(requestID)
	if err != nil {
		t.Fatalf("GetPath failed: %v", err)
	}

	// Verify the file exists and contains the original data.
	got, err := os.ReadFile(path) // #nosec
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	if string(got) != string(data) {
		t.Errorf("file content = %q, want %q", got, data)
	}

	// Verify the file is under the tenant directory (path-traversal guard).
	tenantDir := filepath.Join(tmp, "")
	if !strings.HasPrefix(path, tenantDir+string(os.PathSeparator)) {
		t.Errorf("path %q escapes tenant dir %q", path, tenantDir)
	}
}

// TestLocalStorageGetPathMissingFile verifies that GetPath returns an error
// when the export file does not exist (e.g. export not yet processed).
func TestLocalStorageGetPathMissingFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	ls := NewLocalStorage(tmp)

	requestID := pgtype.UUID{Bytes: [16]byte{4, 5, 6}, Valid: true}

	_, err := ls.GetPath(requestID)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestLocalStoragePathTraversalGuard verifies that GetPath rejects paths
// that would escape the storage root. Since the filename is derived from
// the request ID (a UUID), this is defense-in-depth.
func TestLocalStoragePathTraversalGuard(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	ls := NewLocalStorage(tmp)

	requestID := pgtype.UUID{Bytes: [16]byte{4, 5, 6}, Valid: true}
	data := []byte("test")

	if _, _, err := ls.Store(t.Context(), requestID, "application/json", data); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	_, err := ls.GetPath(requestID)
	if err == nil {
		t.Fatal("expected error for cross-tenant access, got nil")
	}
}
