package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Default security scan thresholds (used when system_settings is not
// configured or the settings reader is nil). These match the values seeded
// by migration 000501_security_scan_settings.up.sql.
const (
	defaultFailedLoginThreshold   = 50
	defaultSLABreachThreshold     = 5
	defaultStaleTokensThreshold   = 100
	defaultUnverifiedUserThreshold = 10
	defaultMinPasswordLength      = 8
	defaultExcessiveAPIKeysThreshold = 20
)

// getSettingInt reads an int system_setting for the tenant, returning the
// fallback default when the settings reader is nil or the key is absent.
// This satisfies Rule 3 (no hardcoded config) while remaining testable.
func (s *Service) getSettingInt(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int {
	if s.settings == nil {
		return fallback
	}
	val, err := s.settings.GetSettingInt(ctx, tenantID, key)
	if err != nil || val <= 0 {
		return fallback
	}
	return val
}

// scanFailedLoginSpike checks if failed logins in the last hour exceed threshold.
// BUG-ANALYTICS-06 fix: query audit_logs (action_type = 'auth.login_failed')
// instead of platform_events, which never contained that event type.
// BUG-003 fix: threshold read from system_settings 'security.scan.failed_login_threshold'.
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

	threshold := int64(s.getSettingInt(ctx, tenantID, "security.scan.failed_login_threshold", defaultFailedLoginThreshold))
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
// BUG-003 fix: threshold read from system_settings 'security.scan.sla_breach_threshold'.
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

	threshold := int64(s.getSettingInt(ctx, tenantID, "security.scan.sla_breach_threshold", defaultSLABreachThreshold))
	if count > threshold {
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
// BUG-003 fix: threshold read from system_settings 'security.scan.stale_tokens_threshold'.
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

	threshold := int64(s.getSettingInt(ctx, tenantID, "security.scan.stale_tokens_threshold", defaultStaleTokensThreshold))
	if count > threshold {
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
// BUG-003 fix: threshold read from system_settings 'security.scan.unverified_users_threshold'.
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

	threshold := int64(s.getSettingInt(ctx, tenantID, "security.scan.unverified_users_threshold", defaultUnverifiedUserThreshold))
	if count > threshold {
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
// BUG-003 fix: recommended min length read from system_settings
// 'security.scan.min_password_length' (defaults to 8).
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

	recommended := s.getSettingInt(ctx, tenantID, "security.scan.min_password_length", defaultMinPasswordLength)
	if minLength < recommended {
		meta, _ := json.Marshal(map[string]any{
			"min_length":  minLength,
			"recommended": recommended,
		})

		_, err = s.store.CreateSecurityScanResult(ctx, gen.CreateSecurityScanResultParams{
			TenantID: tenantID,
			ScanType: "weak_password_settings",
			Severity: string(SeverityWarning),
			Finding:  fmt.Sprintf("Weak password policy: minimum length is %d (recommended: %d+)", minLength, recommended),
			Meta:     meta,
		})
		if err != nil {
			return fmt.Errorf("recording weak password settings: %w", err)
		}
	}

	return nil
}

// scanExcessiveAPIKeys checks for API keys without expiration.
// BUG-003 fix: threshold read from system_settings 'security.scan.excessive_api_keys_threshold'.
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

	threshold := int64(s.getSettingInt(ctx, tenantID, "security.scan.excessive_api_keys_threshold", defaultExcessiveAPIKeysThreshold))
	if count > threshold {
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
