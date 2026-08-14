// Package server — agent_tools_registry.go adapts the per-tool services
// into the ai.ToolsRegistryService interface for the AI Agent Executor's
// tools.list / tools.read handlers (Section 103, Phase 9).
//
// The adapter queries record counts from each tool table and reads
// individual records by ID, returning them as map[string]any so the ai
// package does not import internal/scholarships, internal/visaroutes,
// etc.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tools app tools)
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
// count and read queries to avoid importing each tool package.
type agentToolsRegistry struct {
	reader pgxQuerier
}

// pgxQuerier is the subset of *pgxpool.Pool needed for count/read queries.
type pgxQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// newAgentToolsRegistry creates a ToolsRegistryService backed by the
// pgx reader pool.
func newAgentToolsRegistry(reader pgxQuerier) *agentToolsRegistry {
	return &agentToolsRegistry{reader: reader}
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
// records. The readSQL selects a single row by id + tenant_id.
var toolCatalog = []toolCatalogDef{
	{
		toolType: "scholarship",
		label:    "Scholarships",
		category: "financial",
		countSQL: "SELECT COUNT(*) FROM scholarships WHERE tenant_id = $1",
		readSQL:  "SELECT id, title, description, country, university, degree_level, deadline, application_url, status, created_at FROM scholarships WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "visa_route",
		label:    "Visa Routes",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM visa_routes WHERE tenant_id = $1",
		readSQL:  "SELECT id, name, slug, visa_category, gov_source_url, status, is_active, created_at FROM visa_routes WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "housing_guide",
		label:    "Housing Guides",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM housing_guides WHERE tenant_id = $1",
		readSQL:  "SELECT id, destination_space_id, accommodation_type, status, is_active, created_at FROM housing_guides WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "attestation_guide",
		label:    "Attestation Guides",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM attestation_pathways WHERE tenant_id = $1",
		readSQL:  "SELECT id, document_type, issuing_space_id, destination_space_id, status, is_active, created_at FROM attestation_pathways WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "arrival_checklist",
		label:    "Arrival Checklists",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM arrival_checklist_guides WHERE tenant_id = $1",
		readSQL:  "SELECT id, destination_space_id, visa_category, applicant_type, version, status, is_active, created_at FROM arrival_checklist_guides WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "credential_pathway",
		label:    "Credential Pathways",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM credential_pathways WHERE tenant_id = $1",
		readSQL:  "SELECT id, profession_id, issuing_space_id, destination_space_id, regulatory_body, pathway_type, status, created_at FROM credential_pathways WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "language_test_requirement",
		label:    "Language Test Requirements",
		category: "immigration",
		countSQL: "SELECT COUNT(*) FROM language_test_requirements WHERE tenant_id = $1",
		readSQL:  "SELECT id, test_id, destination_space_id, visa_category, minimum_score, status, created_at FROM language_test_requirements WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "tax_obligation_guide",
		label:    "Tax Obligation Guides",
		category: "financial",
		countSQL: "SELECT COUNT(*) FROM tax_obligation_guides WHERE tenant_id = $1",
		readSQL:  "SELECT id, origin_space_id, destination_space_id, employment_status, status, is_active, created_at FROM tax_obligation_guides WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "community_organisation",
		label:    "Community Organisations",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM community_organisations WHERE tenant_id = $1",
		readSQL:  "SELECT id, name, org_type, destination_space_id, status, is_active, created_at FROM community_organisations WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "wiki_page",
		label:    "Wiki Pages",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM wiki_pages WHERE tenant_id = $1",
		readSQL:  "SELECT id, slug, title, category, space_id, is_locked, created_at FROM wiki_pages WHERE id = $1 AND tenant_id = $2",
	},
	{
		toolType: "calendar_entry",
		label:    "Calendar Entries",
		category: "community",
		countSQL: "SELECT COUNT(*) FROM calendar_entries WHERE tenant_id = $1",
		readSQL:  "SELECT id, title, category, destination_space_id, start_at, end_at, status, created_at FROM calendar_entries WHERE id = $1 AND tenant_id = $2",
	},
}

// ListTools returns the catalog of available Japa tools with record counts.
func (r *agentToolsRegistry) ListTools(ctx context.Context, tenantID pgtype.UUID) ([]ai.ToolCatalogEntry, error) {
	if !tenantID.Valid {
		return nil, fmt.Errorf("tools registry: tenant_id is required")
	}
	entries := make([]ai.ToolCatalogEntry, 0, len(toolCatalog))
	for _, def := range toolCatalog {
		var count int64
		err := r.reader.QueryRow(ctx, def.countSQL, tenantID).Scan(&count)
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
	if !tenantID.Valid {
		return nil, fmt.Errorf("tools registry: tenant_id is required")
	}
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

	rows, err := r.reader.Query(ctx, def.readSQL, idUUID, tenantID)
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

// Compile-time interface check.
var _ ai.ToolsRegistryService = (*agentToolsRegistry)(nil)
