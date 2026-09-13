package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"regexp"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// Wallet ledger constants — inlined from internal/wallet to avoid an import
// cycle (internal/wallet/tasks.go imports internal/worker). These mirror
// wallet.LedgerTypeCredits and wallet.OperationEarn; keep in sync.
const (
	affiliateLedgerTypeCredits = "credits"
	affiliateOperationEarn     = "earn"
)

// ─── Affiliate: Credit Author Commission ────────────────────────────

// HandleAffiliateCreditAuthorCommission credits an author's revenue share from
// an external affiliate commission to the author's wallet via the immutable
// credits ledger (Section 28). Idempotent on the commission row ID.
// // AUDIT-BE-FIN-001: the previous implementation mutated a non-existent
// `users.credits_balance` column with a direct UPDATE, violating the
// immutable-ledger requirement. It now appends a wallet_ledger entry inside a
// transaction holding pg_advisory_xact_lock(user_id), matching the
// SpendCredits/ConvertPointsToCredits pattern.
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

	var commissionID pgtype.UUID
	if err := commissionID.Scan(payload.CommissionID); err != nil {
		return fmt.Errorf("affiliate: invalid commission_id %q: %w", payload.CommissionID, err)
	}

	readQ := gen.New(dbPool.Reader)

	row, err := readQ.GetAffiliateContentCommissionForCredit(ctx, commissionID)
	if err != nil {
		slog.Warn("affiliate: commission not found", "id", payload.CommissionID, "error", err)
		return nil
	}

	if row.Credited {
		return nil
	}

	// author_share is numeric(12,2) in major currency units; the wallet ledger
	// amount is BIGINT in kobo (minor units). Convert with rounding.
	shareFloat, err := row.AuthorShare.Float64Value()
	if err != nil || !shareFloat.Valid {
		slog.Warn("affiliate: invalid author_share", "commission_id", payload.CommissionID, "error", err)
		return nil
	}

	amountKobo := int64(math.Round(shareFloat.Float64 * 100))
	if amountKobo <= 0 {
		slog.Warn("affiliate: non-positive author share, skipping credit", "commission_id", payload.CommissionID, "amount", amountKobo)
		return nil
	}

	if err := creditAuthorCommissionTx(ctx, row, amountKobo); err != nil {
		return err
	}

	slog.Info("affiliate: credited author commission", "commission_id", payload.CommissionID, "author_user_id", row.AuthorUserID.String(), "amount_kobo", amountKobo)

	return nil
}

// creditAuthorCommissionTx credits the author's wallet via the immutable
// ledger inside a tx holding pg_advisory_xact_lock(tenant:user) to serialize
// concurrent balance mutations, then marks the commission credited in the
// same tx (idempotent guard: only updates rows still flagged credited=false).
func creditAuthorCommissionTx(ctx context.Context, row gen.GetAffiliateContentCommissionForCreditRow, amountKobo int64) error {
	tx, err := dbPool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("affiliate: beginning tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := gen.New(tx)

	lockKey := "wallet:" + row.AuthorUserID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("affiliate: acquiring wallet lock: %w", err)
	}

	currentCredits, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{

		UserID:     row.AuthorUserID,
		LedgerType: affiliateLedgerTypeCredits,
	})
	if err != nil {
		currentCredits = 0
	}

	newBalance := currentCredits + amountKobo

	idempotencyKey := "affiliate_commission:" + row.ID.String()
	source := "affiliate_commission"
	reference := row.ID.String()

	if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{

		UserID:           row.AuthorUserID,
		LedgerType:       affiliateLedgerTypeCredits,
		Operation:        affiliateOperationEarn,
		Amount:           amountKobo,
		ResultingBalance: newBalance,
		Source:           &source,
		Reference:        &reference,
		IdempotencyKey:   &idempotencyKey,
		Reason:           nil,
	}); err != nil {
		return fmt.Errorf("affiliate: inserting ledger entry: %w", err)
	}

	// Mark the commission credited within the same tx (idempotent guard:
	// only updates rows still flagged credited=false).
	if _, err = qtx.MarkAffiliateCommissionCredited(ctx, row.ID); err != nil {
		return fmt.Errorf("affiliate: marking commission credited: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("affiliate: committing tx: %w", err)
	}

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

// HandleContentScanAffiliateLinks scans a published content body for links to
// domains that are not registered affiliate providers, flagging potential
// unregistered affiliate links per the affiliate.unregistered_link_policy
// system setting.
func HandleContentScanAffiliateLinks(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string 
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
	// AUDIT-BE-TASK-001: respect the affiliate.scan_on_publish system setting.
	// If disabled, exit early without scanning.
	if val, err := getAffSystemSetting(ctx, tenantID, "affiliate.scan_on_publish"); err == nil && (val == "false" || val == "0") {
		return nil
	}

	policy := "flag"
	if val, err := getAffSystemSetting(ctx, tenantID, "affiliate.unregistered_link_policy"); err == nil {
		policy = val
	}

	var body string

	err := dbPool.Reader.QueryRow(ctx, `SELECT body FROM contents WHERE id = $1`, contentID).Scan(&body)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}

		return fmt.Errorf("task:affiliate:scan_content: load content body: %w", err)
	}

	urls := extractURLsFromTipTap(body)
	if len(urls) == 0 {
		return nil
	}

	registered := getRegisteredDomains(ctx, tenantID)

	for _, rawURL := range urls {
		scanURLForAffiliate(ctx, tenantID, contentID, rawURL, policy, registered)
	}

	return nil
}

// scanURLForAffiliate checks one URL against the affiliate detection rules and
// records a content_affiliate_scan_results row when it matches. Registered
// providers and non-affiliate URLs are skipped.
func scanURLForAffiliate(ctx context.Context, tenantID, contentID pgtype.UUID, rawURL, policy string, registered map[string]bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return
	}

	domain := parsed.Hostname()
	if domain == "" || isRegProvider(domain, registered) {
		return
	}

	method := ""
	if hasAffParams(parsed) {
		method = "param_match"
	} else if isKnownAffDomain(domain) {
		method = "domain_match"
	}

	if method == "" {
		return
	}

	action := policy
	if action == "strip" {
		action = "flagged"
	}

	if _, err := dbPool.Writer.Exec(ctx, `INSERT INTO content_affiliate_scan_results
		 (content_id, url, domain, detection_method, action_taken)
		 VALUES ($1, $2, $3, $4, $5)`, contentID, rawURL, domain, method, action); err != nil {
		slog.Warn("affiliate scan: insert scan result failed", "content_id", contentID, "url", rawURL, "domain", domain, "error", err)
	}

	slog.Info("affiliate scan: detected unregistered link", "content_id", contentID, "url", rawURL, "domain", domain)
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

	rows, err := dbPool.Reader.Query(ctx, `SELECT base_url FROM affiliate_providers WHERE is_active = true AND base_url IS NOT NULL`)
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
	err := dbPool.Reader.QueryRow(ctx, `SELECT value::text FROM system_settings WHERE key = $1`, key).Scan(&value)

	return strings.Trim(value, "\""), err
}
