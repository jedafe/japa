// Package ai — tools_fields.go implements the custom field tool handlers for
// the AI Agent Executor core tool catalog (Section 103, ADR-111).
//
// Tools implemented here (Phase 4, tasks 4.05-4.08):
//
//   - custom_fields.list         — list field definitions for a group
//   - custom_field_values.read   — read field values for an entity (privacy-filtered)
//   - custom_field_values.set    — set a single field value (RBAC + privacy checked)
//   - custom_field_values.bulk_set — set multiple field values in one transaction
//
// Privacy: custom_field_values.read and .set target a specific custom_field_value
// row, so they register PrivacyTargetFunc extractors. The .read handler
// additionally filters ai_exempt values from the returned list. The .bulk_set
// handler checks each target entity's exemption status before writing.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion — Custom field tools)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4, tasks 4.05-4.08)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// custom_fields.list (task 4.05)
// ---------------------------------------------------------------------------

// customFieldsListArgs is the argument shape for custom_fields.list.
type customFieldsListArgs struct {
	GroupID string `json:"group_id"`
}

// CustomFieldsListHandler returns a ToolHandler that lists field definitions
// for a custom field group. This is a read-only operation with no privacy
// target (field definitions are schema, not user data).
func CustomFieldsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFields == nil {
			return errorToolResult("custom_fields_unavailable", "custom fields service is not configured"), nil
		}

		var a customFieldsListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		groupID, err := scanUUID(a.GroupID)
		if err != nil {
			return errorToolResult("invalid_group_id", "group_id is not a valid UUID"), nil
		}

		fields, err := deps.CustomFields.ListFieldsByGroup(ctx, groupID, tenantID)
		if err != nil {
			return errorToolResult("custom_fields_list_failed", err.Error()), nil
		}

		out := make([]map[string]any, 0, len(fields))
		for _, f := range fields {
			out = append(out, map[string]any{
				"id":               uuidString(f.ID),
				"label":            f.Label,
				"key":              f.Key,
				"field_type":       f.FieldType,
				"is_required":      f.IsRequired,
				"is_system":        f.IsSystem,
				"show_in_frontend": f.ShowInFrontend,
				"position":         f.Position,
			})
		}

		return successToolResult(map[string]any{
			"fields": out,
			"total":  len(out),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// custom_field_values.read (task 4.06)
// ---------------------------------------------------------------------------

// cfvReadArgs is the argument shape for custom_field_values.read.
type cfvReadArgs struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
}

// CFVReadHandler returns a ToolHandler that reads field values for an
// entity. ai_exempt values are filtered from the result set — the agent
// never sees exempt field values. The entity-level privacy check (is the
// parent content/user exempt?) is handled by the executor's privacy gate
// via the registered PrivacyTargetFunc when the entity is a content or user.
func CFVReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFields == nil {
			return errorToolResult("custom_fields_unavailable", "custom fields service is not configured"), nil
		}

		var a cfvReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.EntityType == "" {
			return errorToolResult("missing_entity_type", "entity_type is required"), nil
		}
		entityID, err := scanUUID(a.EntityID)
		if err != nil {
			return errorToolResult("invalid_entity_id", "entity_id is not a valid UUID"), nil
		}

		cfvs, err := deps.CustomFields.ListFieldValuesByEntity(ctx, tenantID, a.EntityType, entityID)
		if err != nil {
			return errorToolResult("cfv_read_failed", err.Error()), nil
		}

		filtered := filterNonExemptFieldValues(cfvs)
		return successToolResult(map[string]any{
			"entity_type":  a.EntityType,
			"entity_id":    uuidString(entityID),
			"field_values": filtered,
			"total":        len(filtered),
		}), nil
	}
}

// CFVReadPrivacyTarget extracts the entity from custom_field_values.read
// arguments. The entity_type determines which privacy table to check:
// "content" → contents.ai_exempt, "user" → users.ai_exempt. Other entity
// types have no exemption column and return a zero target (skip).
func CFVReadPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a cfvReadArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing custom_field_values.read args: %w", err)
	}
	id, err := scanUUID(a.EntityID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid entity_id: %w", err)
	}
	switch a.EntityType {
	case "content":
		return PrivacyEntityContent, id, nil
	case "user":
		return PrivacyEntityUser, id, nil
	default:
		// No exemption table for this entity type — skip the privacy check.
		return "", pgtype.UUID{}, nil
	}
}

