// Package server — agent_tools_registry.go adapts the per-tool services
// into the ai.ToolsRegistryService interface for the AI Agent Executor's
// tools.list / tools.read handlers (Section 103, Phase 9).
// // The adapter queries record counts from each tool table and reads
// individual records by ID, returning them as map[string]any so the ai
// package does not import internal/scholarships, internal/visaroutes,
// etc.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tools app tools)
// Ref: japaconnect-engineering/sections/78-tools-web-surface.md
package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
)

// agentToolsRegistry adapts the per-tool services into the
// ai.ToolsRegistryService interface. It uses the pgx reader pool for
// count and read queries and the toolWriter map for create/update/delete/assign-geo.
type agentToolsRegistry struct {
	reader  pgxQuerier
	writers map[string]*toolWriter
}

// toolWriter holds the write/assign-geo closures for a single tool_type.
// Each closure adapts the generic map[string]any contract used by the
// ai package into the typed input/outputs of the per-tool service.
type toolWriter struct {
	create    func(ctx context.Context, tenantID, userID pgtype.UUID, payload map[string]any) (map[string]any, error)
	update    func(ctx context.Context, tenantID, userID pgtype.UUID, id string, payload map[string]any) (map[string]any, error)
	delete    func(ctx context.Context, tenantID, userID pgtype.UUID, id string) error
	assignGeo func(ctx context.Context, tenantID, userID pgtype.UUID, id string, payload map[string]any) (map[string]any, error)
}

// pgxQuerier is the subset of *pgxpool.Pool needed for count/read queries.
type pgxQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// newAgentToolsRegistry creates a ToolsRegistryService backed by the
// pgx reader pool and the optional write/assign-geo writer map.
func newAgentToolsRegistry(reader pgxQuerier, writers map[string]*toolWriter) *agentToolsRegistry {
	if writers == nil {
		writers = make(map[string]*toolWriter)
	}

	return &agentToolsRegistry{reader: reader, writers: writers}
}

// toolCatalogDef defines a tool in the catalog: its type, label, category,
// and the count query + read query used by ListTools / ReadTool.
type toolCatalogDef struct {
	toolType string
	label    string
	category string
	countSQL string
	readSQL  string
}

