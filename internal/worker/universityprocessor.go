// Package worker — universityprocessor.go defines the UniversityProcessor
// interface that the five university Asynq tasks delegate to. The university
// package implements this interface and registers itself at startup via
// RegisterUniversityProcessor.
//
// Ref: japaconnect-engineering/sections/36-learning-platform.md (Japa University)
//
//	japaconnect-engineering/adr/ADR-063-japa-university-creator-economy.md
package worker

import "context"

// UniversityProcessor defines the contract for the Japa University creator
// economy background tasks (ADR-063): paid enrollment ledger crediting,
// period payout aggregation, payout execution, creator notifications, and
// university logo fetching for auto-seeded university spaces (Section 39).
type UniversityProcessor interface {
	ProcessEnrollment(ctx context.Context, userID, trackID, paymentID, source string, grossKobo int64) error
	CalculatePeriodPayouts(ctx context.Context, periodStart, periodEnd string) error
	SendInstructorPayout(ctx context.Context, payoutID string) error
	NotifyInstructorApproved(ctx context.Context, instructorID string) error
	NotifyCourseApproved(ctx context.Context, trackID string) error
	FetchLogo(ctx context.Context, spaceID, logoURL, uploadedBy string) error
}

var universityProcessor UniversityProcessor

// RegisterUniversityProcessor registers the university processor for the
// five university Asynq tasks. Call once at startup before the worker begins
// processing tasks.
func RegisterUniversityProcessor(p UniversityProcessor) {
	universityProcessor = p
}
