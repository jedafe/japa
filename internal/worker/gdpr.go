package worker

import (
	"context"
)

// GDPRProcessor defines the interface for GDPR background task processing.
// The GDPR service implements this interface and registers with the worker.
type GDPRProcessor interface {
	ProcessDataExport(ctx context.Context, requestID [16]byte) error
	ProcessAccountDeletion(ctx context.Context, requestID [16]byte) error
	// CleanupExpiredExports deletes files and nulls download URLs for
	// completed data export requests whose ExpiresAt has passed
	// (BUG-GDPR-005, Section 19 time-limited download links).
	CleanupExpiredExports(ctx context.Context) error
}

var gdprProcessor GDPRProcessor

// RegisterGDPRProcessor registers the GDPR service processor for background tasks.
func RegisterGDPRProcessor(p GDPRProcessor) {
	gdprProcessor = p
}
