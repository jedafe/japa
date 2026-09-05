// Package ai — tools_content_bulk.go implements bulk content creation and
// publishing tools for the AI agent (Section 103).
//
// Bulk operations are HITL-gated as a defense-in-depth control against large
// unreviewed writes. The handlers are implemented as thin loops over the same
// single-item service functions used by content.create and content.publish.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (HITL Rules)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content tools)
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// bulkCreateItem is the per-item argument shape for content.bulk_create.
type bulkCreateItem struct {
	Title          string            `json:"title"`
	ContentType    string            `json:"content_type"`
	Body           string            `json:"body,omitempty"`
	AuthorID       string            `json:"author_id,omitempty"`
	SpaceID        string            `json:"space_id,omitempty"`
	GeoSpaceID     string            `json:"geo_space_id,omitempty"`
	OrganizationID string            `json:"organization_id,omitempty"`
	PublishAt      string            `json:"publish_at,omitempty"`
	GuideCategory  string            `json:"guide_category,omitempty"`
	FieldValues    map[string]any    `json:"field_values,omitempty"`
	Provenance     *aiProvenanceArgs `json:"_ai_provenance,omitempty"`
}

// bulkCreateArgs is the LLM-produced argument shape for content.bulk_create.
type bulkCreateArgs struct {
	Items []bulkCreateItem `json:"items"`
}

// bulkCreateResult is the per-item return shape for content.bulk_create.
type bulkCreateResult struct {
	Index     int    `json:"index"`
	ContentID string `json:"content_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Slug      string `json:"slug,omitempty"`
	Status    string `json:"status,omitempty"`
	AuthorID  string `json:"author_id,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// bulkMaxItems returns the configured maximum number of items for a bulk tool,
// falling back to 50 when the settings reader is unavailable. The limit is
// controlled by system_settings keys agent.bulk_create_max_items and
// agent.bulk_publish_max_items.
//
// Ref: japaconnect-engineering/conventions/system-settings-catalog.md.
func bulkMaxItems(ctx context.Context, settings SettingsReader, tenantID pgtype.UUID, key string) int {
	if settings == nil {
		return 50
	}

	return settings.GetIntOr(ctx, tenantID, key, 50)
}

// ContentBulkCreateHandler returns a ToolHandler that creates multiple content
// rows in one call. Each item is validated, permission-checked, and passed to
// the ContentCreator individually; partial success is reported.
//
// Permissions: content.create (executor catalog gate). Per-item author changes
// require content.assign_author. Per-content-type agent grants may gate the
// requested content_type (see agent.content_type.*).
//
// HITL: the tool config sets requires_hitl=true because bulk creation is a bulk
// operation under §103 HITL Rules.
func ContentBulkCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ContentCreator == nil {
			return errorToolResult("content_bulk_create_unavailable", "content bulk creation is not wired"), nil
		}

		var a bulkCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if len(a.Items) == 0 {
			return errorToolResult("missing_items", "items array is required"), nil
		}

		maxItems := bulkMaxItems(ctx, deps.Settings, tenantID, "agent.bulk_create_max_items")
		if len(a.Items) > maxItems {
			return errorToolResult("too_many_items", fmt.Sprintf("bulk_create supports at most %d items", maxItems)), nil
		}

		permittedTypes, err := allowedContentTypes(ctx, deps, userID, tenantID)
		if err != nil {
			return errorToolResult("content_type_check_failed", err.Error()), nil
		}

		results := make([]bulkCreateResult, 0, len(a.Items))
		successCount := 0
		errorCount := 0

		for i, item := range a.Items {
			res, created := createOneBulkItem(ctx, deps, userID, tenantID, i, item, permittedTypes)
			results = append(results, res)

			if created {
				successCount++
			} else {
				errorCount++
			}
		}

		return successToolResult(map[string]any{
			"succeeded": successCount,
			"failed":    errorCount,
			"total":     len(a.Items),
			"results":   results,
		}), nil
	}
}

