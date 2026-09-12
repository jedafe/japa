// Package advancedanalytics implements the Advanced Analytics Dashboard
// (Section 66, ADR-084).
//
// Ref: japaconnect-engineering/sections/66-advanced-analytics.md
//
//	japaconnect-engineering/adr/ADR-084-advanced-analytics.md
package advancedanalytics

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Period constants.
const (
	Period7d  = "7d"
	Period30d = "30d"
	Period90d = "90d"
)

// Service implements the advanced analytics business logic.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates an advanced analytics service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// GetTopContent returns top content by engagement score.
func (s *Service) GetTopContent(ctx context.Context, tenantID pgtype.UUID, period, contentType string, limit int) ([]gen.AdvancedAnalyticsTopContentRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	days := periodToDays(period)

	return s.store.TopContent(ctx, gen.AdvancedAnalyticsTopContentParams{

		Days:        int32(days), // #nosec
		ContentType: contentType,
		TopN:        int32(limit), // #nosec
	})
}

// periodToDays converts a period string ("7d", "30d", "90d") to a day count.
// Defaults to 7 days for unrecognized values.
func periodToDays(period string) int {
	switch period {
	case Period30d:
		return 30
	case Period90d:
		return 90
	default:
		return 7
	}
}

// GetRevenueStreams returns monthly revenue breakdown by stream.
func (s *Service) GetRevenueStreams(ctx context.Context, tenantID pgtype.UUID, period string) ([]gen.MvRevenueStream, error) {
	// Default: last 12 months
	fromMonth := pgtype.Date{Time: time.Now().AddDate(-1, 0, 0), Valid: true}
	if period == "quarterly" {
		fromMonth = pgtype.Date{Time: time.Now().AddDate(-2, 0, 0), Valid: true}
	}

	return s.store.RevenueStreams(ctx, gen.AdvancedAnalyticsRevenueStreamsParams{

		FromMonth: fromMonth,
	})
}

// GetAdPerformance returns ad campaign aggregate performance.
func (s *Service) GetAdPerformance(ctx context.Context, tenantID pgtype.UUID, status string, limit int) ([]gen.AdvancedAnalyticsAdPerformanceRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	return s.store.AdPerformance(ctx, gen.AdvancedAnalyticsAdPerformanceParams{

		Status: status,
		TopN:   int32(limit), // #nosec
	})
}

// GetNewsletterEngagement returns newsletter open/click rates.
func (s *Service) GetNewsletterEngagement(ctx context.Context, tenantID pgtype.UUID, limit int) ([]gen.AdvancedAnalyticsNewsletterEngagementRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	return s.store.NewsletterEngagement(ctx, gen.AdvancedAnalyticsNewsletterEngagementParams{

		TopN: int32(limit), // #nosec
	})
}

// GetSubscriberGrowth returns newsletter subscriber counts per list.
func (s *Service) GetSubscriberGrowth(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdvancedAnalyticsSubscriberGrowthRow, error) {
	return s.store.SubscriberGrowth(ctx, tenantID)
}

// GetRetentionCohorts returns D1/D7/D30 retention cohorts.
func (s *Service) GetRetentionCohorts(ctx context.Context, tenantID pgtype.UUID, limit int) ([]gen.AdvancedAnalyticsRetentionCohortsRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	fromDate := pgtype.Timestamptz{Time: time.Now().AddDate(0, -3, 0), Valid: true}

	return s.store.RetentionCohorts(ctx, gen.AdvancedAnalyticsRetentionCohortsParams{

		FromDate: fromDate,
		TopN:     int32(limit), // #nosec
	})
}

