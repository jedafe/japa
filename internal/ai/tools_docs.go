// Package ai — tools_docs.go implements the documentation tool handlers
// for the AI Agent Executor extended tool catalog (Section 103, Phase 9).
//
// Tools implemented here (Phase 9, tasks 9.03):
//
//   - docs.list     — list all documentation pages (any status)
//   - docs.read     — read a single doc page by ID
//   - docs.create   — create a doc page (admin, HITL)
//   - docs.update   — update a doc page
//   - docs.organize — move/reorder pages in the nav tree
//
// Docs tools respect the existing nav-tree/document structure (Section 44):
// pages have an audience, parent_id, position, and review_status. The
// review workflow (draft → in_review → approved → published) is preserved
// — docs.create always creates with review_status="draft" and
// docs.update cannot directly publish (the agent never publishes directly,
// per §103).
//
// Privacy: docs are not user data and have no ai_exempt column, so no
// PrivacyTargetFunc is registered. The executor's RBAC gate enforces the
// required_permission on each tool.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Docs tools)
// Ref: japaconnect-engineering/sections/44-documentation-system.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, task 9.03)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// docs.list (task 9.03a)
// ---------------------------------------------------------------------------

// DocsListHandler returns a ToolHandler that lists all documentation pages
// for the tenant (any status, any audience — the agent needs the full
// catalog for admin operations).
func DocsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Docs == nil {
			return errorToolResult("docs_unavailable", "docs service is not configured"), nil
		}

		pages, err := deps.Docs.ListAllDocPages(ctx, tenantID)
		if err != nil {
			return errorToolResult("docs_list_failed", err.Error()), nil
		}

		results := make([]map[string]any, 0, len(pages))
		for _, p := range pages {
			results = append(results, docsPageToMap(p))
		}

		return successToolResult(map[string]any{
			"pages": results,
			"total": len(results),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// docs.read (task 9.03b)
// ---------------------------------------------------------------------------

// docsReadArgs is the argument shape for docs.read.
type docsReadArgs struct {
	DocID string `json:"doc_id"`
}

// DocsReadHandler returns a ToolHandler that reads a single doc page by ID.
func DocsReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Docs == nil {
			return errorToolResult("docs_unavailable", "docs service is not configured"), nil
		}

		var a docsReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		docID, err := scanUUID(a.DocID)
		if err != nil {
			return errorToolResult("invalid_doc_id", "doc_id is not a valid UUID"), nil
		}

		page, err := deps.Docs.GetDocPageByID(ctx, tenantID, docID)
		if err != nil {
			return errorToolResult("doc_not_found", fmt.Sprintf("doc not found: %v", err)), nil
		}

		return successToolResult(map[string]any{
			"page": docsPageToMap(*page),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// docs.create (task 9.03c)
// ---------------------------------------------------------------------------

// docsCreateArgs is the LLM-produced argument shape for docs.create.
type docsCreateArgs struct {
	Title        string          `json:"title"`
	Slug         string          `json:"slug"`
	Audience     string          `json:"audience"`
	Content      json.RawMessage `json:"content,omitempty"`
	AIGenerated  bool            `json:"ai_generated,omitempty"`
	ReviewStatus string          `json:"review_status,omitempty"`
	ParentID     *string         `json:"parent_id,omitempty"`
	Position     int32           `json:"position,omitempty"`
}

// DocsCreateHandler returns a ToolHandler that creates a documentation
// page. The executor's RBAC gate checks the required_permission
// (docs.admin); the tool config sets requires_hitl = true. The review
// status defaults to "draft" — the agent never publishes directly.
func DocsCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Docs == nil {
			return errorToolResult("docs_unavailable", "docs service is not configured"), nil
		}

		var a docsCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}
		if a.Slug == "" {
			return errorToolResult("missing_slug", "slug is required"), nil
		}
		if a.Audience == "" {
			return errorToolResult("missing_audience", "audience is required"), nil
		}

		// The agent never publishes directly (§44 + §103) — the
		// approved → published transition is a human reviewer action.
		if a.ReviewStatus == "published" {
			return errorToolResult("cannot_publish_directly", "the agent cannot publish directly; use review_status 'draft' or 'in_review' and let a human reviewer approve + publish"), nil
		}

		reviewStatus := a.ReviewStatus
		if reviewStatus == "" {
			reviewStatus = "draft"
		}

		content := a.Content
		if len(content) == 0 {
			content = json.RawMessage(`""`)
		}

		id, err := deps.Docs.CreateDocPage(ctx, tenantID, DocsCreatePageRequest{
			Title:        a.Title,
			Slug:         a.Slug,
			Audience:     a.Audience,
			Content:      content,
			AIGenerated:  true, // Always mark AI-created docs
			ReviewStatus: reviewStatus,
			ParentID:     a.ParentID,
			Position:     a.Position,
		})
		if err != nil {
			return errorToolResult("docs_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"doc_id":        uuidString(id),
			"title":         a.Title,
			"slug":          a.Slug,
			"audience":      a.Audience,
			"review_status": reviewStatus,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// docs.update (task 9.03d)
// ---------------------------------------------------------------------------

// docsUpdateArgs is the LLM-produced argument shape for docs.update.
type docsUpdateArgs struct {
	DocID        string          `json:"doc_id"`
	Title        string          `json:"title,omitempty"`
	Content      json.RawMessage `json:"content,omitempty"`
	ReviewStatus string          `json:"review_status,omitempty"`
}

// DocsUpdateHandler returns a ToolHandler that updates a documentation
// page. The review_status cannot be set to "published" directly by the
// agent — the §44 workflow requires approved → published transition via
// the human reviewer. The handler rejects "published" status.
func DocsUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Docs == nil {
			return errorToolResult("docs_unavailable", "docs service is not configured"), nil
		}

		var a docsUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		docID, err := scanUUID(a.DocID)
		if err != nil {
			return errorToolResult("invalid_doc_id", "doc_id is not a valid UUID"), nil
		}

		// The agent never publishes directly (§44 + §103).
		if a.ReviewStatus == "published" {
			return errorToolResult("cannot_publish_directly", "the agent cannot publish directly; use review_status 'draft' or 'in_review' and let a human reviewer approve + publish"), nil
		}

		reviewStatus := a.ReviewStatus
		if reviewStatus == "" {
			reviewStatus = "draft"
		}

		if err := deps.Docs.UpdateDocPage(ctx, tenantID, docID, DocsUpdatePageRequest{
			Title:        a.Title,
			Content:      a.Content,
			ReviewStatus: reviewStatus,
		}); err != nil {
			return errorToolResult("docs_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"doc_id":        uuidString(docID),
			"review_status": reviewStatus,
			"updated":       true,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// docs.organize (task 9.03e)
// ---------------------------------------------------------------------------

// docsOrganizeArgs is the LLM-produced argument shape for docs.organize.
type docsOrganizeArgs struct {
	Entries []docsOrganizeEntry `json:"entries"`
}

type docsOrganizeEntry struct {
	DocID    string  `json:"doc_id"`
	ParentID *string `json:"parent_id,omitempty"`
	Position int32   `json:"position"`
}

// DocsOrganizeHandler returns a ToolHandler that reorganizes doc pages in
// the nav tree by updating their parent_id and position. Requires the
// DocsOrganizer bridge — nil returns "not configured".
func DocsOrganizeHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil {
			return errorToolResult("docs_unavailable", "tool deps not configured"), nil
		}
		if deps.DocsOrganize == nil {
			return errorToolResult("docs_organize_unavailable", "docs.organize bridge is not wired — the docs service does not currently expose a parent_id/position update method"), nil
		}

		var a docsOrganizeArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if len(a.Entries) == 0 {
			return errorToolResult("empty_entries", "at least one organize entry is required"), nil
		}

		entries := make([]DocsOrganizeEntry, 0, len(a.Entries))
		for _, e := range a.Entries {
			id, err := scanUUID(e.DocID)
			if err != nil {
				return errorToolResult("invalid_doc_id", fmt.Sprintf("doc_id %q is not a valid UUID", e.DocID)), nil
			}
			var parentID *pgtype.UUID
			if e.ParentID != nil && *e.ParentID != "" {
				pid, err := scanUUID(*e.ParentID)
				if err != nil {
					return errorToolResult("invalid_parent_id", fmt.Sprintf("parent_id %q is not a valid UUID", *e.ParentID)), nil
				}
				parentID = &pid
			}
			entries = append(entries, DocsOrganizeEntry{
				ID:       id,
				ParentID: parentID,
				Position: e.Position,
			})
		}

		if err := deps.DocsOrganize(ctx, tenantID, entries); err != nil {
			return errorToolResult("docs_organize_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"organized": len(entries),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// Shared converter
// ---------------------------------------------------------------------------

func docsPageToMap(p DocsPage) map[string]any {
	m := map[string]any{
		"id":            p.ID,
		"title":         p.Title,
		"slug":          p.Slug,
		"audience":      p.Audience,
		"ai_generated":  p.AIGenerated,
		"review_status": p.ReviewStatus,
		"position":      p.Position,
		"created_at":    p.CreatedAt,
		"updated_at":    p.UpdatedAt,
	}
	if p.ParentID != nil {
		m["parent_id"] = *p.ParentID
	}
	return m
}
