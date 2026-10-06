// Package worker — academyprocessor.go defines the AcademyProcessor
// interface that the academy background tasks delegate to. The academy
// package (or a noop implementation) implements this interface and registers
// itself at startup via RegisterAcademyProcessor.
package worker

import "context"

// AcademyProcessor defines the contract for academy background
// tasks — computing track completion percentages and issuing certificates.
type AcademyProcessor interface {
	UpdateTrackCompletion(ctx context.Context, userID, trackID string) error
	IssueCertificate(ctx context.Context, userID, trackID, courseName string) error
}

var academyProcessor AcademyProcessor

// RegisterAcademyProcessor registers the academy processor for the
// academy background tasks. Call once at startup before the worker
// begins processing tasks.
func RegisterAcademyProcessor(p AcademyProcessor) {
	academyProcessor = p
}
