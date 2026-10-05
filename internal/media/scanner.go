package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// ScanProcessor implements worker.MediaScanProcessor. It reads the media
// record from the database, fetches the file from object storage, sends
// it to ClamAV, and updates the media status based on the result.
// When ClamAV reports a virus, the media item is soft-deleted and the
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

	// Fetch the media record — the Asynq payload only carries media_id.
	m, err := p.svc.getMediaForScan(ctx, mid[:])
	if err != nil {
		return fmt.Errorf("media:scan: fetch media %s: %w", mediaID, err)
	}

	// Fetch the object — S3 when configured, local disk otherwise. This
	// must NOT use openLocalFile alone: presigned uploads and ingestion
	// writes land in S3 on production deployments, so a local-only read
	// fails every scan and post-hoc quarantine never fires.
	data, err := p.svc.fetchObjectBytes(ctx, m.StorageBucket, m.StoragePath, MaxMediaUploadBody)
	if err != nil {
		if errors.Is(err, errObjectMissing) {
			// Pending upload whose object never arrived, or an already
			// removed object — nothing to scan; skip without retrying.
			slog.Warn("media:scan: stored object missing, skipping",
				"media_id", mediaID)
			return nil
		}
		return fmt.Errorf("media:scan: open file %s/%s: %w", m.StorageBucket, m.StoragePath, err)
	}

	// Send to ClamAV.
	result, err := p.scanner.Scan(bytes.NewReader(data))
	if err != nil {
		// ClamAV unreachable — log and confirm the media item rather
		// than blocking uploads. This matches the graceful-degradation
		// pattern used throughout the codebase. Daemon-down is a WARN
		// (deployment gap); a real scan-protocol error stays ERROR.
		if errors.Is(err, ErrClamAVUnavailable) {
			slog.Warn("media:scan: ClamAV unavailable, confirming without scan",
				"media_id", mediaID, "error", err)
		} else {
			slog.Error("media:scan: ClamAV error, confirming without scan",
				"media_id", mediaID, "error", err)
		}
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
func scanUUID(s string, dst []byte) error {
	if len(dst) < 16 {
		return errors.New("scanUUID: destination too small")
	}
	// Simple UUID parsing: remove dashes and parse hex.
	if len(s) == 36 && s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-' {
		hex := s[:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
		return scanHex(hex, dst)
	}
	// Try as raw hex (no dashes).
	if len(s) == 32 {
		return scanHex(s, dst)
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
