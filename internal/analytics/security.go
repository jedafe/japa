package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// scanFailedLoginSpike checks if failed logins in the last hour exceed threshold.
// BUG-ANALYTICS-06 fix: query audit_logs (action_type = 'auth.login_failed')
// instead of platform_events, which never contained that event type.
func (s *Service) scanFailedLoginSpike(ctx context.Context, tenantID pgtype.UUID) error {
	oneHourAgo := time.Now().Add(-1 * time.Hour)

	var count int64

	err := s.db.Reader.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM audit_logs
		WHERE tenant_id = $1
		  AND action_type = 'auth.login_failed'
		  AND created_at >= $2
	`, tenantID, oneHourAgo).Scan(&count)
	if err != nil {
		return fmt.Errorf("counting failed logins: %w", err)
	}

	threshold := int64(50)
	if count >= threshold {
		meta, _ := json.Marshal(map[string]any{
			"failed_count": count,
			"threshold":    threshold,
			"period":       "1h",
		})

		_, err = s.store.CreateSecurityScanResult(ctx, gen.CreateSecurityScanResultParams{
			TenantID: tenantID,
			ScanType: string(ScanFailedLoginSpike),
			Severity: string(SeverityWarning),
			Finding:  fmt.Sprintf("Failed login spike: %d attempts in the last hour (threshold: %d)", count, threshold),
			Meta:     meta,
		})
		if err != nil {
			return fmt.Errorf("recording failed login spike: %w", err)
		}
	}

	return nil
}

// scanSLABreachCount checks for SLA breaches in the ticket system.
// BUG-ANALYTICS-07 fix: query the tickets table for actual SLA breaches
// (sla_breach_at IS NOT NULL AND status is open) instead of counting
// previous security_scan_results findings (which was circular).
func (s *Service) scanSLABreachCount(ctx context.Context, tenantID pgtype.UUID) error {
	var count int64

	err := s.db.Reader.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tickets
		WHERE tenant_id = $1
		  AND sla_breach_at IS NOT NULL
		  AND sla_breach_at < now()
		  AND status NOT IN ('resolved', 'closed')
	`, tenantID).Scan(&count)
	if err != nil {
		return fmt.Errorf("counting SLA breaches: %w", err)
	}

	if count > 5 {
		meta, _ := json.Marshal(map[string]any{
			"breach_count": count,
			"period":       "current",
		})

		_, err = s.store.CreateSecurityScanResult(ctx, gen.CreateSecurityScanResultParams{
			TenantID: tenantID,
			ScanType: string(ScanSLABreachCount),
			Severity: string(SeverityCritical),
			Finding:  fmt.Sprintf("High SLA breach count: %d unresolved open tickets with breached SLA", count),
			Meta:     meta,
		})
		if err != nil {
			return fmt.Errorf("recording SLA breach count: %w", err)
		}
	}

	return nil
}

// scanStaleTokens checks for refresh tokens older than 30 days.
func (s *Service) scanStaleTokens(ctx context.Context, tenantID pgtype.UUID) error {
	var count int64

	err := s.db.Reader.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM refresh_tokens
		WHERE tenant_id = $1
		  AND created_at < now() - interval '30 days'
		  AND revoked_at IS NULL
	`, tenantID).Scan(&count)
	if err != nil {
		// Table may not exist or have different schema — treat as non-fatal.
		return nil //nolint:nilerr // intentional: degradable security check
	}

	if count > 100 {
		meta, _ := json.Marshal(map[string]any{
			"stale_count": count,
			"max_age":     "30d",
		})

		_, err = s.store.CreateSecurityScanResult(ctx, gen.CreateSecurityScanResultParams{
			TenantID: tenantID,
			ScanType: string(ScanStaleTokens),
			Severity: string(SeverityWarning),
			Finding:  fmt.Sprintf("Stale tokens detected: %d unrevoked tokens older than 30 days", count),
			Meta:     meta,
		})
		if err != nil {
			return fmt.Errorf("recording stale tokens: %w", err)
		}
	}

	return nil
}

// scanUnverifiedUsers checks for users who haven't verified their email.
func (s *Service) scanUnverifiedUsers(ctx context.Context, tenantID pgtype.UUID) error {
	var count int64

	err := s.db.Reader.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM users
		WHERE tenant_id = $1
		  AND email_verified_at IS NULL
		  AND created_at < now() - interval '7 days'
	`, tenantID).Scan(&count)
	if err != nil {
		return nil //nolint:nilerr // table may not exist
	}

	if count > 10 {
		meta, _ := json.Marshal(map[string]any{
			"unverified_count": count,
			"threshold":        "7 days",
		})

		_, err = s.store.CreateSecurityScanResult(ctx, gen.CreateSecurityScanResultParams{
			TenantID: tenantID,
			ScanType: "unverified_users",
			Severity: string(SeverityInfo),
			Finding:  fmt.Sprintf("Unverified users: %d users with unverified email older than 7 days", count),
			Meta:     meta,
		})
		if err != nil {
			return fmt.Errorf("recording unverified users: %w", err)
		}
	}

	return nil
}

// scanWeakPasswordSettings checks for weak password configuration.
func (s *Service) scanWeakPasswordSettings(ctx context.Context, tenantID pgtype.UUID) error {
	var minLength int

	err := s.db.Reader.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT value::int FROM system_settings WHERE tenant_id = $1 AND key = 'auth.password.min_length'),
			8
		)
	`, tenantID).Scan(&minLength)
	if err != nil {
		return nil //nolint:nilerr
	}

	if minLength < 8 {
		meta, _ := json.Marshal(map[string]any{
			"min_length":  minLength,
			"recommended": 8,
		})

		_, err = s.store.CreateSecurityScanResult(ctx, gen.CreateSecurityScanResultParams{
			TenantID: tenantID,
			ScanType: "weak_password_settings",
			Severity: string(SeverityWarning),
			Finding:  fmt.Sprintf("Weak password policy: minimum length is %d (recommended: 8+)", minLength),
			Meta:     meta,
		})
		if err != nil {
			return fmt.Errorf("recording weak password settings: %w", err)
		}
	}

	return nil
}

// scanExcessiveAPIKeys checks for API keys without expiration.
func (s *Service) scanExcessiveAPIKeys(ctx context.Context, tenantID pgtype.UUID) error {
	var count int64

	err := s.db.Reader.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM api_keys
		WHERE tenant_id = $1
		  AND status = 'active'
		  AND expires_at IS NULL
	`, tenantID).Scan(&count)
	if err != nil {
		return nil //nolint:nilerr
	}

	if count > 20 {
		meta, _ := json.Marshal(map[string]any{
			"key_count":      count,
			"issue":          "no_expiration",
			"recommendation": "Set expiration dates on API keys",
		})

		_, err = s.store.CreateSecurityScanResult(ctx, gen.CreateSecurityScanResultParams{
			TenantID: tenantID,
			ScanType: "excessive_api_keys",
			Severity: string(SeverityInfo),
			Finding:  fmt.Sprintf("API keys without expiration: %d active keys have no expiration date", count),
			Meta:     meta,
		})
		if err != nil {
			return fmt.Errorf("recording excessive API keys: %w", err)
		}
	}

	return nil
}