// ExportCSV exports analytics data as CSV.
func (s *Service) ExportCSV(ctx context.Context, tenantID pgtype.UUID, metric, period string) (string, []string, error) {
	enabled, err := s.settings.GetSettingBool(ctx, tenantID, "analytics.export_enabled")
	if err != nil || !enabled {
		return "", nil, errors.New("advancedanalytics: CSV export is disabled")
	}

	switch metric {
	case "top_content":
		rows, err := s.GetTopContent(ctx, tenantID, period, "", 100)
		if err != nil {
			return "", nil, err
		}

		headers := []string{"content_id", "title", "slug", "content_type", "total_views", "total_reactions", "total_comments", "engagement_score"}

		return writeCSV(headers, func(w *csv.Writer) {
			for _, r := range rows {
				title := ""
				if r.Title != nil {
					title = *r.Title
				}

				slug := ""
				if r.Slug != nil {
					slug = *r.Slug
				}

				contentType := ""
				if r.ContentType != nil {
					contentType = *r.ContentType
				}

				w.Write([]string{
					r.ContentID.String(), title, slug, contentType,
					strconv.FormatInt(r.TotalViews, 10),
					strconv.FormatInt(r.TotalReactions, 10),
					strconv.FormatInt(r.TotalComments, 10),
					strconv.FormatInt(int64(r.EngagementScore), 10),
				})
			}
		}), headers, nil

	case "revenue_streams":
		rows, err := s.GetRevenueStreams(ctx, tenantID, period)
		if err != nil {
			return "", nil, err
		}

		headers := []string{"month", "subscription_revenue", "course_revenue", "ad_revenue", "credit_revenue"}

		return writeCSV(headers, func(w *csv.Writer) {
			for _, r := range rows {
				monthStr := ""
				if r.Month.Valid {
					monthStr = r.Month.Time.Format("2006-01-02")
				}

				w.Write([]string{
					monthStr,
					strconv.FormatInt(r.SubscriptionRevenue, 10),
					strconv.FormatInt(r.CourseRevenue, 10),
					strconv.FormatInt(r.AdRevenue, 10),
					strconv.FormatInt(r.CreditRevenue, 10),
				})
			}
		}), headers, nil

	case "ad_performance":
		rows, err := s.GetAdPerformance(ctx, tenantID, "", 100)
		if err != nil {
			return "", nil, err
		}

		headers := []string{"campaign_id", "campaign_name", "status", "impressions", "clicks", "ctr", "spent"}

		return writeCSV(headers, func(w *csv.Writer) {
			for _, r := range rows {
				w.Write([]string{
					r.CampaignID.String(), r.CampaignName, r.Status,
					strconv.FormatInt(r.Impressions, 10),
					strconv.FormatInt(r.Clicks, 10),
					strconv.FormatInt(int64(r.Ctr), 10),
					strconv.FormatInt(r.Spent, 10),
				})
			}
		}), headers, nil

	case "newsletter":
		campaigns, err := s.GetNewsletterEngagement(ctx, tenantID, 100)
		if err != nil {
			return "", nil, err
		}

		headers := []string{"campaign_id", "campaign_name", "subject", "sent_at", "total_sent", "total_opened", "total_clicked", "open_rate", "click_rate"}

		return writeCSV(headers, func(w *csv.Writer) {
			for _, r := range campaigns {
				sentAt := ""
				if r.SentAt.Valid {
					sentAt = r.SentAt.Time.Format(time.RFC3339)
				}

				w.Write([]string{
					r.CampaignID.String(), r.CampaignName, r.Subject, sentAt,
					strconv.FormatInt(r.TotalSent, 10),
					strconv.FormatInt(r.TotalOpened, 10),
					strconv.FormatInt(r.TotalClicked, 10),
					strconv.FormatInt(int64(r.OpenRate), 10),
					strconv.FormatInt(int64(r.ClickRate), 10),
				})
			}
		}), headers, nil

	case "retention":
		cohorts, err := s.GetRetentionCohorts(ctx, tenantID, 100)
		if err != nil {
			return "", nil, err
		}

		headers := []string{"cohort_date", "cohort_size", "retained_d1", "retained_d7", "retained_d30", "d1_rate", "d7_rate", "d30_rate"}

		return writeCSV(headers, func(w *csv.Writer) {
			for _, r := range cohorts {
				cohortDate := ""
				if r.CohortDate.Valid {
					cohortDate = r.CohortDate.Time.Format("2006-01-02")
				}

				w.Write([]string{
					cohortDate,
					strconv.FormatInt(r.CohortSize, 10),
					strconv.FormatInt(r.RetainedD1, 10),
					strconv.FormatInt(r.RetainedD7, 10),
					strconv.FormatInt(r.RetainedD30, 10),
					strconv.FormatInt(int64(r.D1Rate), 10),
					strconv.FormatInt(int64(r.D7Rate), 10),
					strconv.FormatInt(int64(r.D30Rate), 10),
				})
			}
		}), headers, nil

	default:
		return "", nil, fmt.Errorf("advancedanalytics: unknown export metric: %s", metric)
	}
}

