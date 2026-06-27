// Package analytics provides platform analytics and site reporting for
// JapaConnect (Section 40). It tracks platform events, refreshes
// materialized views for dashboard queries, and runs security scans.
package analytics

import "time"

// EventType identifies the kind of platform event being tracked.
type EventType string

const (
	EventContentCreated   EventType = "content.created"
	EventContentViewed    EventType = "content.viewed"
	EventSpaceJoined      EventType = "space.joined"
	EventSpaceFollowed    EventType = "space.followed"
	EventTagFollowed      EventType = "tag.followed"
	EventUserRegistered   EventType = "user.registered"
	EventPaymentCompleted EventType = "payment.completed"
	EventCourseEnrolled   EventType = "course.enrolled"
	EventReactionGiven    EventType = "reaction.given"
	EventCommentCreated   EventType = "comment.created"
	EventAdViewed         EventType = "ad.viewed"
	EventAdClicked        EventType = "ad.clicked"
	EventSearchPerformed  EventType = "search.performed"
	EventDMSent           EventType = "dm.sent"
	EventTicketSubmitted  EventType = "ticket.submitted"
	EventFollowCreated    EventType = "follow.created"
	EventShareClicked     EventType = "share.clicked"
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
	SeverityInfo     ScanSeverity = "info"
	SeverityWarning  ScanSeverity = "warning"
	SeverityCritical ScanSeverity = "critical"
)

// ScanType identifies which security check produced a finding.
type ScanType string

const (
	ScanDependencyAudit  ScanType = "dependency_audit"
	ScanFailedLoginSpike ScanType = "failed_login_spike"
	ScanLargeErrorRate   ScanType = "large_error_rate"
	ScanSLABreachCount   ScanType = "sla_breach_count"
	ScanSuspiciousIPs    ScanType = "suspicious_ip_patterns"
	ScanOrphanedMedia    ScanType = "orphaned_media"
	ScanStaleTokens      ScanType = "stale_tokens"
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
