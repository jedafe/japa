// Package gdpr implements GDPR/NDPR Compliance for JapaConnect (Section 19).
//
// Two-layer architecture: consent management, data export, account deletion
// with anonymization, and audit logging. Asynq background workers handle
// data export generation and account deletion cleanup.
//
// Status state machine for data exports and deletion requests:
//
//	pending → processing → completed
//	pending → processing → failed
package gdpr

// ConsentType represents the category of user consent.
type ConsentType string

const (
	// ConsentMarketingEmail is the consent marketing email constant.
	ConsentMarketingEmail ConsentType = "marketing_email"
	// ConsentAnalyticsTracking is the consent analytics tracking.
	ConsentAnalyticsTracking ConsentType = "analytics_tracking"
	// ConsentThirdPartySharing is the consent third party sharing.
	ConsentThirdPartySharing ConsentType = "third_party_sharing"
	// ConsentAIProcessing is the consent a i processing.
	ConsentAIProcessing ConsentType = "ai_processing"
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
	// ExportPending is the export pending constant.
	ExportPending ExportStatus = "pending"
	// ExportProcessing is the export processing.
	ExportProcessing ExportStatus = "processing"
	// ExportCompleted is the export completed.
	ExportCompleted ExportStatus = "completed"
	// ExportFailed is the export failed.
	ExportFailed ExportStatus = "failed"
)

// DeletionStatus represents the lifecycle state of an account deletion request.
type DeletionStatus string

const (
	// DeletionPending is the deletion pending constant.
	DeletionPending DeletionStatus = "pending"
	// DeletionProcessing is the deletion processing.
	DeletionProcessing DeletionStatus = "processing"
	// DeletionRetentionWait means the user is anonymized but hard delete is
	// waiting for the retention period to elapse.
	DeletionRetentionWait DeletionStatus = "retention_wait"
	// DeletionCompleted is the deletion completed.
	DeletionCompleted DeletionStatus = "completed"
	// DeletionFailed is the deletion failed.
	DeletionFailed DeletionStatus = "failed"
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
