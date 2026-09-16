package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// ScanProcessor implements worker.MediaScanProcessor. It reads the media
// record from the database, fetches the file from object storage, sends
// it to ClamAV, and updates the media status based on the result.
// // When ClamAV reports a virus, the media item is soft-deleted and the
// incident is logged. When the scan is clean, the item is confirmed.
// If ClamAV is unreachable, the scan is logged and the item is confirmed
// (graceful degradation — missing antivirus should not block uploads).
type ScanProcessor struct {
	svc     *Service
	scanner *ClamAVClient
}

// NewScanProcessor creates a ScanProcessor backed by the given service
// and ClamAV client.
func NewScanProcessor(svc *Service, scanner *ClamAVClient) *ScanProcessor {
	return &ScanProcessor{svc: svc, scanner: scanner}
}

// ProcessMediaScan fetches the media item, reads its file, scans it with
// ClamAV, and updates the item accordingly. It satisfies the
// worker.MediaScanProcessor interface.
func (p *ScanProcessor) ProcessMediaScan(ctx context.Context, mediaID string) error {
	if p.scanner == nil {
		slog.Warn("media:scan: no ClamAV client configured, skipping scan",
			"media_id", mediaID)
		return nil
	}

	// Parse media ID.
	var mid [16]byte
	if err := scanUUID(mediaID, mid[:]); err != nil {
		return fmt.Errorf("media:scan: invalid media_id %q: %w", mediaID, err)
	}

	// Fetch media record including for scoped writes.
	// The Asynq payload only carries media_id, but GetMediaForScan returns
	// the from the row, which is then threaded into confirm/reject
	// operations to prevent cross-platform modification.
	m, err := p.svc.getMediaForScan(ctx, mid[:])
	if err != nil {
		return fmt.Errorf("media:scan: fetch media %s: %w", mediaID, err)
	}

	// Open the file from object storage.
	file, err := openLocalFile(m.StorageBucket, m.StoragePath)
	if err != nil {
		return fmt.Errorf("media:scan: open file %s/%s: %w", m.StorageBucket, m.StoragePath, err)
	}

	defer func() { _ = file.Close() }()

	// Send to ClamAV.
	result, err := p.scanner.Scan(file)
	if err != nil {
		// ClamAV unreachable — log and confirm the media item rather
		// than blocking uploads. This matches the graceful-degradation
		// pattern used throughout the codebase.
		slog.Error("media:scan: ClamAV error, confirming without scan",
			"media_id", mediaID, "error", err)
		return p.svc.confirmMediaByID(ctx, mid[:])
	}

	if result.Clean {
		slog.Info("media:scan: clean", "media_id", mediaID)
		return p.svc.confirmMediaByID(ctx, mid[:])
	}

	// Infected — soft-delete and log the incident.
	slog.Error(
		"media:scan: virus detected, rejecting upload",
		"media_id", mediaID,
		"virus", result.VirusName,
		"filename", m.OriginalFilename,
		"uploader", m.UploaderUserID,
	)

	if err := p.svc.rejectMediaByID(ctx, mid[:], result.VirusName); err != nil {
		return fmt.Errorf("media:scan: reject infected media %s: %w", mediaID, err)
	}

	return nil
}

// scanUUID is a helper to scan a UUID string into a byte slice.
func scanUUID(s string, _ []byte) error {
	var id [16]byte
	// Simple UUID parsing: remove dashes and parse hex.
	if len(s) == 36 && s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-' {
		hex := s[:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
		return scanHex(hex, id[:])
	}
	// Try as raw hex (no dashes).
	if len(s) == 32 {
		return scanHex(s, id[:])
	}

	return fmt.Errorf("invalid UUID format: %s", s)
}

func scanHex(s string, dst []byte) error {
	if len(s) != 32 {
		return fmt.Errorf("hex string must be 32 characters, got %d", len(s))
	}

	for i := range 16 {
		hi := unhex(s[i*2])
		lo := unhex(s[i*2+1])

		if hi < 0 || lo < 0 {
			return errors.New("invalid hex character in UUID")
		}

		// #nosec G115 // value bounded by caller
		dst[i] = byte(hi<<4 | lo)
	}

	return nil
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}

	return -1
}
