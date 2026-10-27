// Package ai — tools_releases.go implements the §113 versioned-releases
// admin surface for the agent catalog (ADR-135): release lifecycle
// management, shipped-item attachment, and commit-entry ingest for
// changelog automation — gated by the same roadmap.* RBAC nodes as the
// /admin/roadmap/* REST routes.
//
// Tools implemented here:
//
//   - roadmap.releases.list        paginated release list (filters)
//   - roadmap.release.get          single release by UUID
//   - roadmap.release.create       draft or directly-published release
//   - roadmap.release.update       patch release fields
//   - roadmap.release.set_status   lifecycle; publish absorbs the entry queue
//   - roadmap.release.attach_items explicit ids or shipped_at window
//   - roadmap.entries.list         entry queue (unreleased/type/scope filters)
//   - roadmap.entries.ingest       CI path — GitHub push-shape commits
//
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
//      (Versioned Releases, Commit-Sourced Entries)
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// releaseArgs is the shared argument shape for create/update.
type releaseArgs struct {
	ID         *string `json:"id"`
	Version    *string `json:"version"`
	Slug       *string `json:"slug"`
	Title      *string `json:"title"`
	Summary    *string `json:"summary"`
	Body       *string `json:"body"`
	Surface    *string `json:"surface"`
	IsESR      *bool   `json:"is_esr"`
	ReleasedAt *string `json:"released_at"`
	EOLAt      *string `json:"eol_at"`
	SortOrder  *int32  `json:"sort_order"`
	Status     *string `json:"status"`
}

func (a releaseArgs) toInput() RoadmapReleaseInput {
	return RoadmapReleaseInput{
		Version: a.Version, Slug: a.Slug, Title: a.Title, Summary: a.Summary,
		Body: a.Body, Surface: a.Surface, IsESR: a.IsESR,
		ReleasedAt: a.ReleasedAt, EOLAt: a.EOLAt, SortOrder: a.SortOrder,
		Status: a.Status,
	}
}

func releasesSvc(deps *ToolDeps) RoadmapReleaseService {
	if deps == nil {
		return nil
	}
	return deps.RoadmapReleases
}

func releasesUnavailable() (*ToolResult, error) {
	return errorToolResult("roadmap_unavailable", "roadmap releases service is not configured"), nil
}

// parseISODate accepts RFC 3339 plus the timezone-less
// "2006-01-02T15:04" and "2006-01-02" shapes (UTC).
func parseISODate(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// RoadmapReleasesListHandler lists releases with surface/status filters.
func RoadmapReleasesListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		svc := releasesSvc(deps)
		if svc == nil {
			return releasesUnavailable()
		}
		var a struct {
			Surface *string `json:"surface"`
			Status  *string `json:"status"`
			Page    *int    `json:"page"`
			Limit   *int    `json:"limit"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		page, limit := 1, 25
		if a.Page != nil && *a.Page > 0 {
			page = *a.Page
		}
		if a.Limit != nil && *a.Limit > 0 {
			limit = min(*a.Limit, roadmapListLimitCap)
		}
		var surface, status string
		if a.Surface != nil {
			surface = *a.Surface
		}
		if a.Status != nil {
			status = *a.Status
		}
		out, err := svc.ListReleases(ctx, RoadmapReleaseFilter{
			Surface: surface, Status: status,
			PageSize: int32(limit), PageOffset: int32((page - 1) * limit),
		})
		if err != nil {
			return roadmapErrResult("list_failed", err), nil
		}
		return successToolResult(out), nil
	}
}

// RoadmapReleaseGetHandler reads one release by UUID.
func RoadmapReleaseGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		svc := releasesSvc(deps)
		if svc == nil {
			return releasesUnavailable()
		}
		var a struct {
			ID *string `json:"id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ID == nil || *a.ID == "" {
			return errorToolResult("missing_id", "id is required"), nil
		}
		id, err := scanUUID(*a.ID)
		if err != nil {
			return errorToolResult("invalid_id", "id is not a valid UUID"), nil
		}
		rel, err := svc.GetRelease(ctx, id)
		if err != nil {
			return roadmapErrResult("get_failed", err), nil
		}
		return successToolResult(rel), nil
	}
}

// RoadmapReleaseCreateHandler creates a release (draft or published).
func RoadmapReleaseCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		svc := releasesSvc(deps)
		if svc == nil {
			return releasesUnavailable()
		}
		var a releaseArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Version == nil || *a.Version == "" {
			return errorToolResult("missing_version", "version is required"), nil
		}
		rel, err := svc.CreateRelease(ctx, a.toInput())
		if err != nil {
			return roadmapErrResult("create_failed", err), nil
		}
		return successToolResult(rel), nil
	}
}

// RoadmapReleaseUpdateHandler patches release fields.
func RoadmapReleaseUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		svc := releasesSvc(deps)
		if svc == nil {
			return releasesUnavailable()
		}
		var a releaseArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ID == nil || *a.ID == "" {
			return errorToolResult("missing_id", "id is required"), nil
		}
		id, err := scanUUID(*a.ID)
		if err != nil {
			return errorToolResult("invalid_id", "id is not a valid UUID"), nil
		}
		rel, err := svc.UpdateRelease(ctx, id, a.toInput())
		if err != nil {
			return roadmapErrResult("update_failed", err), nil
		}
		return successToolResult(rel), nil
	}
}