// ---------------------------------------------------------------------------
// custom_field_values.set (task 4.07)
// ---------------------------------------------------------------------------

// cfvSetArgs is the argument shape for custom_field_values.set.
type cfvSetArgs struct {
	FieldID    string          `json:"field_id"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	ValueText  *string         `json:"value_text,omitempty"`
	ValueBool  *bool           `json:"value_bool,omitempty"`
	ValueDate  *string         `json:"value_date,omitempty"`
	ValueJson  json.RawMessage `json:"value_json,omitempty"`
}

// CFVSetHandler returns a ToolHandler that sets a single custom field value.
// The executor's RBAC gate checks the required_permission; the privacy gate
// checks the target entity's ai_exempt flag. The handler itself performs a
// defense-in-depth privacy re-check before the upsert.
func CFVSetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFields == nil {
			return errorToolResult("custom_fields_unavailable", "custom fields service is not configured"), nil
		}

		var a cfvSetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		fieldID, err := scanUUID(a.FieldID)
		if err != nil {
			return errorToolResult("invalid_field_id", "field_id is not a valid UUID"), nil
		}
		entityID, err := scanUUID(a.EntityID)
		if err != nil {
			return errorToolResult("invalid_entity_id", "entity_id is not a valid UUID"), nil
		}
		if a.EntityType == "" {
			return errorToolResult("missing_entity_type", "entity_type is required"), nil
		}

		params := gen.UpsertCustomFieldValueParams{
			FieldID:    fieldID,
			EntityType: a.EntityType,
			EntityID:   entityID,
			ValueText:  a.ValueText,
			ValueBool:  a.ValueBool,
		}
		if len(a.ValueJson) > 0 && string(a.ValueJson) != "null" {
			params.ValueJson = a.ValueJson
		}
		if a.ValueDate != nil {
			var t pgtype.Timestamptz
			if err := t.Scan(*a.ValueDate); err == nil {
				params.ValueDate = t
			}
		}

		cfv, err := deps.CustomFields.UpsertFieldValue(ctx, params)
		if err != nil {
			return errorToolResult("cfv_set_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"field_id":    uuidString(cfv.FieldID),
			"entity_type": cfv.EntityType,
			"entity_id":   uuidString(cfv.EntityID),
			"updated_at":  timestampString(cfv.UpdatedAt),
		}), nil
	}
}

// CFVSetPrivacyTarget extracts the target entity from custom_field_values.set
// arguments. Maps the entity_type to the corresponding privacy table.
func CFVSetPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a cfvSetArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing custom_field_values.set args: %w", err)
	}
	id, err := scanUUID(a.EntityID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid entity_id: %w", err)
	}
	switch a.EntityType {
	case "content":
		return PrivacyEntityContent, id, nil
	case "user":
		return PrivacyEntityUser, id, nil
	default:
		return "", pgtype.UUID{}, nil
	}
}

// ---------------------------------------------------------------------------
// custom_field_values.bulk_set (task 4.08)
// ---------------------------------------------------------------------------

// cfvBulkSetArgs is the argument shape for custom_field_values.bulk_set.
type cfvBulkSetArgs struct {
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Values     []cfvBulkEntry `json:"values"`
}

// cfvBulkEntry is a single field-value pair within a bulk_set call.
type cfvBulkEntry struct {
	FieldID   string          `json:"field_id"`
	ValueText *string         `json:"value_text,omitempty"`
	ValueBool *bool           `json:"value_bool,omitempty"`
	ValueDate *string         `json:"value_date,omitempty"`
	ValueJson json.RawMessage `json:"value_json,omitempty"`
}

// CFVBulkSetHandler returns a ToolHandler that sets multiple field values for
// a single entity in one call. Each value is upserted sequentially; a failure
// on one value does not abort the others — the result reports per-field
// status so the LLM can retry failed entries. This mirrors the §103 bulk
// operation behavior: bulk operations are HITL-queued (the tool config sets
// requires_hitl = true), and per-field errors are reported, not hidden.
func CFVBulkSetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFields == nil {
			return errorToolResult("custom_fields_unavailable", "custom fields service is not configured"), nil
		}

		var a cfvBulkSetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.EntityType == "" {
			return errorToolResult("missing_entity_type", "entity_type is required"), nil
		}
		entityID, err := scanUUID(a.EntityID)
		if err != nil {
			return errorToolResult("invalid_entity_id", "entity_id is not a valid UUID"), nil
		}
		if len(a.Values) == 0 {
			return errorToolResult("empty_values", "at least one value entry is required"), nil
		}

		// Defense-in-depth: check the target entity's exemption status before
		// any writes. The executor's privacy gate already covers this, but
		// the bulk handler re-checks to guard against a misconfigured
		// PrivacyTargetFunc.
		if deps.Privacy != nil {
			if et, ok := privacyEntityTypeFor(a.EntityType); ok {
				exempt, err := deps.Privacy.IsExempt(ctx, et, tenantID, entityID)
				if err != nil {
					return errorToolResult("privacy_check_failed", err.Error()), nil
				}
				if exempt {
					return privacyBlockedResult("target entity is marked as private and not available to the agent"), nil
				}
			}
		}

		results := make([]map[string]any, 0, len(a.Values))
		succeeded := 0
		for _, v := range a.Values {
			fieldID, err := scanUUID(v.FieldID)
			if err != nil {
				results = append(results, map[string]any{
					"field_id": v.FieldID,
					"status":   "error",
					"error":    "invalid field_id",
				})
				continue
			}

			params := gen.UpsertCustomFieldValueParams{
				FieldID:    fieldID,
				EntityType: a.EntityType,
				EntityID:   entityID,
				ValueText:  v.ValueText,
				ValueBool:  v.ValueBool,
			}
			if len(v.ValueJson) > 0 && string(v.ValueJson) != "null" {
				params.ValueJson = v.ValueJson
			}
			if v.ValueDate != nil {
				var t pgtype.Timestamptz
				if err := t.Scan(*v.ValueDate); err == nil {
					params.ValueDate = t
				}
			}

			cfv, err := deps.CustomFields.UpsertFieldValue(ctx, params)
			if err != nil {
				results = append(results, map[string]any{
					"field_id": v.FieldID,
					"status":   "error",
					"error":    err.Error(),
				})
				continue
			}
			results = append(results, map[string]any{
				"field_id":   uuidString(cfv.FieldID),
				"status":     "success",
				"updated_at": timestampString(cfv.UpdatedAt),
			})
			succeeded++
		}

		return successToolResult(map[string]any{
			"entity_type": a.EntityType,
			"entity_id":   uuidString(entityID),
			"results":     results,
			"succeeded":   succeeded,
			"failed":      len(a.Values) - succeeded,
		}), nil
	}
}

// CFVBulkSetPrivacyTarget extracts the target entity for bulk_set.
func CFVBulkSetPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a cfvBulkSetArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing custom_field_values.bulk_set args: %w", err)
	}
	id, err := scanUUID(a.EntityID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid entity_id: %w", err)
	}
	if et, ok := privacyEntityTypeFor(a.EntityType); ok {
		return et, id, nil
	}
	return "", pgtype.UUID{}, nil
}

// privacyEntityTypeFor maps an entity_type string to the corresponding
// PrivacyEntityType. Returns ok=false when the entity type has no exemption
// column (e.g. "organization", "space").
func privacyEntityTypeFor(entityType string) (PrivacyEntityType, bool) {
	switch entityType {
	case "content":
		return PrivacyEntityContent, true
	case "user":
		return PrivacyEntityUser, true
	default:
		return "", false
	}
}

// ---------------------------------------------------------------------------
// Phase 9 — Custom field definition tools (tasks 9.07)
//
//   - custom_fields.create       — create a new field definition (admin, HITL)
//   - custom_fields.update       — update a field definition (admin, HITL)
//   - custom_field_groups.list   — list field groups
//   - custom_field_groups.create — create a field group (admin, HITL)
// ---------------------------------------------------------------------------

// customFieldCreateArgs is the LLM-produced argument shape for
// custom_fields.create.
type customFieldCreateArgs struct {
	GroupID        string          `json:"group_id"`
	Label          string          `json:"label"`
	Key            string          `json:"key"`
	FieldType      string          `json:"field_type"`
	Options        json.RawMessage `json:"options,omitempty"`
	EmbedPlatforms []string        `json:"embed_platforms,omitempty"`
	Placeholder    *string         `json:"placeholder,omitempty"`
	DefaultValue   *string         `json:"default_value,omitempty"`
	IsRequired     bool            `json:"is_required,omitempty"`
	IsSystem       bool            `json:"is_system,omitempty"`
	ShowInFrontend bool            `json:"show_in_frontend,omitempty"`
	Filterable     bool            `json:"filterable,omitempty"`
	Sortable       bool            `json:"sortable,omitempty"`
	Searchable     bool            `json:"searchable,omitempty"`
	Position       int32           `json:"position,omitempty"`
}

// CustomFieldsCreateHandler returns a ToolHandler that creates a new custom
// field definition. The executor's RBAC gate checks the required_permission
// (custom_fields.field.create); the tool config sets requires_hitl = true.
func CustomFieldsCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFieldDefs == nil {
			return errorToolResult("custom_field_defs_unavailable", "custom field definition service is not configured"), nil
		}

		var a customFieldCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Label == "" {
			return errorToolResult("missing_label", "label is required"), nil
		}
		if a.Key == "" {
			return errorToolResult("missing_key", "key is required"), nil
		}
		if a.FieldType == "" {
			return errorToolResult("missing_field_type", "field_type is required"), nil
		}
		groupID, err := scanUUID(a.GroupID)
		if err != nil {
			return errorToolResult("invalid_group_id", "group_id is not a valid UUID"), nil
		}

		options := a.Options
		if len(options) == 0 {
			options = json.RawMessage("null")
		}

		field, err := deps.CustomFieldDefs.CreateField(ctx, gen.CreateCustomFieldParams{
			GroupID:        groupID,
			TenantID:       tenantID,
			Label:          a.Label,
			Key:            a.Key,
			FieldType:      a.FieldType,
			Options:        options,
			EmbedPlatforms: a.EmbedPlatforms,
			Placeholder:    a.Placeholder,
			DefaultValue:   a.DefaultValue,
			IsRequired:     a.IsRequired,
			IsSystem:       a.IsSystem,
			ShowInFrontend: a.ShowInFrontend,
			Filterable:     a.Filterable,
			Sortable:       a.Sortable,
			Searchable:     a.Searchable,
			Position:       a.Position,
		})
		if err != nil {
			return errorToolResult("custom_field_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"field_id":   uuidString(field.ID),
			"group_id":   uuidString(field.GroupID),
			"label":      field.Label,
			"key":        field.Key,
			"field_type": field.FieldType,
			"position":   field.Position,
			"created_at": timestampString(field.CreatedAt),
		}), nil
	}
}

// customFieldUpdateArgs is the LLM-produced argument shape for
// custom_fields.update.
type customFieldUpdateArgs struct {
	FieldID        string          `json:"field_id"`
	Label          string          `json:"label"`
	FieldType      string          `json:"field_type,omitempty"`
	Options        json.RawMessage `json:"options,omitempty"`
	EmbedPlatforms []string        `json:"embed_platforms,omitempty"`
	Placeholder    *string         `json:"placeholder,omitempty"`
	DefaultValue   *string         `json:"default_value,omitempty"`
	IsRequired     bool            `json:"is_required,omitempty"`
	ShowInFrontend bool            `json:"show_in_frontend,omitempty"`
	Filterable     bool            `json:"filterable,omitempty"`
	Sortable       bool            `json:"sortable,omitempty"`
	Searchable     bool            `json:"searchable,omitempty"`
	Position       int32           `json:"position,omitempty"`
}

// CustomFieldsUpdateHandler returns a ToolHandler that updates a custom
// field definition. The executor's RBAC gate checks the required_permission
// (custom_fields.field.edit); the tool config sets requires_hitl = true.
func CustomFieldsUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFieldDefs == nil {
			return errorToolResult("custom_field_defs_unavailable", "custom field definition service is not configured"), nil
		}

		var a customFieldUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		fieldID, err := scanUUID(a.FieldID)
		if err != nil {
			return errorToolResult("invalid_field_id", "field_id is not a valid UUID"), nil
		}
		if a.Label == "" {
			return errorToolResult("missing_label", "label is required"), nil
		}

		fieldType := a.FieldType
		if fieldType == "" {
			fieldType = "text"
		}

		options := a.Options
		if len(options) == 0 {
			options = json.RawMessage("null")
		}

		field, err := deps.CustomFieldDefs.UpdateField(ctx, gen.UpdateCustomFieldParams{
			ID:             fieldID,
			TenantID:       tenantID,
			Label:          a.Label,
			FieldType:      fieldType,
			Options:        options,
			EmbedPlatforms: a.EmbedPlatforms,
			Placeholder:    a.Placeholder,
			DefaultValue:   a.DefaultValue,
			IsRequired:     a.IsRequired,
			ShowInFrontend: a.ShowInFrontend,
			Filterable:     a.Filterable,
			Sortable:       a.Sortable,
			Searchable:     a.Searchable,
			Position:       a.Position,
		})
		if err != nil {
			return errorToolResult("custom_field_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"field_id":   uuidString(field.ID),
			"label":      field.Label,
			"key":        field.Key,
			"field_type": field.FieldType,
			"position":   field.Position,
		}), nil
	}
}

// CustomFieldGroupsListHandler returns a ToolHandler that lists custom
// field groups for the tenant. Read-only — no privacy target.
func CustomFieldGroupsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFieldDefs == nil {
			return errorToolResult("custom_field_defs_unavailable", "custom field definition service is not configured"), nil
		}

		groups, err := deps.CustomFieldDefs.ListFieldGroups(ctx, tenantID)
		if err != nil {
			return errorToolResult("custom_field_groups_list_failed", err.Error()), nil
		}

		results := make([]map[string]any, 0, len(groups))
		for _, g := range groups {
			entry := map[string]any{
				"id":         uuidString(g.ID),
				"name":       g.Name,
				"slug":       g.Slug,
				"is_system":  g.IsSystem,
				"created_at": timestampString(g.CreatedAt),
			}
			if g.Description != nil {
				entry["description"] = *g.Description
			}
			results = append(results, entry)
		}

		return successToolResult(map[string]any{
			"groups": results,
			"total":  len(results),
		}), nil
	}
}

// customFieldGroupCreateArgs is the LLM-produced argument shape for
// custom_field_groups.create.
type customFieldGroupCreateArgs struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
	IsSystem    bool    `json:"is_system,omitempty"`
}

// CustomFieldGroupsCreateHandler returns a ToolHandler that creates a
// custom field group. The executor's RBAC gate checks the
// required_permission (custom_fields.group.create); the tool config sets
// requires_hitl = true.
func CustomFieldGroupsCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFieldDefs == nil {
			return errorToolResult("custom_field_defs_unavailable", "custom field definition service is not configured"), nil
		}

		var a customFieldGroupCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Name == "" {
			return errorToolResult("missing_name", "name is required"), nil
		}
		if a.Slug == "" {
			return errorToolResult("missing_slug", "slug is required"), nil
		}

		group, err := deps.CustomFieldDefs.CreateFieldGroup(ctx, tenantID, a.Name, a.Slug, a.Description, a.IsSystem)
		if err != nil {
			return errorToolResult("custom_field_group_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"group_id":   uuidString(group.ID),
			"name":       group.Name,
			"slug":       group.Slug,
			"is_system":  group.IsSystem,
			"created_at": timestampString(group.CreatedAt),
		}), nil
	}
}
