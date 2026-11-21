package templates

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"strings"
	texttemplate "text/template"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/brand"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service is an exported type.
type Service struct {
	store *Store
	db    *db.DB
}

// NewService is an exported function.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)},
		db:    database,
	}
}

// ListTemplatesResponse is an exported type.
type ListTemplatesResponse struct {
	Items      []gen.ListEmailTemplatesRow `json:"items"`
	Page       int32                       `json:"page"`
	Total      int64                       `json:"total"`
	TotalPages int32                       `json:"total_pages"`
}

// ListTemplates is an exported method.
func (s *Service) ListTemplates(ctx context.Context, filterType, filterIsActive string, limit, offset int32) (*ListTemplatesResponse, error) {
	templates, err := s.store.listTemplates(ctx, filterType, filterIsActive, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}

	total, err := s.store.countTemplates(ctx, filterType, filterIsActive)
	if err != nil {
		return nil, fmt.Errorf("counting templates: %w", err)
	}

	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}

	return &ListTemplatesResponse{Items: templates, Page: offset / limit, Total: int64(total), TotalPages: totalPages}, nil
}

// GetTemplate is an exported method.
func (s *Service) GetTemplate(ctx context.Context, id pgtype.UUID) (gen.GetEmailTemplateRow, error) {
	return s.store.getTemplate(ctx, id)
}

// UpdateTemplate is an exported method.
func (s *Service) UpdateTemplate(ctx context.Context, id pgtype.UUID, subject, htmlBody, plainTextBody string, isActive bool) (gen.UpdateEmailTemplateRow, error) {
	// ADR-055: validate Go template syntax at save time. An unparseable
	// template would otherwise persist and silently degrade to the
	// fallback body at send time.
	if _, err := texttemplate.New("subject").Parse(subject); err != nil {
		return gen.UpdateEmailTemplateRow{}, fmt.Errorf("%w: invalid subject template: %v", ErrInvalidTemplate, err)
	}

	if htmlBody != "" {
		if _, err := template.New("html_body").Parse(htmlBody); err != nil {
			return gen.UpdateEmailTemplateRow{}, fmt.Errorf("%w: invalid html_body template: %v", ErrInvalidTemplate, err)
		}
	}

	return s.store.updateTemplate(ctx, gen.UpdateEmailTemplateParams{
		ID: id,

		Subject:       subject,
		HtmlBody:      htmlBody,
		PlainTextBody: plainTextBody,
		IsActive:      isActive,
	})
}

// ResetTemplate is an exported method.
func (s *Service) ResetTemplate(ctx context.Context, id pgtype.UUID) error {
	// The reset query is a bare UPDATE — verify the row exists first so a
	// nonexistent id returns 404 instead of a silent 200 no-op.
	if _, err := s.store.getTemplate(ctx, id); err != nil {
		return err
	}

	return s.store.resetTemplate(ctx, id)
}

// SampleTemplateData builds the template_data payload for a test send from
// the template's declared variables — every declared var gets a realistic
// sample so the delivered email renders instead of printing <no value>
// placeholders. site_name and logo_url are left out: the worker resolves
// both from platform settings on every templated send.
func (s *Service) SampleTemplateData(ctx context.Context, variables []byte) map[string]any {
	var declared []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(variables, &declared)

	base := s.testSiteBase(ctx)

	data := make(map[string]any, len(declared))
	for _, v := range declared {
		key := pascalToSnake(v.Name)
		if key == "" || key == "site_name" || key == "logo_url" {
			continue
		}
		data[key] = sampleTemplateVar(v.Name, base)
	}

	return data
}

// TestSiteName resolves the display site name for a test-send payload —
// general.site_name with the brand fallback, mirroring the digest service.
func (s *Service) TestSiteName(ctx context.Context) string {
	if s.db != nil {
		if row, err := gen.New(s.db.Reader).GetSettingValue(ctx, "general.site_name"); err == nil && len(row) > 0 {
			var name string
			if json.Unmarshal(row, &name) == nil && name != "" {
				return name
			}
		}
	}

	return brand.Get()
}

// testSiteBase resolves the canonical site URL for sample link values:
// site.base_url from system_settings, then JC_SITE_URL — the same order as
// the worker's resolveBaseURL.
func (s *Service) testSiteBase(ctx context.Context) string {
	if s.db != nil {
		if row, err := gen.New(s.db.Reader).GetSettingValue(ctx, "site.base_url"); err == nil && len(row) > 0 {
			var base string
			if json.Unmarshal(row, &base) == nil && base != "" {
				return strings.TrimRight(base, "/")
			}
		}
	}

	if base := strings.TrimRight(os.Getenv("JC_SITE_URL"), "/"); base != "" {
		return base
	}

	// Sample-only fallback for unconfigured dev environments — a reserved
	// example domain keeps generated sample links valid-looking.
	return "https://example.com"
}

// pascalToSnake converts a template variable name (ActionURL, TicketID) to
// the snake_case template_data key the worker maps back via snakeToPascal.
func pascalToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 &&
			(unicode.IsLower(rune(s[i-1])) || (i+1 < len(s) && unicode.IsLower(rune(s[i+1])))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}

	return b.String()
}

// sampleTemplateVar returns a realistic sample value for a declared template
// variable, keyed by the variable's PascalCase name. Names not in the table
// fall back to a generic "Sample <name>" so nothing renders as <no value>.
func sampleTemplateVar(name, siteBase string) string {
	if strings.HasSuffix(name, "URL") {
		return siteBase + "/preview"
	}

	samples := map[string]string{
		"SiteURL":           siteBase,
		"SupportURL":        siteBase + "/support",
		"UserName":          "Test User",
		"AppellantUsername": "jane_doe",
		"ReporterUsername":  "john_doe",
		"NewUsername":       "new_handle",
		"OrgName":           "Sample Organization",
		"CourseName":        "Sample Course",
		"PlanName":          "Pro",
		"Role":              "Moderator",
		"Status":            "active",
		"OldStatus":         "pending",
		"NewStatus":         "approved",
		"ActionType":        "review",
		"Location":          "Lagos, Nigeria",
		"DeviceInfo":        "Chrome on macOS",
		"NewEmail":          "new@example.com",
		"Reason":            "Policy violation",
		"FailureReason":     "Card declined",
		"Note":              "Sample note",
		"Expiry":            "1 hour",
		"ExpiryDate":        "2025-12-31",
		"NextBillingDate":   "2025-02-01",
		"AccessUntil":       "2025-12-31",
		"Timestamp":         "2025-01-15 10:30 UTC",
		"Amount":            "25.00",
		"Currency":          "USD",
		"PaymentMethod":     "Card ending 4242",
		"TransactionID":     "txn_sample_123",
		"TicketID":          "TKT-12345",
		"Subject":           "Sample subject line",
		"Title":             "New reply to your thread",
		"Body":              "Someone replied to a thread you follow.",
		"Message":           "This is a sample message.",
		"Description":       "Sample description text.",
		"Content":           "<p>Sample content preview.</p>",
		"DigestContent":     "<p>Sample digest items preview.</p>",
	}

	if v, ok := samples[name]; ok {
		return v
	}

	return "Sample " + name
}
