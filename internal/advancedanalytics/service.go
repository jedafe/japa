// Package advancedanalytics implements the Advanced Analytics Dashboard
// (Section 66, ADR-084).
//
// Ref: japaconnect-engineering/sections/66-advanced-analytics.md
//      japaconnect-engineering/adr/ADR-084-advanced-analytics.md
package advancedanalytics

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
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
	// The mv_top_content_7d view is always 7d. For 30d/90d, we'd need
	// additional views — for now, the 7d view is the only one materialized.
	// The period parameter is accepted for forward compatibility.
	return s.store.TopContent(ctx, gen.AdvancedAnalyticsTopContentParams{
		TenantID:     tenantID,
		ContentType:  contentType,
		TopN:         int32(limit),
	})
}

// GetRevenueStreams returns monthly revenue breakdown by stream.
func (s *Service) GetRevenueStreams(ctx context.Context, tenantID pgtype.UUID, period string) ([]gen.MvRevenueStream, error) {
	// Default: last 12 months
	fromMonth := pgtype.Date{Time: time.Now().AddDate(-1, 0, 0), Valid: true}
	if period == "quarterly" {
		fromMonth = pgtype.Date{Time: time.Now().AddDate(-2, 0, 0), Valid: true}
	}
	return s.store.RevenueStreams(ctx, gen.AdvancedAnalyticsRevenueStreamsParams{
		TenantID:  tenantID,
		FromMonth: fromMonth,
	})
}

// GetAdPerformance returns ad campaign aggregate performance.
func (s *Service) GetAdPerformance(ctx context.Context, tenantID pgtype.UUID, status string, limit int) ([]gen.AdvancedAnalyticsAdPerformanceRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.store.AdPerformance(ctx, gen.AdvancedAnalyticsAdPerformanceParams{
		TenantID: tenantID,
		Status:   status,
		TopN:     int32(limit),
	})
}

// GetNewsletterEngagement returns newsletter open/click rates.
func (s *Service) GetNewsletterEngagement(ctx context.Context, tenantID pgtype.UUID, limit int) ([]gen.AdvancedAnalyticsNewsletterEngagementRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.store.NewsletterEngagement(ctx, gen.AdvancedAnalyticsNewsletterEngagementParams{
		TenantID: tenantID,
		TopN:     int32(limit),
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
		TenantID: tenantID,
		FromDate: fromDate,
		TopN:     int32(limit),
	})
}

// ExportCSV exports analytics data as CSV.
func (s *Service) ExportCSV(ctx context.Context, tenantID pgtype.UUID, metric, period string) (string, []string, error) {
	enabled, err := s.settings.GetSettingBool(ctx, tenantID, "analytics.export_enabled")
	if err != nil || !enabled {
		return "", nil, fmt.Errorf("advancedanalytics: CSV export is disabled")
	}

	switch metric {
	case "top_content":
		rows, err := s.GetTopContent(ctx, tenantID, period, "", 100)
		if err != nil {
			return "", nil, err
		}
		headers := []string{"content_id", "title", "slug", "content_type", "total_views", "total_reactions", "total_comments", "engagement_score"}
		var sb strings.Builder
		sb.WriteString(strings.Join(headers, ",") + "\n")
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
			sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%d,%d,%d,%d\n",
				r.ContentID.String(), title, slug, contentType,
				r.TotalViews, r.TotalReactions, r.TotalComments, r.EngagementScore))
		}
		return sb.String(), headers, nil

	case "revenue_streams":
		rows, err := s.GetRevenueStreams(ctx, tenantID, period)
		if err != nil {
			return "", nil, err
		}
		headers := []string{"month", "subscription_revenue", "course_revenue", "ad_revenue", "credit_revenue"}
		var sb strings.Builder
		sb.WriteString(strings.Join(headers, ",") + "\n")
		for _, r := range rows {
			monthStr := ""
			if r.Month.Valid {
				monthStr = r.Month.Time.Format("2006-01-02")
			}
			sb.WriteString(fmt.Sprintf("%s,%d,%d,%d,%d\n",
				monthStr, r.SubscriptionRevenue, r.CourseRevenue, r.AdRevenue, r.CreditRevenue))
		}
		return sb.String(), headers, nil

	case "ad_performance":
		rows, err := s.GetAdPerformance(ctx, tenantID, "", 100)
		if err != nil {
			return "", nil, err
		}
		headers := []string{"campaign_id", "campaign_name", "status", "impressions", "clicks", "ctr", "spent"}
		var sb strings.Builder
		sb.WriteString(strings.Join(headers, ",") + "\n")
		for _, r := range rows {
			sb.WriteString(fmt.Sprintf("%s,%s,%s,%d,%d,%d,%d\n",
				r.CampaignID.String(), r.CampaignName, r.Status,
				r.Impressions, r.Clicks, r.Ctr, r.Spent))
		}
		return sb.String(), headers, nil

	default:
		return "", nil, fmt.Errorf("advancedanalytics: unknown export metric: %s", metric)
	}
}