// createOneBulkItem creates a single content row from a bulk item.
func createOneBulkItem(ctx context.Context, deps *ToolDeps, userID, tenantID pgtype.UUID, idx int, item bulkCreateItem, permittedTypes map[string]bool) (bulkCreateResult, bool) {
	res := bulkCreateResult{Index: idx, Title: item.Title}

	if item.Title == "" {
		res.ErrorCode = "missing_title"
		res.Error = "title is required"

		return res, false
	}

	if item.ContentType == "" {
		res.ErrorCode = "missing_content_type"
		res.Error = "content_type is required"

		return res, false
	}

	if ok, msg := isContentTypePermitted(permittedTypes, item.ContentType); !ok {
		res.ErrorCode = "unauthorized_content_type"
		res.Error = msg

		return res, false
	}

	// Resolve and validate author assignment.
	authorID, err := scanUUID(item.AuthorID)
	if err != nil || !authorID.Valid {
		authorID = userID
	}

	if authorID.Valid && authorID != userID && !permissionCanAssignAuthor(ctx, deps, userID, tenantID) {
		res.ErrorCode = "unauthorized_author"
		res.Error = "setting author_id to another user requires content.assign_author permission"

		return res, false
	}

	spaceID, err := optionalUUID(item.SpaceID)
	if err != nil {
		res.ErrorCode = "invalid_space_id"
		res.Error = "space_id is not a valid UUID"

		return res, false
	}

	geoSpaceID, err := optionalUUID(item.GeoSpaceID)
	if err != nil {
		res.ErrorCode = "invalid_geo_space_id"
		res.Error = "geo_space_id is not a valid UUID"

		return res, false
	}

	organizationID, err := optionalUUID(item.OrganizationID)
	if err != nil {
		res.ErrorCode = "invalid_organization_id"
		res.Error = "organization_id is not a valid UUID"

		return res, false
	}

	var publishAt pgtype.Timestamptz

	if item.PublishAt != "" {
		t, err := time.Parse(time.RFC3339, item.PublishAt)
		if err != nil {
			res.ErrorCode = "invalid_publish_at"
			res.Error = "publish_at is not a valid RFC 3339 timestamp"

			return res, false
		}

		publishAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	fieldValues := item.FieldValues
	if fieldValues == nil {
		fieldValues = map[string]any{}
	}

	fieldValues["_ai_provenance"] = buildProvenance(item.Provenance, userID)

	fvJSON, err := json.Marshal(fieldValues)
	if err != nil {
		res.ErrorCode = "field_values_error"
		res.Error = fmt.Sprintf("failed to encode field values: %v", err)

		return res, false
	}

	row, err := deps.ContentCreator(ctx, tenantID, ContentCreateInput{
		AuthorID:       authorID,
		SpaceID:        spaceID,
		ContentType:    item.ContentType,
		Title:          item.Title,
		Body:           item.Body,
		FieldValues:    fvJSON,
		GeoSpaceID:     geoSpaceID,
		Status:         "draft",
		GuideCategory:  item.GuideCategory,
		OrganizationID: organizationID,
		PublishAt:      publishAt,
	})
	if err != nil {
		res.ErrorCode = "content_create_failed"
		res.Error = err.Error()

		return res, false
	}

	res.ContentID = uuidString(row.ID)
	res.Slug = row.Slug
	res.Status = row.Status
	res.AuthorID = uuidString(row.AuthorID)

	return res, true
}

// bulkPublishArgs is the LLM-produced argument shape for content.bulk_publish.
type bulkPublishArgs struct {
	ContentIDs []string `json:"content_ids"`
	Status     string   `json:"status,omitempty"`
}

// bulkPublishResult is the per-item return shape for content.bulk_publish.
type bulkPublishResult struct {
	Index     int    `json:"index"`
	ContentID string `json:"content_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Slug      string `json:"slug,omitempty"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// ContentBulkPublishHandler returns a ToolHandler that transitions the status
// of multiple content rows in one call. It is HITL-gated as a bulk operation.
//
// Permissions: content.publish (executor catalog gate).
func ContentBulkPublishHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ContentLifecycle == nil {
			return errorToolResult("content_bulk_publish_unavailable", "content lifecycle service is not configured"), nil
		}

		var a bulkPublishArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if len(a.ContentIDs) == 0 {
			return errorToolResult("missing_content_ids", "content_ids array is required"), nil
		}

		maxItems := bulkMaxItems(ctx, deps.Settings, tenantID, "agent.bulk_publish_max_items")
		if len(a.ContentIDs) > maxItems {
			return errorToolResult("too_many_items", fmt.Sprintf("bulk_publish supports at most %d items", maxItems)), nil
		}

		status := a.Status
		if status == "" {
			status = "published"
		}

		results := make([]bulkPublishResult, 0, len(a.ContentIDs))
		successCount := 0
		errorCount := 0

		for i, idStr := range a.ContentIDs {
			res, ok := publishOneBulkItem(ctx, deps, tenantID, i, idStr, status)
			results = append(results, res)

			if ok {
				successCount++
			} else {
				errorCount++
			}
		}

		return successToolResult(map[string]any{
			"succeeded": successCount,
			"failed":    errorCount,
			"total":     len(a.ContentIDs),
			"results":   results,
		}), nil
	}
}

func publishOneBulkItem(ctx context.Context, deps *ToolDeps, tenantID pgtype.UUID, idx int, idStr, status string) (bulkPublishResult, bool) {
	res := bulkPublishResult{Index: idx}

	contentID, err := scanUUID(idStr)
	if err != nil {
		res.ErrorCode = "invalid_content_id"
		res.Error = "content_id is not a valid UUID"

		return res, false
	}

	res.ContentID = uuidString(contentID)

	// Defense-in-depth privacy re-check.
	if deps.Privacy != nil {
		exempt, err := deps.Privacy.IsExempt(ctx, PrivacyEntityContent, tenantID, contentID)
		if err != nil {
			res.ErrorCode = "privacy_check_failed"
			res.Error = err.Error()

			return res, false
		}

		if exempt {
			res.ErrorCode = "privacy_blocked"
			res.Error = "content is marked as private and not available to the agent"

			return res, false
		}
	}

	row, err := deps.ContentLifecycle.UpdateContentStatus(ctx, contentID, tenantID, status)
	if err != nil {
		res.ErrorCode = "content_publish_failed"
		res.Error = err.Error()

		return res, false
	}

	res.Title = row.Title
	res.Slug = row.Slug
	res.Status = row.Status

	return res, true
}

// ContentBulkPublishPrivacyTarget extracts the content_ids from
// content.bulk_publish arguments for the executor's privacy gate. The privacy
// check is applied to every item in the array.
func ContentBulkPublishPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a bulkPublishArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing content.bulk_publish args: %w", err)
	}

	if len(a.ContentIDs) == 0 {
		return "", pgtype.UUID{}, nil
	}
	// The executor's privacy gate currently handles a single target. Target the
	// first content_id; the handler performs a per-item privacy re-check before
	// updating each row.
	id, err := scanUUID(a.ContentIDs[0])
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid content_id: %w", err)
	}

	return PrivacyEntityContent, id, nil
}
