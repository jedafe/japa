package geoip

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oschwald/geoip2-golang"
)

// UpdateURL is the MaxMind GeoLite2-City download endpoint.
// Requires a license key (free MaxMind account).
const UpdateURL = "https://download.maxmind.com/geoip/databases/GeoLite2-City/update"

// Updater handles downloading and applying GeoLite2 database updates.
type Updater struct {
	licenseKey string
	dbPath     string
	reader     *Reader
}

// NewUpdater creates an Updater that downloads updates using the
// given MaxMind license key and stores the database at dbPath.
// The reader is used to atomically swap in the new database.
func NewUpdater(licenseKey, dbPath string, reader *Reader) *Updater {
	return &Updater{
		licenseKey: licenseKey,
		dbPath:     dbPath,
		reader:     reader,
	}
}

// DownloadAndUpdate downloads the latest GeoLite2-City database,
// extracts the .mmdb file, and atomically swaps it into the reader.
// This is called by the maintenance:update_geoip_db Asynq task
// (weekly, Sunday 02:00 UTC).
func (u *Updater) DownloadAndUpdate() error {
	if u.licenseKey == "" {
		return errors.New("JC_MAXMIND_LICENSE_KEY is not set - cannot update geoip database")
	}

	// Download the database archive
	req, err := http.NewRequest(http.MethodGet, UpdateURL, nil)
	if err != nil {
		return fmt.Errorf("creating update request: %w", err)
	}

	req.SetBasicAuth(u.licenseKey, u.licenseKey)

	client := &http.Client{Timeout: 5 * time.Minute}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading geoip database: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// The download is a .tar.gz archive containing the .mmdb file
	// Extract to a temporary file, then swap
	tmpPath := u.dbPath + ".tmp"
	if err := extractMMDB(resp.Body, tmpPath); err != nil {
		return fmt.Errorf("extracting mmdb: %w", err)
	}

	// Open the new database
	newReader, err := geoip2.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("opening new database: %w", err)
	}

	// Atomic swap
	u.reader.Swap(newReader)

	// Move temp file to final location
	if err := os.Rename(tmpPath, u.dbPath); err != nil {
		return fmt.Errorf("moving database file: %w", err)
	}

	return nil
}

// extractMMDB extracts the .mmdb file from a .tar.gz stream.
func extractMMDB(r io.Reader, outputPath string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}

	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		if filepath.Ext(header.Name) == ".mmdb" {
			out, err := os.Create(outputPath) //nolint:gosec // G304: outputPath derived from trusted dbPath config
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}

			// Limit decompression to 200MB to prevent decompression bomb attacks.
			// GeoLite2-City is typically ~60-80MB; 200MB provides headroom.
			const maxMMDBSize = 200 * 1024 * 1024
			if _, err := io.Copy(out, io.LimitReader(tr, maxMMDBSize)); err != nil {
				_ = out.Close()
				return fmt.Errorf("writing mmdb: %w", err)
			}

			return out.Close()
		}
	}

	return errors.New("no .mmdb file found in archive")
}
