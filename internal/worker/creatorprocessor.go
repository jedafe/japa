// Package worker — creatorprocessor.go defines the CreatorProcessor
// interface that the five academy creator-economy Asynq tasks delegate to.
// The creator package implements this interface and registers itself at
// startup via RegisterCreatorProcessor.
//
// Ref: japaconnect-engineering/sections/36-learning-platform.md (Japa Academy)
//
//	japaconnect-engineering/adr/ADR-063-japa-academy-creator-economy.md
package worker

import "context"

// CreatorProcessor defines the contract for the Japa Academy creator
// economy background tasks (ADR-063): paid enrollment ledger crediting,
// period payout aggregation, payout execution, creator notifications, and
// university logo fetching for auto-seeded university spaces (Section 39).
type CreatorProcessor interface {
	ProcessEnrollment(ctx context.Context, userID, trackID, paymentID, source string, grossKobo int64) error
	CalculatePeriodPayouts(ctx context.Context, periodStart, periodEnd string) error
	SendInstructorPayout(ctx context.Context, payoutID string) error
	NotifyInstructorApproved(ctx context.Context, instructorID string) error
	NotifyCourseApproved(ctx context.Context, trackID string) error
	FetchLogo(ctx context.Context, spaceID, logoURL, uploadedBy string) error
}

var creatorProcessor CreatorProcessor

// RegisterCreatorProcessor registers the creator processor for the
// five academy creator-economy Asynq tasks. Call once at startup before the
// worker begins processing tasks.
func RegisterCreatorProcessor(p CreatorProcessor) {
	creatorProcessor = p
}
