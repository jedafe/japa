// Package gdpr implements GDPR/NDPR Compliance for JapaConnect (Section 19).
//
// Two-layer architecture: consent management, data export, account deletion
// with anonymization, and audit logging. Asynq background workers handle
// data export generation and account deletion cleanup.
//
// Status state machine for data exports and deletion requests:
//   pending → processing → completed
//   pending → processing → failed
package gdpr

// ConsentType represents the category of user consent.
type ConsentType string

const (
	ConsentMarketingEmail    ConsentType = "marketing_email"
	ConsentAnalyticsTracking ConsentType = "analytics_tracking"
	ConsentThirdPartySharing ConsentType = "third_party_sharing"
	ConsentAIProcessing     ConsentType = "ai_processing"
)

// AllConsentTypes returns all valid consent types.
func AllConsentTypes() []ConsentType {
	return []ConsentType{
		ConsentMarketingEmail,
		ConsentAnalyticsTracking,
		ConsentThirdPartySharing,
		ConsentAIProcessing,
	}
}

// ExportStatus represents the lifecycle state of a data export request.
type ExportStatus string

const (
	ExportPending    ExportStatus = "pending"
	ExportProcessing ExportStatus = "processing"
	ExportCompleted  ExportStatus = "completed"
	ExportFailed     ExportStatus = "failed"
)

// DeletionStatus represents the lifecycle state of an account deletion request.
type DeletionStatus string

const (
	DeletionPending    DeletionStatus = "pending"
	DeletionProcessing DeletionStatus = "processing"
	DeletionCompleted  DeletionStatus = "completed"
	DeletionFailed     DeletionStatus = "failed"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ValidConsentType checks whether a string is a valid consent type.
func ValidConsentType(s string) bool {
	for _, ct := range AllConsentTypes() {
		if string(ct) == s {
			return true
		}
	}
	return false
}
