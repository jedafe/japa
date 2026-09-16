package gdpr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Storage is the interface for persisting GDPR data export files and
// retrieving their on-disk paths for download. The default implementation
// (LocalStorage) writes to data/gdpr-exports/{ID}/ and returns a
// marker URL. In production this can be replaced with an S3/SeaweedFS-backed
// adapter (ADR-048) that returns presigned URLs.
//
// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
//
//	japaconnect-engineering/sections/67-platform-data-portability.md
type Storage interface {
	// Store persists the export file and returns a download URL marker plus
	// the file size in bytes. The filename is derived deterministically from
	// the request ID so GetPath can resolve it later without storing extra
	// metadata.
	Store(ctx context.Context, requestID pgtype.UUID, mimeType string, data []byte) (url string, size int64, err error)

	// GetPath returns the absolute filesystem path for the export file
	// belonging to the given request. Used by the download handler to serve
	// the file with http.ServeFile. Returns an error if the file does not
	// exist or the resolved path escapes the storage root.
	GetPath(requestID pgtype.UUID) (string, error)
}

// exportFilename returns the deterministic filename for a GDPR data export.
// The filename is purely derived from the request ID so Store and GetPath
// agree without needing to persist extra metadata.
func exportFilename(requestID pgtype.UUID) string {
	return fmt.Sprintf("data-export-%s.json", requestID.String())
}

// LocalStorage writes GDPR data export files to
// data/gdpr-exports/{ID}/ and returns a marker URL of the form
// gdpr-local://{requestID}. Files are created with 0600 permissions because
// they contain PII.
type LocalStorage struct {
	root string
}

// NewLocalStorage creates a LocalStorage rooted at the given directory
// (default "data/gdpr-exports").
func NewLocalStorage(root string) *LocalStorage {
	if root == "" {
		root = "data/gdpr-exports"
	}

	return &LocalStorage{root: root}
}

func (ls *LocalStorage) dirForPlatform() (string, error) {
	abs, err := filepath.Abs(ls.root)
	if err != nil {
		return "", fmt.Errorf("resolving gdpr export root: %w", err)
	}

	dir := filepath.Join(abs, "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating gdpr export directory: %w", err)
	}

	return dir, nil
}

// Store implements Storage.
func (ls *LocalStorage) Store(_ context.Context, requestID pgtype.UUID, mimeType string, data []byte) (string, int64, error) {
	dir, err := ls.dirForPlatform()
	if err != nil {
		return "", 0, err
	}

	filename := exportFilename(requestID)

	full := filepath.Join(dir, filename)
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return "", 0, fmt.Errorf("writing gdpr export file: %w", err)
	}

	info, err := os.Stat(full)
	if err != nil {
		return "", 0, fmt.Errorf("statting gdpr export file: %w", err)
	}
	// The URL is a marker — the download handler resolves the file path
	// via GetPath, not by following this URL. It simply signals "file ready".
	url := "gdpr-local://" + requestID.String()
	_ = mimeType // reserved for future S3 content-type header

	return url, info.Size(), nil
}

// GetPath implements Storage.
func (ls *LocalStorage) GetPath(requestID pgtype.UUID) (string, error) {
	dir, err := ls.dirForPlatform()
	if err != nil {
		return "", err
	}

	filename := exportFilename(requestID)
	full := filepath.Join(dir, filename)
	// Path-traversal guard: ensure the resolved path stays under the platform dir.
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}

	if abs != dir && !strings.HasPrefix(abs, dir+string(os.PathSeparator)) {
		return "", errors.New("path escapes gdpr export directory")
	}

	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("gdpr export file not found: %w", err)
	}

	return abs, nil
}
