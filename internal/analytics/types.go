// Package analytics provides platform analytics and site reporting for
// JapaConnect (Section 40). It tracks platform events, refreshes
// materialized views for dashboard queries, and runs security scans.
package analytics

import "time"

// EventType identifies the kind of platform event being tracked.
type EventType string

const (
	// EventContentCreated is the event content created constant.
	EventContentCreated EventType = "content.created"
	// EventContentViewed is the event content viewed.
	EventContentViewed EventType = "content.viewed"
	// EventSpaceJoined is the event space joined.
	EventSpaceJoined EventType = "space.joined"
	// EventSpaceFollowed is the event space followed.
	EventSpaceFollowed EventType = "space.followed"
	// EventTagFollowed is the event tag followed.
	EventTagFollowed EventType = "tag.followed"
	// EventUserRegistered is the event user registered.
	EventUserRegistered EventType = "user.registered"
	// EventPaymentCompleted is the event payment completed.
	EventPaymentCompleted EventType = "payment.completed"
	// EventCourseEnrolled is the event course enrolled.
	EventCourseEnrolled EventType = "course.enrolled"
	// EventReactionGiven is the event reaction given.
	EventReactionGiven EventType = "reaction.given"
	// EventCommentCreated is the event comment created.
	EventCommentCreated EventType = "comment.created"
	// EventAdViewed is the event ad viewed.
	EventAdViewed EventType = "ad.viewed"
	// EventAdClicked is the event ad clicked.
	EventAdClicked EventType = "ad.clicked"
	// EventSearchPerformed is the event search performed.
	EventSearchPerformed EventType = "search.performed"
	// EventDMSent is the event d m sent.
	EventDMSent EventType = "dm.sent"
	// EventTicketSubmitted is the event ticket submitted.
	EventTicketSubmitted EventType = "ticket.submitted"
	// EventFollowCreated is the event follow created.
	EventFollowCreated EventType = "follow.created"
	// EventShareClicked is the event share clicked.
	EventShareClicked EventType = "share.clicked"
)

// ValidEventTypes returns all recognised event types.
func ValidEventTypes() []EventType {
	return []EventType{
		EventContentCreated,
		EventContentViewed,
		EventSpaceJoined,
		EventSpaceFollowed,
		EventTagFollowed,
		EventUserRegistered,
		EventPaymentCompleted,
		EventCourseEnrolled,
		EventReactionGiven,
		EventCommentCreated,
		EventAdViewed,
		EventAdClicked,
		EventSearchPerformed,
		EventDMSent,
		EventTicketSubmitted,
		EventFollowCreated,
		EventShareClicked,
	}
}

// ScanSeverity classifies the severity of a security scan finding.
type ScanSeverity string

const (
	// SeverityInfo is the severity info constant.
	SeverityInfo ScanSeverity = "info"
	// SeverityWarning is the severity warning.
	SeverityWarning ScanSeverity = "warning"
	// SeverityCritical is the severity critical.
	SeverityCritical ScanSeverity = "critical"
)

// ScanType identifies which security check produced a finding.
type ScanType string

const (
	// ScanDependencyAudit is the scan dependency audit constant.
	ScanDependencyAudit ScanType = "dependency_audit"
	// ScanFailedLoginSpike is the scan failed login spike.
	ScanFailedLoginSpike ScanType = "failed_login_spike"
	// ScanLargeErrorRate is the scan large error rate.
	ScanLargeErrorRate ScanType = "large_error_rate"
	// ScanSLABreachCount is the scan s l a breach count.
	ScanSLABreachCount ScanType = "sla_breach_count"
	// ScanSuspiciousIPs is the scan suspicious i ps.
	ScanSuspiciousIPs ScanType = "suspicious_ip_patterns"
	// ScanOrphanedMedia is the scan orphaned media.
	ScanOrphanedMedia ScanType = "orphaned_media"
	// ScanStaleTokens is the scan stale tokens.
	ScanStaleTokens ScanType = "stale_tokens"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// EventFilter holds optional filters for listing platform events.
type EventFilter struct {
	EventType  string
	UserID     string
	EntityType string
	StartDate  *time.Time
	EndDate    *time.Time
	Page       int32
	Limit      int32
}

// SecurityScanFilter holds optional filters for listing scan results.
type SecurityScanFilter struct {
	Severity   string
	Unresolved bool
	Page       int32
	Limit      int32
}
