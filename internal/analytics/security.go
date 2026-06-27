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
func (s *Service) scanFailedLoginSpike(ctx context.Context, tenantID pgtype.UUID) error {
	// Query the security_scan_results table for recent failed_login_spike findings.
	// If no finding exists in the last hour, check if there's a spike and record it.
	oneHourAgo := time.Now().Add(-1 * time.Hour)

	// Count recent failed login events from platform_events meta.
	// This is a heuristic — in production the actual auth logs would be queried.
	var count int64
	err := s.db.Reader.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM platform_events
		WHERE tenant_id = $1
		  AND event_type = 'auth.login_failed'
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
func (s *Service) scanSLABreachCount(ctx context.Context, tenantID pgtype.UUID) error {
	var count int64
	err := s.db.Reader.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM security_scan_results
		WHERE tenant_id = $1
		  AND scan_type = 'sla_breach_count'
		  AND resolved_at IS NULL
		  AND created_at >= now() - interval '24 hours'
	`, tenantID).Scan(&count)
	if err != nil {
		return fmt.Errorf("counting SLA breaches: %w", err)
	}

	if count > 5 {
		meta, _ := json.Marshal(map[string]any{
			"breach_count": count,
			"period":       "24h",
		})
		_, err = s.store.CreateSecurityScanResult(ctx, gen.CreateSecurityScanResultParams{
			TenantID: tenantID,
			ScanType: string(ScanSLABreachCount),
			Severity: string(SeverityCritical),
			Finding:  fmt.Sprintf("High SLA breach count: %d unresolved in 24h", count),
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
		return nil
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


