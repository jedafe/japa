package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// ─── Affiliate: Credit Author Commission ────────────────────────────

func HandleAffiliateCreditAuthorCommission(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		CommissionID string `json:"commission_id"`
	}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}
	if dbPool == nil {
		return nil
	}
	var commissionID, authorUserID pgtype.UUID
	var authorShare float64
	var credited bool
	if err := dbPool.Reader.QueryRow(ctx,
		`SELECT id, author_user_id, author_share, credited
		 FROM affiliate_content_commissions WHERE id = $1`, payload.CommissionID,
	).Scan(&commissionID, &authorUserID, &authorShare, &credited); err != nil {
		slog.Warn("affiliate: commission not found", "id", payload.CommissionID, "error", err)
		return nil
	}
	if credited {
		return nil
	}
	if _, err := dbPool.Writer.Exec(ctx,
		`UPDATE users SET credits_balance = credits_balance + $1 WHERE id = $2`,
		authorShare, authorUserID); err != nil {
		slog.Error("affiliate: failed to credit author wallet", "error", err)
		return err
	}
	if _, err := dbPool.Writer.Exec(ctx,
		`UPDATE affiliate_content_commissions SET credited = true, credited_at = now() WHERE id = $1`,
		commissionID); err != nil {
		slog.Error("affiliate: failed to mark commission credited", "error", err)
		return err
	}
	slog.Info("affiliate: credited author commission",
		"commission_id", payload.CommissionID,
		"author_user_id", authorUserID.String(), "amount", authorShare)
	return nil
}

// ─── Content: Scan Affiliate Links ──────────────────────────────────

var affScanParams = []string{
	"ref", "referral", "affiliate", "aff_id", "aff", "aid", "tag",
	"utm_source", "utm_medium", "partner", "promo", "click_id", "sub_id",
}
var affScanDomains = []string{
	"amazon.com", "booking.com", "remitly.com", "wise.com", "flutterwave.com",
	"paystack.com", "udemy.com", "skillshare.com", "hostinger.com",
	"aliexpress.com", "shareasale.com", "cj.com", "impact.com",
	"rakuten.com", "awin.com", "clickbank.com",
}

func HandleContentScanAffiliateLinks(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
	}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}
	if dbPool == nil {
		return nil
	}
	var contentID, tenantID pgtype.UUID
	if err := contentID.Scan(payload.ContentID); err != nil {
		return err
	}
	if err := tenantID.Scan(payload.TenantID); err != nil {
		return err
	}
	policy := "flag"
	if val, err := getAffSystemSetting(ctx, tenantID, "affiliate.unregistered_link_policy"); err == nil {
		policy = val
	}
	var body string
	if err := dbPool.Reader.QueryRow(ctx,
		`SELECT body FROM contents WHERE id = $1 AND tenant_id = $2`,
		contentID, tenantID).Scan(&body); err != nil {
		return nil
	}
	urls := extractURLsFromTipTap(body)
	if len(urls) == 0 {
		return nil
	}
	registered := getRegisteredDomains(ctx, tenantID)
	for _, rawURL := range urls {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		domain := parsed.Hostname()
		if domain == "" || isRegProvider(domain, registered) {
			continue
		}
		method := ""
		if hasAffParams(parsed) {
			method = "param_match"
		} else if isKnownAffDomain(domain) {
			method = "domain_match"
		}
		if method == "" {
			continue
		}
		action := policy
		if action == "strip" {
			action = "flagged"
		}
		dbPool.Writer.Exec(ctx,
			`INSERT INTO content_affiliate_scan_results
			 (tenant_id, content_id, url, domain, detection_method, action_taken)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenantID, contentID, rawURL, domain, method, action)
		slog.Info("affiliate scan: detected unregistered link",
			"content_id", payload.ContentID, "url", rawURL, "domain", domain)
	}
	return nil
}

// ─── Helpers ────────────────────────────────────────────────────────

func extractURLsFromTipTap(body string) []string {
	re := regexp.MustCompile(`https?://[^\s"'\\}]+`)
	matches := re.FindAllString(body, -1)
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		m = strings.TrimRight(m, ".,;:!?)")
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}

func getRegisteredDomains(ctx context.Context, tenantID pgtype.UUID) map[string]bool {
	domains := make(map[string]bool)
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT base_url FROM affiliate_providers WHERE tenant_id = $1 AND is_active = true AND base_url IS NOT NULL`,
		tenantID)
	if err != nil {
		return domains
	}
	defer rows.Close()
	for rows.Next() {
		var baseURL string
		if rows.Scan(&baseURL) != nil {
			continue
		}
		if parsed, err := url.Parse(baseURL); err == nil {
			if d := parsed.Hostname(); d != "" {
				domains[d] = true
			}
		}
	}
	return domains
}

func isRegProvider(domain string, registered map[string]bool) bool {
	if registered[domain] {
		return true
	}
	parts := strings.Split(domain, ".")
	if len(parts) > 2 {
		return registered[strings.Join(parts[len(parts)-2:], ".")]
	}
	return false
}

func hasAffParams(u *url.URL) bool {
	for _, p := range affScanParams {
		if u.Query().Has(p) {
			return true
		}
	}
	return false
}

func isKnownAffDomain(domain string) bool {
	for _, ad := range affScanDomains {
		if strings.Contains(domain, ad) {
			return true
		}
	}
	return false
}

func getAffSystemSetting(ctx context.Context, tenantID pgtype.UUID, key string) (string, error) {
	var value string
	err := dbPool.Reader.QueryRow(ctx,
		`SELECT value::text FROM system_settings WHERE tenant_id = $1 AND key = $2`,
		tenantID, key).Scan(&value)
	return strings.Trim(value, "\""), err
}