// toolCatalog is the static catalog of Japa tools. Each entry maps a
// tool_type to its backing table and the SQL needed to count and read
// records. The readSQL selects a single row by id + .
var toolCatalog = []toolCatalogDef{
	{
		toolType: "scholarship",
		label:    "Scholarships",
		category: "financial",
		countSQL: "SELECT COUNT(*) FROM scholarships",
		readSQL:  "SELECT id, title, description, country, country_space_id, university, degree_level, deadline, application_url, required_documents, minimum_gpa, language_requirements, verification_status, freshness_score, created_at FROM scholarships WHERE id = $1",
	},
	{
		toolType: "visa_route",
		label:    "Visa Routes",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM visa_routes",
		readSQL:  "SELECT id, name, slug, visa_category, gov_source_url, status, is_active, created_at FROM visa_routes WHERE id = $1",
	},
	{
		toolType: "housing_guide",
		label:    "Housing Guides",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM housing_guides",
		readSQL:  "SELECT id, destination_space_id, accommodation_type, status, is_active, created_at FROM housing_guides WHERE id = $1",
	},
	{
		toolType: "attestation_guide",
		label:    "Attestation Guides",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM attestation_pathways",
		readSQL:  "SELECT id, document_type, issuing_space_id, destination_space_id, status, is_active, created_at FROM attestation_pathways WHERE id = $1",
	},
	{
		toolType: "arrival_checklist",
		label:    "Arrival Checklists",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM arrival_checklist_guides",
		readSQL:  "SELECT id, destination_space_id, visa_category, applicant_type, version, status, is_active, created_at FROM arrival_checklist_guides WHERE id = $1",
	},
	{
		toolType: "credential_pathway",
		label:    "Credential Pathways",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM credential_pathways",
		readSQL:  "SELECT id, profession_id, issuing_space_id, destination_space_id, regulatory_body, pathway_type, status, created_at FROM credential_pathways WHERE id = $1",
	},
	{
		toolType: "language_test_requirement",
		label:    "Language Test Requirements",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM language_test_requirements",
		readSQL:  "SELECT id, test_id, destination_space_id, visa_category, minimum_score, status, created_at FROM language_test_requirements WHERE id = $1",
	},
	{
		toolType: "tax_obligation_guide",
		label:    "Tax Obligation Guides",
		category: "financial",
		countSQL: "SELECT COUNT(*) FROM tax_obligation_guides",
		readSQL:  "SELECT id, origin_space_id, destination_space_id, employment_status, status, is_active, created_at FROM tax_obligation_guides WHERE id = $1",
	},
	{
		toolType: "community_organisation",
		label:    "Community Organisations",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM community_organisations",
		readSQL:  "SELECT id, name, org_type, destination_space_id, status, is_active, created_at FROM community_organisations WHERE id = $1",
	},
	{
		toolType: "wiki_page",
		label:    "Wiki Pages",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM wiki_pages",
		readSQL:  "SELECT id, slug, title, category, space_id, is_locked, created_at FROM wiki_pages WHERE id = $1",
	},
	{
		toolType: "calendar_entry",
		label:    "Calendar Entries",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM calendar_entries",
		readSQL:  "SELECT id, title, category, destination_space_id, start_at, end_at, status, created_at FROM calendar_entries WHERE id = $1",
	},
	// V2 community features
	{
		toolType: "scam_report",
		label:    "Scam Reports",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM scam_reports",
		readSQL:  "SELECT id, reporter_id, scam_type, destination_space_id, origin_space_id, origin_community, platform_used, approximate_date, amount_range, description, reported_to_authorities, status, moderator_note, is_pinned, pinned_space_id, reviewed_by, reviewed_at, created_at FROM scam_reports WHERE id = $1",
	},
	{
		toolType: "planning_group",
		label:    "Planning Groups",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM planning_groups",
		readSQL:  "SELECT id, name, description, destination_space_id, visa_category, target_departure_window, target_departure_at, max_members, is_open_join, owner_id, expires_at, created_at FROM planning_groups WHERE id = $1",
	},
	{
		toolType: "visa_timeline",
		label:    "Visa Timelines",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM visa_timelines",
		readSQL:  "SELECT id, user_id, origin_space_id, destination_space_id, visa_category, consulate, application_method, outcome, is_shared, anonymised_at, created_at FROM visa_timelines WHERE id = $1",
	},
	{
		toolType: "processing_time",
		label:    "Visa Processing Times",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM visa_processing_stats",
		readSQL:  "SELECT id, origin_space_id, destination_space_id, visa_category, consulate, period_start, sample_count, avg_days, median_days, p10_days, p90_days, refreshed_at FROM visa_processing_stats WHERE id = $1",
	},
	{
		toolType: "digest_edition",
		label:    "Community Digests",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM digest_editions",
		readSQL:  "SELECT id, space_id, cadence, status, scheduled_for, published_at, content, email_sent, email_sent_at, generated_by, reviewed_by, created_at FROM digest_editions WHERE id = $1",
	},
	{
		toolType: "mutual_aid",
		label:    "Mutual Aid Posts",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM mutual_aid_posts",
		readSQL:  "SELECT id, user_id, post_type, category, title, description, destination_space_id, city_space_id, expires_at, is_fulfilled, status, created_at FROM mutual_aid_posts WHERE id = $1",
	},
	// V2 functional tools
	{
		toolType: "embassy",
		label:    "Embassy Monitors",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM monitored_embassies",
		readSQL:  "SELECT id, destination_space_id, slug, name, booking_url, poll_interval_seconds, is_active, last_polled_at, created_at FROM monitored_embassies WHERE id = $1",
	},
	{
		toolType: "appointment_alert",
		label:    "Appointment Alerts",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM appointment_alerts",
		readSQL:  "SELECT id, user_id, destination_space_id, embassy_slug, visa_category, date_range_start, date_range_end, status, last_checked_at, fired_at, expires_at, notification_channels, created_at FROM appointment_alerts WHERE id = $1",
	},
	{
		toolType: "credential_profession",
		label:    "Credential Professions",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM credential_professions",
		readSQL:  "SELECT id, slug, name, category, created_at FROM credential_professions WHERE id = $1",
	},
	{
		toolType: "cv_formatter",
		label:    "CV Format Standards",
		category: "career",
		countSQL: "SELECT COUNT(*) FROM cv_standards",
		readSQL:  "SELECT id, destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls, created_at FROM cv_standards WHERE id = $1",
	},
	{
		toolType: "language_test",
		label:    "Language Tests",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM language_tests",
		readSQL:  "SELECT id, slug, name, official_url, validity_months, created_at FROM language_tests WHERE id = $1",
	},
	{
		toolType: "mentorship_connection",
		label:    "Mentorship Connections",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM mentorship_connections",
		readSQL:  "SELECT id, mentor_id, mentee_id, status, goals, destination_space_id, responded_at, completed_at, cancelled_at, cancel_reason, created_at FROM mentorship_connections WHERE id = $1",
	},
	{
		toolType: "remittance_rate",
		label:    "Remittance Rates",
		category: "financial",
		countSQL: "SELECT COUNT(*) FROM remittance_rates",
		readSQL:  "SELECT id, provider_id, send_currency, receive_currency, rate, fee_flat, fee_pct, transfer_time, fetched_at, is_manual, source_url, intel_record_id, confidence_overall, verification_status, freshness_score FROM remittance_rates WHERE id = $1",
	},
	{
		toolType: "remittance_provider",
		label:    "Remittance Providers",
		category: "financial",
		countSQL: "SELECT COUNT(*) FROM remittance_providers",
		readSQL:  "SELECT id, name, slug, logo_media_id, website_url, data_source, is_active, display_order, created_at FROM remittance_providers WHERE id = $1",
	},
	{
		toolType: "readiness_assessment",
		label:    "Readiness Assessments",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM assessment_results",
		readSQL:  "SELECT id, user_id, submission_id, total_score, dimension_scores, threshold_label, roadmap_content, created_at FROM assessment_results WHERE id = $1",
	},
	{
		toolType: "tax_treaty_pair",
		label:    "Tax Treaty Pairs",
		category: "financial",
		countSQL: "SELECT COUNT(*) FROM tax_treaty_pairs",
		readSQL:  "SELECT id, origin_space_id, destination_space_id, treaty_exists, treaty_summary, treaty_source_url, created_at FROM tax_treaty_pairs WHERE id = $1",
	},
	{
		toolType: "travel_provider",
		label:    "Travel Providers",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM travel_providers",
		readSQL:  "SELECT id, name, slug, provider_type, api_config, affiliate_config, is_active, created_at FROM travel_providers WHERE id = $1",
	},
}

