package worker

import (
	"context"
)

// GDPRProcessor defines the interface for GDPR background task processing.
// The GDPR service implements this interface and registers with the worker.
type GDPRProcessor interface {
	ProcessDataExport(ctx context.Context, requestID [16]byte) error
	ProcessAccountDeletion(ctx context.Context, requestID [16]byte) error
}

var gdprProcessor GDPRProcessor

// RegisterGDPRProcessor registers the GDPR service processor for background tasks.
func RegisterGDPRProcessor(p GDPRProcessor) {
	gdprProcessor = p
}
