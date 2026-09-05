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
	accountID  string
	licenseKey string
	dbPath     string
	reader     *Reader
}

// NewUpdater creates an Updater that downloads updates using the
// given MaxMind account ID and license key, and stores the database at
// dbPath. The reader is used to atomically swap in the new database.
//
// MaxMind's download endpoint requires the Account ID as the Basic Auth
// username and the License Key as the password. If accountID is empty,
// the license key is used as both (backward compatibility with older
// configs that only set JC_MAXMIND_LICENSE_KEY).
func NewUpdater(accountID, licenseKey, dbPath string, reader *Reader) *Updater {
	return &Updater{
		accountID:  accountID,
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

	// MaxMind requires a User-Agent header; requests without one may be
	// rejected with HTTP 403.
	req.Header.Set("User-Agent", "JapaConnect-GeoIP-Updater/1.0")

	// MaxMind's download endpoint uses Basic Auth with the Account ID as
	// the username and the License Key as the password. Fall back to the
	// license key as the username when no account ID is configured (older
	// configs that only set JC_MAXMIND_LICENSE_KEY).
	username := u.accountID
	if username == "" {
		username = u.licenseKey
	}

	req.SetBasicAuth(username, u.licenseKey)

	client := &http.Client{Timeout: 5 * time.Minute}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading geoip database: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// The download is a .tar.gz archive containing the .mmdb file.
	// Extract to a temporary file, then move it into place before swapping
	// the reader. Renaming first keeps dbPath as the on-disk source of
	// truth: if the rename fails the previous database is still intact and
	// no swap occurs, so the live reader never points at a tmp file that a
	// later update could overwrite.
	tmpPath := u.dbPath + ".tmp"
	if err := extractMMDB(resp.Body, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("extracting mmdb: %w", err)
	}

	// Move the temp file to the final location before opening it, so the
	// reader always owns dbPath (not tmpPath).
	if err := os.Rename(tmpPath, u.dbPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("moving database file: %w", err)
	}

	// Open the newly installed database and atomically swap it in.
	newReader, err := geoip2.Open(u.dbPath)
	if err != nil {
		return fmt.Errorf("opening new database: %w", err)
	}

	u.reader.Swap(newReader)

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