// ListTools returns the catalog of available Japa tools with record counts.
func (r *agentToolsRegistry) ListTools(ctx context.Context, tenantID pgtype.UUID) ([]ai.ToolCatalogEntry, error) {

	entries := make([]ai.ToolCatalogEntry, 0, len(toolCatalog))

	for _, def := range toolCatalog {
		var count int64

		err := r.reader.QueryRow(ctx, def.countSQL).Scan(&count)
		if err != nil {
			// Skip tools whose table may not exist yet or query fails.
			continue
		}

		entries = append(entries, ai.ToolCatalogEntry{
			ToolType: def.toolType,
			Label:    def.label,
			Count:    count,
			Category: def.category,
		})
	}

	return entries, nil
}

// ReadTool reads a single record from a tool table by type + ID.
func (r *agentToolsRegistry) ReadTool(ctx context.Context, tenantID pgtype.UUID, toolType, id string) (map[string]any, error) {

	var idUUID pgtype.UUID
	if err := idUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("tools registry: invalid tool_id: %w", err)
	}

	var def *toolCatalogDef

	for i := range toolCatalog {
		if toolCatalog[i].toolType == toolType {
			def = &toolCatalog[i]
			break
		}
	}

	if def == nil {
		return nil, fmt.Errorf("tools registry: unknown tool_type %q", toolType)
	}

	rows, err := r.reader.Query(ctx, def.readSQL, idUUID)
	if err != nil {
		return nil, fmt.Errorf("tools registry: query %s: %w", toolType, err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()

	values := make([]any, len(fieldDescs))
	for i := range values {
		values[i] = &values[i]
	}

	if !rows.Next() {
		return nil, fmt.Errorf("tools registry: %s %s not found", toolType, id)
	}

	if err := rows.Scan(values...); err != nil {
		return nil, fmt.Errorf("tools registry: scan %s: %w", toolType, err)
	}

	result := make(map[string]any, len(fieldDescs))

	for i, fd := range fieldDescs {
		colName := string(fd.Name)
		val := values[i]
		// Convert pgtype values to JSON-friendly types.
		switch v := val.(type) {
		case *pgtype.UUID:
			if v.Valid {
				result[colName] = v.String()
			} else {
				result[colName] = nil
			}
		case *pgtype.Timestamptz:
			if v.Valid {
				result[colName] = v.Time.Format("2006-01-02T15:04:05Z07:00")
			} else {
				result[colName] = nil
			}
		case *pgtype.Text:
			if v.Valid {
				result[colName] = v.String
			} else {
				result[colName] = nil
			}
		case *pgtype.Bool:
			if v.Valid {
				result[colName] = v.Bool
			} else {
				result[colName] = nil
			}
		case *pgtype.Int4:
			if v.Valid {
				result[colName] = v.Int32
			} else {
				result[colName] = nil
			}
		default:
			// Fallback: marshal to JSON then unmarshal to any.
			if b, err := json.Marshal(val); err == nil {
				var jv any
				if json.Unmarshal(b, &jv) == nil {
					result[colName] = jv
				} else {
					result[colName] = fmt.Sprintf("%v", val)
				}
			} else {
				result[colName] = fmt.Sprintf("%v", val)
			}
		}
	}

	return result, nil
}

// ToolTypes returns the tool_type values in the catalog.
func (r *agentToolsRegistry) ToolTypes() []string {
	types := make([]string, 0, len(toolCatalog))
	for _, def := range toolCatalog {
		types = append(types, def.toolType)
	}

	return types
}

func (r *agentToolsRegistry) findToolWriter(toolType string) (*toolWriter, error) {
	w, ok := r.writers[toolType]
	if !ok || w == nil {
		return nil, fmt.Errorf("tools registry: %q does not support write operations yet", toolType)
	}

	return w, nil
}

// CreateTool creates a new record in the named tool table.
func (r *agentToolsRegistry) CreateTool(ctx context.Context, tenantID, userID pgtype.UUID, toolType string, payload map[string]any) (map[string]any, error) {

	w, err := r.findToolWriter(toolType)
	if err != nil {
		return nil, err
	}

	if w.create == nil {
		return nil, fmt.Errorf("tools registry: %q does not support create yet", toolType)
	}

	return w.create(ctx, tenantID, userID, payload)
}

// UpdateTool updates an existing record in the named tool table.
func (r *agentToolsRegistry) UpdateTool(ctx context.Context, tenantID, userID pgtype.UUID, toolType, id string, payload map[string]any) (map[string]any, error) {

	w, err := r.findToolWriter(toolType)
	if err != nil {
		return nil, err
	}

	if w.update == nil {
		return nil, fmt.Errorf("tools registry: %q does not support update yet", toolType)
	}

	return w.update(ctx, tenantID, userID, id, payload)
}

// DeleteTool soft-deletes an existing record from the named tool table.
func (r *agentToolsRegistry) DeleteTool(ctx context.Context, tenantID, userID pgtype.UUID, toolType, id string) error {

	w, err := r.findToolWriter(toolType)
	if err != nil {
		return err
	}

	if w.delete == nil {
		return fmt.Errorf("tools registry: %q does not support delete yet", toolType)
	}

	return w.delete(ctx, tenantID, userID, id)
}

// AssignGeo updates the geo-scoping fields of an existing record in the named tool table.
func (r *agentToolsRegistry) AssignGeo(ctx context.Context, tenantID, userID pgtype.UUID, toolType, id string, payload map[string]any) (map[string]any, error) {

	w, err := r.findToolWriter(toolType)
	if err != nil {
		return nil, err
	}

	if w.assignGeo == nil {
		return nil, fmt.Errorf("tools registry: %q does not support assign_geo yet", toolType)
	}

	return w.assignGeo(ctx, tenantID, userID, id, payload)
}

// Compile-time interface check.
var _ ai.ToolsRegistryService = (*agentToolsRegistry)(nil)