// writeCSV builds a CSV string with proper escaping using encoding/csv.
func writeCSV(headers []string, writeRows func(w *csv.Writer)) string {
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	_ = w.Write(headers)
	writeRows(w)
	w.Flush()

	return sb.String()
}

// --- HTTP Handlers ---

// Handlers exposes the advanced analytics HTTP routes.
type Handlers struct {
	svc *Service
}

// NewHandlers creates advanced analytics handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// HandleTopContent returns top content by engagement.
// GET /admin/analytics/top-content.
func (h *Handlers) HandleTopContent(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = Period7d
	}

	contentType := r.URL.Query().Get("type")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, err := h.svc.GetTopContent(r.Context(), user.TenantID, period, contentType, limit)
	if err != nil {
		slog.Error("advancedanalytics: top content query failed", "error", err, "tenant_id", user.TenantID)
		errpkg.WriteError(w, http.StatusInternalServerError, "query_failed", "Failed to retrieve top content")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"content": rows})
}

// HandleRevenueStreams returns revenue breakdown.
// GET /admin/analytics/revenue-streams.
func (h *Handlers) HandleRevenueStreams(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "monthly"
	}

	rows, err := h.svc.GetRevenueStreams(r.Context(), user.TenantID, period)
	if err != nil {
		slog.Error("advancedanalytics: revenue streams query failed", "error", err, "tenant_id", user.TenantID)
		errpkg.WriteError(w, http.StatusInternalServerError, "query_failed", "Failed to retrieve revenue streams")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"revenue": rows})
}

// HandleAdPerformance returns ad campaign aggregate stats.
// GET /admin/analytics/ads.
func (h *Handlers) HandleAdPerformance(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, err := h.svc.GetAdPerformance(r.Context(), user.TenantID, status, limit)
	if err != nil {
		slog.Error("advancedanalytics: ad performance query failed", "error", err, "tenant_id", user.TenantID)
		errpkg.WriteError(w, http.StatusInternalServerError, "query_failed", "Failed to retrieve ad performance")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"ads": rows})
}

// HandleNewsletterEngagement returns newsletter open/click rates.
// GET /admin/analytics/newsletter.
func (h *Handlers) HandleNewsletterEngagement(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	campaigns, err := h.svc.GetNewsletterEngagement(r.Context(), user.TenantID, limit)
	if err != nil {
		slog.Error("advancedanalytics: newsletter engagement query failed", "error", err, "tenant_id", user.TenantID)
		errpkg.WriteError(w, http.StatusInternalServerError, "query_failed", "Failed to retrieve newsletter engagement")

		return
	}

	lists, err := h.svc.GetSubscriberGrowth(r.Context(), user.TenantID)
	if err != nil {
		slog.Error("advancedanalytics: subscriber growth query failed", "error", err, "tenant_id", user.TenantID)
		errpkg.WriteError(w, http.StatusInternalServerError, "query_failed", "Failed to retrieve subscriber growth")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"campaigns": campaigns,
		"lists":     lists,
	})
}

// HandleRetention returns retention cohorts.
// GET /admin/analytics/retention.
func (h *Handlers) HandleRetention(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, err := h.svc.GetRetentionCohorts(r.Context(), user.TenantID, limit)
	if err != nil {
		slog.Error("advancedanalytics: retention cohorts query failed", "error", err, "tenant_id", user.TenantID)
		errpkg.WriteError(w, http.StatusInternalServerError, "query_failed", "Failed to retrieve retention cohorts")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"cohorts": rows})
}

// HandleExport exports analytics as CSV.
// GET /admin/analytics/export.
func (h *Handlers) HandleExport(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}

	metric := r.URL.Query().Get("metric")
	period := r.URL.Query().Get("period")

	if metric == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "metric is required")
		return
	}

	csvData, _, err := h.svc.ExportCSV(r.Context(), user.TenantID, metric, period)
	if err != nil {
		slog.Error("advancedanalytics: CSV export failed", "error", err, "metric", metric, "tenant_id", user.TenantID)
		errpkg.WriteError(w, http.StatusBadRequest, "export_failed", "Export unavailable")

		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+metric+"_export.csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(csvData))
}

// --- shared helpers ---

// requireAuthUser extracts the authenticated user from the request context
// using the auth middleware's context key. Returns the AuthenticatedUser and
// writes a 401 error + false if no authenticated user is present.
func requireAuthUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid || !u.TenantID.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}