// --- HTTP Handlers ---

// Handlers exposes the advanced analytics HTTP routes.
type Handlers struct {
	svc *Service
}

// NewHandlers creates advanced analytics handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// HandleTopContent returns top content by engagement.
// GET /admin/analytics/top-content
func (h *Handlers) HandleTopContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = Period7d
	}
	contentType := r.URL.Query().Get("type")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, err := h.svc.GetTopContent(r.Context(), tenantID, period, contentType, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"content": rows})
}

// HandleRevenueStreams returns revenue breakdown.
// GET /admin/analytics/revenue-streams
func (h *Handlers) HandleRevenueStreams(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "monthly"
	}

	rows, err := h.svc.GetRevenueStreams(r.Context(), tenantID, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"revenue": rows})
}

// HandleAdPerformance returns ad campaign aggregate stats.
// GET /admin/analytics/ads
func (h *Handlers) HandleAdPerformance(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, err := h.svc.GetAdPerformance(r.Context(), tenantID, status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ads": rows})
}

// HandleNewsletterEngagement returns newsletter open/click rates.
// GET /admin/analytics/newsletter
func (h *Handlers) HandleNewsletterEngagement(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	campaigns, err := h.svc.GetNewsletterEngagement(r.Context(), tenantID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	lists, err := h.svc.GetSubscriberGrowth(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"campaigns": campaigns,
		"lists":     lists,
	})
}

// HandleRetention returns retention cohorts.
// GET /admin/analytics/retention
func (h *Handlers) HandleRetention(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, err := h.svc.GetRetentionCohorts(r.Context(), tenantID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"cohorts": rows})
}

// HandleExport exports analytics as CSV.
// GET /admin/analytics/export
func (h *Handlers) HandleExport(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}

	metric := r.URL.Query().Get("metric")
	period := r.URL.Query().Get("period")
	if metric == "" {
		writeError(w, http.StatusBadRequest, "missing_param", "metric is required")
		return
	}

	csvData, _, err := h.svc.ExportCSV(r.Context(), tenantID, metric, period)
	if err != nil {
		writeError(w, http.StatusBadRequest, "export_failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+metric+"_export.csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(csvData))
}

// --- shared helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"status":  status,
		},
	})
}

type tenantCtxKey struct{}
type userCtxKey struct{}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	v := r.Context().Value(tenantCtxKey{})
	if v == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Tenant context required")
		return pgtype.UUID{}, false
	}
	id, ok := v.(pgtype.UUID)
	if !ok || !id.Valid {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid tenant context")
		return pgtype.UUID{}, false
	}
	return id, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	v := r.Context().Value(userCtxKey{})
	if v == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}
	id, ok := v.(pgtype.UUID)
	if !ok || !id.Valid {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid user context")
		return pgtype.UUID{}, false
	}
	return id, true
}

// keep csv import
var _ = csv.NewWriter
