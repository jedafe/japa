package services

import "time"

// FreshnessScore computes a 0-100 freshness score based on age.
// 100 = today, 90 = this week, 70 = this month, 40 = older, 0 = stale (>90 days).
func FreshnessScore(verifiedAt, now time.Time) int {
	age := now.Sub(verifiedAt)

	switch {
	case age <= 24*time.Hour:
		return 100
	case age <= 7*24*time.Hour:
		return 90
	case age <= 30*24*time.Hour:
		return 70
	case age <= 90*24*time.Hour:
		return 40
	default:
		return 0
	}
}
