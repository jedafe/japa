// Package worker — universityprocessor.go defines the UniversityProcessor
// interface that the five university Asynq tasks delegate to. The university
// package implements this interface and registers itself at startup via
// RegisterUniversityProcessor.
//
// Ref: japaconnect-engineering/sections/36-learning-platform.md (Japa University)
//      japaconnect-engineering/adr/ADR-063-japa-university-creator-economy.md
package worker

import "context"

// UniversityProcessor defines the contract for the Japa University creator
// economy background tasks (ADR-063): paid enrollment ledger crediting,
// period payout aggregation, payout execution, and creator notifications.
type UniversityProcessor interface {
	ProcessEnrollment(ctx context.Context, tenantID, userID, trackID, paymentID, source string, grossKobo int64) error
	CalculatePeriodPayouts(ctx context.Context, tenantID, periodStart, periodEnd string) error
	SendInstructorPayout(ctx context.Context, tenantID, payoutID string) error
	NotifyInstructorApproved(ctx context.Context, tenantID, instructorID string) error
	NotifyCourseApproved(ctx context.Context, tenantID, trackID string) error
}

var universityProcessor UniversityProcessor

// RegisterUniversityProcessor registers the university processor for the
// five university Asynq tasks. Call once at startup before the worker begins
// processing tasks.
func RegisterUniversityProcessor(p UniversityProcessor) {
	universityProcessor = p
}