// RoadmapReleaseSetStatusHandler drives draft → published → yanked with
// the unreleased-entry absorb sweep on publish.
func RoadmapReleaseSetStatusHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		svc := releasesSvc(deps)
		if svc == nil {
			return releasesUnavailable()
		}
		var a struct {
			ID               *string `json:"id"`
			Status           *string `json:"status"`
			AbsorbUnreleased *bool   `json:"absorb_unreleased"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ID == nil || *a.ID == "" {
			return errorToolResult("missing_id", "id is required"), nil
		}
		if a.Status == nil || (*a.Status != "draft" && *a.Status != "published" && *a.Status != "yanked") {
			return errorToolResult("invalid_status", "status must be draft, published, or yanked"), nil
		}
		id, err := scanUUID(*a.ID)
		if err != nil {
			return errorToolResult("invalid_id", "id is not a valid UUID"), nil
		}
		absorb := true
		if a.AbsorbUnreleased != nil {
			absorb = *a.AbsorbUnreleased
		}
		rel, err := svc.SetReleaseStatus(ctx, id, *a.Status, absorb)
		if err != nil {
			return roadmapErrResult("set_status_failed", err), nil
		}
		return successToolResult(rel), nil
	}
}

// RoadmapReleaseAttachItemsHandler attaches shipped items by id or by
// shipped_at window.
func RoadmapReleaseAttachItemsHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		svc := releasesSvc(deps)
		if svc == nil {
			return releasesUnavailable()
		}
		var a struct {
			ID           *string   `json:"id"`
			ItemIDs      *[]string `json:"item_ids"`
			ShippedSince *string   `json:"shipped_since"`
			ShippedUntil *string   `json:"shipped_until"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ID == nil || *a.ID == "" {
			return errorToolResult("missing_id", "id is required"), nil
		}
		id, err := scanUUID(*a.ID)
		if err != nil {
			return errorToolResult("invalid_id", "id is not a valid UUID"), nil
		}
		if a.ItemIDs != nil {
			ids := make([]pgtype.UUID, 0, len(*a.ItemIDs))
			for _, raw := range *a.ItemIDs {
				uid, err := scanUUID(raw)
				if err != nil {
					return errorToolResult("invalid_item_id", "item_ids must be UUIDs: "+raw), nil
				}
				ids = append(ids, uid)
			}
			if err := svc.ReplaceReleaseItems(ctx, id, ids); err != nil {
				return roadmapErrResult("attach_failed", err), nil
			}
			return successToolResult(map[string]any{"attached": len(ids)}), nil
		}
		if a.ShippedSince != nil || a.ShippedUntil != nil {
			var since, until *time.Time
			if a.ShippedSince != nil {
				t, ok := parseISODate(*a.ShippedSince)
				if !ok {
					return errorToolResult("invalid_date", "shipped_since must be ISO 8601"), nil
				}
				since = &t
			}
			if a.ShippedUntil != nil {
				t, ok := parseISODate(*a.ShippedUntil)
				if !ok {
					return errorToolResult("invalid_date", "shipped_until must be ISO 8601"), nil
				}
				until = &t
			}
			if err := svc.AttachItemsByWindow(ctx, id, since, until); err != nil {
				return roadmapErrResult("attach_failed", err), nil
			}
			return successToolResult(map[string]any{"attached": "window"}), nil
		}
		return errorToolResult("invalid_arguments", "provide item_ids or a shipped_since/shipped_until window"), nil
	}
}

// RoadmapEntriesListHandler lists changelog entries with filters.
func RoadmapEntriesListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		svc := releasesSvc(deps)
		if svc == nil {
			return releasesUnavailable()
		}
		var a struct {
			Unreleased *bool   `json:"unreleased"`
			Type       *string `json:"type"`
			Scope      *string `json:"scope"`
			Page       *int    `json:"page"`
			Limit      *int    `json:"limit"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		page, limit := 1, 25
		if a.Page != nil && *a.Page > 0 {
			page = *a.Page
		}
		if a.Limit != nil && *a.Limit > 0 {
			limit = min(*a.Limit, roadmapListLimitCap)
		}
		f := RoadmapEntryFilter{PageSize: int32(limit), PageOffset: int32((page - 1) * limit)}
		if a.Unreleased != nil {
			f.Unreleased = *a.Unreleased
		}
		if a.Type != nil {
			f.CommitType = *a.Type
		}
		if a.Scope != nil {
			f.Scope = *a.Scope
		}
		out, err := svc.ListEntries(ctx, f)
		if err != nil {
			return roadmapErrResult("list_failed", err), nil
		}
		return successToolResult(out), nil
	}
}

// RoadmapEntriesIngestHandler ingests GitHub push-shape commits — the CI
// path for changelog automation.
func RoadmapEntriesIngestHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		svc := releasesSvc(deps)
		if svc == nil {
			return releasesUnavailable()
		}
		var a struct {
			Commits []RoadmapGitHubCommit `json:"commits"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if len(a.Commits) == 0 {
			return errorToolResult("missing_commits", "commits is required"), nil
		}
		created, skipped, err := svc.IngestCommits(ctx, a.Commits)
		if err != nil {
			return roadmapErrResult("ingest_failed", err), nil
		}
		return successToolResult(map[string]any{"created": created, "skipped": skipped}), nil
	}
}
