// Package wiki — service_fields.go implements the §80 community field ops
// surface: entity-targeted structured-edit submissions (ops arrays) riding
// the existing wiki_revisions pending→approved queue, tiered auto-approve
// for wiki.fields.manage holders, the editable field schema read behind the
// shared <SuggestEdit> component, and the approve path that applies ops
// transactionally before propagating JCE cache busts.
// // Ref: docs/superpowers/specs/2026-09-24-wiki-community-fields-design.md
// //	japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/valkey"
)

// TaskFieldsPropagate is the Asynq task type enqueued after a fields/mixed
// revision is approved — the worker busts JCE comparison caches for the
// revision's target entity (spec §6 step 4; registered in
// .jeos/conventions/asynq-tasks.md).
const TaskFieldsPropagate = "wiki:fields_propagate"

// TaskSeedStubPages is the Asynq task type enqueued once per country space
// when wiki.enabled is activated — the worker seeds the 7 default stub
// pages idempotently (§80; registered in .jeos/conventions/asynq-tasks.md).
const TaskSeedStubPages = "wiki:seed_stub_pages"

// RBAC node names for the field-ops permission tiers (spec §10; seeded by
// migration 000731_wiki_community_fields).
const (
	PermFieldsEdit    = "wiki.fields.edit"    // set_value ops
	PermFieldsPropose = "wiki.fields.propose" // propose_field / propose_group ops
	PermFieldsManage  = "wiki.fields.manage"  // upsert_field / assign_group ops + auto-approve
)

// ErrOpNotPermitted is returned when a submission contains an op the
// submitter's RBAC grants do not cover, or a set_value targets a field whose
// custom_field_permissions deny can_edit to the submitter's user group (§6).
// Handlers map this to a 403 forbidden API error.
var ErrOpNotPermitted = errors.New("wiki: field op not permitted")

// ErrEntityNotResolvable is returned by SuggestFields when the item_ref
// resolver registry cannot resolve the submitted (entity_type, entity_id) —
// the entity does not exist or the type has no registered resolver.
var ErrEntityNotResolvable = errors.New("wiki: entity not resolvable")

// ErrFieldSchemaUnavailable marks a schema-load failure on an entity-
// targeted submission (CF21): the assignment/field reads errored, so the
// entity's field map cannot be built. Handlers map this to 400
// schema_load_failed with an explicit retry hint — the submitter must either
// retry (transient read) or resubmit carrying explicit schema context
// (propose the field in the same ops array). The flag lives on the ops
// payload (accept-no-schema): set_value ops carry target entity fields, and
// proposal ops carry their own definitions, so the review queue only
// accepts schema-free array shapes it can validate without a DB read.
var ErrFieldSchemaUnavailable = errors.New("wiki: field schema unavailable")

// guestGroupSlug is the user_groups.slug of the anonymous/guest group
// (§03 system group). Anonymous readers have no user_group_memberships row,
// so the wiki schema resolves their can_view grants by group slug instead.
const guestGroupSlug = "guest"

// fieldReadStore is the narrow read-side interface the wiki service needs
// from customfields.Service: submit-time field lookups + batched per-field
// RBAC, and the group/assignment/value reads behind GetEntityFields. Writes
// during ops application go through gen.New(tx) inside applyOps — the
// pool-bound service cannot join the caller's transaction, so no write
// methods belong here.
type fieldReadStore interface {
	ListVisibleFieldIDsForUser(ctx context.Context, userID pgtype.UUID) ([]pgtype.UUID, error)
	ListVisibleFieldIDsForGroupSlug(ctx context.Context, groupSlug string) ([]pgtype.UUID, error)
	ListEditableFieldIDsForUser(ctx context.Context, userID pgtype.UUID) ([]pgtype.UUID, error)
	GetCustomFieldByKey(ctx context.Context, key string) (gen.GetCustomFieldByKeyRow, error)
	ListAssignmentsByEntityClass(ctx context.Context, entityClass string) ([]gen.CustomFieldGroupAssignment, error)
	ListFieldValuesWithDefsByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) ([]gen.ListCustomFieldValuesWithDefsByEntityRow, error)
	ListFieldValuesWithDefsForEntities(ctx context.Context, entityType string, entityIDs []pgtype.UUID) ([]gen.ListCustomFieldValuesWithDefsForEntitiesRow, error)
	GetFieldGroup(ctx context.Context, id pgtype.UUID) (gen.CustomFieldGroup, error)
	GetFieldGroupBySlug(ctx context.Context, slug string) (gen.CustomFieldGroup, error)
	ListFieldsByGroup(ctx context.Context, groupID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error)
}

// fieldGroupProvisioner is the write-side interface used to auto-provision
// a category's dedicated custom_field_groups row + entity-class assignment
// on category create (§80 — group slug wiki_{type}_{category}). Category
// admin writes run outside the ops-apply transaction, so a pool-bound
// service is correct here. Satisfied by *customfields.Service; picked up
// via type assertion in SetCustomFields.
type fieldGroupProvisioner interface {
	GetFieldGroupBySlug(ctx context.Context, slug string) (gen.CustomFieldGroup, error)
	CreateFieldGroup(ctx context.Context, name, slug string, description *string, isSystem bool) (gen.CustomFieldGroup, error)
	CreateAssignment(ctx context.Context, params gen.CreateAssignmentParams) (gen.CustomFieldGroupAssignment, error)
}

// richTextSanitizer scrubs rich_text HTML before it is coerced for storage
// (CF2 — submit + apply paths share it via valueToUpsertParams and
// checkSetValueFields). Injected from server.go to avoid an import cycle
// (wiki already imports content; this is the same content.SanitizeHTML the
// prose path uses). Nil disables scrubbing (test-only).
// SetRichTextSanitizer wires the HTML sanitizer used to scrub rich_text
// field values on the wiki ops path (CF2 — submit-time validation and
// apply-time coercion). Production wires content.SanitizeHTML from
// server.go; tests may omit it.
var wikiRichTextSanitizer func(string) string

// SetRichTextSanitizer wires the HTML sanitizer used to scrub rich_text
// field values on the wiki ops path (CF2 — submit-time validation and
// apply-time coercion). Production wires content.SanitizeHTML from
// server.go; tests may omit it.
func SetRichTextSanitizer(sanitize func(string) string) {
	wikiRichTextSanitizer = sanitize
}

// sanitizeRichTextValue scrubs one rich_text string leaf when a sanitizer is
// wired; passthrough otherwise so tests without wiring keep exact bytes.
func sanitizeRichTextValue(s string) string {
	if wikiRichTextSanitizer == nil {
		return s
	}
	return wikiRichTextSanitizer(s)
}

// entityResolver resolves a JCE item_ref ({type, id}) into a normalized item.
// Satisfied by *resolver.Registry — the same registry comparison.Service
// receives via SetResolver.
type entityResolver interface {
	Resolve(ctx context.Context, ref map[string]any) (resolver.ResolvedItem, error)
}

// jceCacheBuster invalidates every JCE comparison cache that references an
// entity — implemented by comparison.Service.BustCachesForItemRef (Task 7:
// finds comparison_items rows via the item_ref GIN index and deletes the
// jc:jce:* cache keys for each).
type jceCacheBuster interface {
	BustCachesForItemRef(ctx context.Context, entityType, entityID string) error
}

// TaskEnqueuer enqueues Asynq tasks fire-and-forget (Rule 8). Satisfied by
// *worker.Enqueuer — the same shape other feature services consume.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// superAdminChecker is the optional capability the injected permission
// resolver exposes for super_admin-group detection — *rbac.RBAC satisfies
// it. Group membership is a stricter gate than the '*' permission
// wildcard: delete_field ops and the bypass suite require actual
// super_admin membership, not a hand-granted wildcard (§80 Super-Admin
// Direct Publish).
type superAdminChecker interface {
	IsSuperAdmin(ctx context.Context, userID pgtype.UUID) bool
}

// isSuperAdmin reports whether the user belongs to the super_admin group.
// Fails closed when the checker lacks the capability (tests may inject
// HasPermission-only fakes).
func (s *Service) isSuperAdmin(ctx context.Context, userID pgtype.UUID) bool {
	if !userID.Valid {
		return false
	}
	if c, ok := s.perms.(superAdminChecker); ok {
		return c.IsSuperAdmin(ctx, userID)
	}
	return false
}

// Compile-time checks that the production collaborators satisfy the
// injection interfaces.
var (
	_ fieldReadStore        = (*customfields.Service)(nil)
	_ fieldGroupProvisioner = (*customfields.Service)(nil)
	_ entityResolver        = (*resolver.Registry)(nil)
)

// ---------------------------------------------------------------------------
// DTOs (spec §7)
// ---------------------------------------------------------------------------

// EntityFieldDTO is one editable field in an entity's field schema: the
// definition plus the entity's current value and the requesting user's
// field-level permissions.
type EntityFieldDTO struct {
	ID        string          `json:"id"`
	Key       string          `json:"key"`
	Label     string          `json:"label"`
	FieldType string          `json:"field_type"`
	Options   json.RawMessage `json:"options,omitempty"`
	Value     json.RawMessage `json:"value"`
	CanEdit   bool            `json:"can_edit"`
	Visible   bool            `json:"visible"`
	// Section is the §80 itemized-pricing sub-grouping token — fields
	// sharing one render under one item table (e.g. 'transportation').
	Section string `json:"section,omitempty"`
	// Computed marks a field whose Value is derived from options.computed
	// sources — never manually editable (set_value ops are rejected).
	Computed bool `json:"computed,omitempty"`
	// System marks a platform-managed field — never deletable, even by
	// super_admin (delete_field ops and the DeleteCustomField query both
	// refuse it). Surfaced so the suggest-edit UI never offers the mark.
	System bool `json:"system,omitempty"`
}

// EntityFieldSectionDTO is one named section inside a field group — fields
// sharing a custom_fields.section token in definition order. The empty key
// collects section-less fields (rendered first, unlabeled). §80 Wiki Topic
// Pages: a section synthesized from a child page template's bound group
// carries PageSlug (the /wiki/{space}/{slug} drill-down target) and Label
// (the child's section_label fallback label); plain token sections carry
// neither.
type EntityFieldSectionDTO struct {
	Key      string           `json:"key"`
	Label    string           `json:"label,omitempty"`
	PageSlug string           `json:"page_slug,omitempty"`
	Fields   []EntityFieldDTO `json:"fields"`
}

// EntityFieldGroupDTO is a field group assigned to the entity's class, with
// its editable fields in display order.
type EntityFieldGroupDTO struct {
	ID     string           `json:"id"`
	Slug   string           `json:"slug"`
	Name   string           `json:"name"`
	Fields []EntityFieldDTO `json:"fields"`
	// Sections regroups Fields by their section token (§80 Itemized
	// Pricing) — same field objects in stable order, for UIs that render
	// one table per section.
	Sections []EntityFieldSectionDTO `json:"sections,omitempty"`
}

// EntityFieldsDTO is the response shape of GET /wiki/entities/:type/:id/fields
// (spec §7): the field groups assigned to the entity's class, current values,
// per-field permissions for the requesting user, and the count of pending
// revisions targeting the entity. can_propose/can_manage are the caller's
// op-tier capabilities so the UI can gate "suggest a new field" affordances
// (§10: wiki.fields.propose / wiki.fields.manage).
type EntityFieldsDTO struct {
	Groups       []EntityFieldGroupDTO `json:"groups"`
	PendingCount int64                 `json:"pending_count"`
	CanPropose   bool                  `json:"can_propose"`
	CanManage    bool                  `json:"can_manage"`
	// CanDelete reports super_admin-group membership — the only tier that
	// may submit delete_field ops (§80 Super-Admin Direct Publish). The
	// SuggestEdit dialog renders per-field delete affordances on this flag.
	CanDelete   bool   `json:"can_delete"`
	EntityClass string `json:"entity_class,omitempty"`
	// EntityCurrencyCode is the entity's resolved country currency
	// (§58 "Country → currency linkage" via its geo space) — the display
	// fallback for currency/price_range fields that don't declare
	// {currency_code} in options.
	EntityCurrencyCode string `json:"entity_currency_code,omitempty"`
	// EntityCurrencyFx carries the local→fx-base display rate for "(base)"
	// equivalents on money fields (§58 + §112); nil when unavailable.
	EntityCurrencyFx *CurrencyFxDTO `json:"entity_currency_fx,omitempty"`
}

// ---------------------------------------------------------------------------
// Submit / suggest
// ---------------------------------------------------------------------------

// SubmitFieldOps validates and stores a structured-edit revision for the
// given entity (§6 submit step). pageID may be an invalid pgtype.UUID —
// page-less ops revisions record NULL wiki_page_id. body carries optional
// TipTap prose: when non-empty the revision is kind='mixed' — one revision
// carrying prose and ops together, approved atomically (prose goes live via
// GetApprovedRevision's kind IN ('content','mixed') filter and ops apply in
// the same transaction). When canManage is true (the caller verified the
// submitter holds wiki.fields.manage) the revision is approved and its ops
// applied immediately in one transaction; the approved row is kept for
// audit. Returns the revision ID and whether it was auto-approved.
func (s *Service) SubmitFieldOps(ctx context.Context, userID, pageID pgtype.UUID, entityType string, entityID pgtype.UUID, body []byte, ops []FieldOp, summary string, canManage bool) (revID string, autoApproved bool, err error) {
	normBody, targetType, targetID, err := s.validateFieldOps(ctx, userID, entityType, entityID, body, ops, canManage)
	if err != nil {
		return "", false, err
	}

	return s.persistFieldOps(ctx, userID, pageID, normBody, targetType, targetID, ops, summary, canManage)
}

// SubmitPageFieldOps is the page-slug-keyed ops entry behind
// POST /wiki/spaces/{space}/pages/{page}/revisions (§80): ops validate
// BEFORE the target resolves so a rejected submission never materializes a
// lazy template page into an empty wiki_pages row. The revision target is
// derived from the ops themselves — the same way the handler invoked
// SubmitFieldOps with an empty entity target.
func (s *Service) SubmitPageFieldOps(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string, body []byte, ops []FieldOp, summary string, canManage bool) (string, bool, error) {
	normBody, targetType, targetID, err := s.validateFieldOps(ctx, userID, "", pgtype.UUID{}, body, ops, canManage)
	if err != nil {
		return "", false, err
	}

	pageID, _, locked, err := s.GetPageEntityTarget(ctx, userID, spaceSlug, pageSlug)
	if err != nil {
		return "", false, err
	}
	// §80 Super-Admin Direct Publish — super_admin submissions ignore
	// is_locked on the ops path exactly like the prose path does.
	if locked && !s.isSuperAdmin(ctx, userID) {
		return "", false, ErrPageLocked
	}

	return s.persistFieldOps(ctx, userID, pageID, normBody, targetType, targetID, ops, summary, canManage)
}

// validateFieldOps runs every pageID-independent check on a field-ops
// submission: op shape/count, prose normalization (BUG-020), target
// resolution, per-op RBAC, entity schema lookup, set_value semantics, and
// schema-op target checks. Returns the normalized body and the resolved
// revision target for persistFieldOps.
func (s *Service) validateFieldOps(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, body []byte, ops []FieldOp, canManage bool) (normBody []byte, targetType string, targetID pgtype.UUID, err error) {
	if len(ops) == 0 {
		return nil, "", pgtype.UUID{}, errors.New("wiki: submit fields: at least one op is required")
	}

	if err := ValidateOps(ops, s.maxOpsPerRevision(ctx)); err != nil {
		return nil, "", pgtype.UUID{}, fmt.Errorf("wiki: submit fields: %w", err)
	}

	// Mixed revisions carry prose — run it through the same
	// sanitize/normalize pass as the content-only path (BUG-020).
	if len(body) > 0 {
		normalized, nerr := normalizeWikiContent(body)
		if nerr != nil {
			return nil, "", pgtype.UUID{}, fmt.Errorf("wiki: submit fields: invalid content: %w", nerr)
		}

		body = normalized
	}

	targetType, targetID, err = resolveOpsTarget(entityType, entityID, ops)
	if err != nil {
		return nil, "", pgtype.UUID{}, fmt.Errorf("wiki: submit fields: %w", err)
	}

	// Per-op RBAC gate (§10): each op maps to a wiki.fields.* node and a
	// submission is rejected when any op exceeds the submitter's grants.
	if err := s.checkOpsPermissions(ctx, userID, ops, canManage); err != nil {
		return nil, "", pgtype.UUID{}, err
	}

	// The entity's assigned-group field schema — set_value resolves
	// field_key inside it (keys are unique per group, never globally). Nil
	// for target-less submissions; ops that need a schema field then fail.
	schema, err := s.entitySchema(ctx, targetType, targetID)
	if err != nil {
		return nil, "", pgtype.UUID{}, err
	}

	// set_value semantic validation: field exists in the entity schema (or
	// is proposed earlier in the same array), the value coerces to the
	// field's value column and belongs to its options, and field-level
	// can_edit allows the submitter.
	if err := s.checkSetValueFields(ctx, userID, ops, canManage, schema); err != nil {
		return nil, "", pgtype.UUID{}, err
	}

	// Schema-op targets: groups must exist (or be proposed earlier in the
	// same array), field keys/group slugs must be free — a revision that
	// can never apply must not queue as an unapprovable pending row.
	if err := s.checkSchemaOpTargets(ctx, ops); err != nil {
		return nil, "", pgtype.UUID{}, err
	}

	return body, targetType, targetID, nil
}

// persistFieldOps is the insert half of SubmitFieldOps/SubmitPageFieldOps —
// edit lock (B6, page-scoped), ops marshal, revision insert, and the
// canManage auto-approve (§6 step 2). Validation must already have run via
// validateFieldOps; body is the normalized form it returns.
func (s *Service) persistFieldOps(ctx context.Context, userID, pageID pgtype.UUID, body []byte, targetType string, targetID pgtype.UUID, ops []FieldOp, summary string, canManage bool) (revID string, autoApproved bool, err error) {
	// B6 edit lock — reused only when a host page exists. Page-less suggest
	// submissions take no lock: two pending revisions may target the same
	// entity+field and both can be approved (last-approver wins on the value —
	// standard wiki semantics; content_hash dedupes identical submissions).
	if pageID.Valid && s.vk != nil {
		lockKey := valkey.MustKey("wiki_edit_lock::%s:%s", pageID.String(), userID.String())

		acquired, lerr := s.vk.SetNX(ctx, lockKey, "1", 5*time.Minute).Result()
		switch {
		case lerr != nil:
			// Fail open — degrade gracefully if Valkey is unreachable.
			slog.Warn("wiki: field-ops edit lock acquire failed, proceeding", "error", lerr)
		case !acquired:
			return "", false, ErrEditInProgress
		default:
			defer func() {
				if err := s.vk.Del(ctx, lockKey).Err(); err != nil {
					slog.Warn("wiki: field-ops edit lock release failed", "error", err)
				}
			}()
		}
	}

	opsJSON, err := json.Marshal(ops)
	if err != nil {
		return "", false, fmt.Errorf("wiki: submit fields: marshal ops: %w", err)
	}

	var targetPtr *string
	if targetType != "" {
		targetPtr = &targetType
	}

	revUUID, err := s.store.SubmitRevision(ctx, pageID, userID, DeriveKind(len(body) > 0, ops), body, opsJSON, summary, targetPtr, targetID)
	if err != nil {
		if errors.Is(err, ErrDuplicateContent) {
			return "", false, ErrDuplicateContent
		}

		return "", false, fmt.Errorf("wiki: submit fields: %w", err)
	}

	if !canManage {
		return revUUID.String(), false, nil
	}

	// Manage tier (§6 step 2): approve + apply immediately — the same
	// transaction the reviewer path uses. The row stays in the queue as an
	// approved audit record. On failure the revision remains pending and the
	// error is returned so the caller can surface it.
	if err := s.approveOpsRevision(ctx, revUUID, userID, "auto-approved (wiki.fields.manage)", ops); err != nil {
		return revUUID.String(), false, fmt.Errorf("wiki: submit fields: auto-approve: %w", err)
	}

	return revUUID.String(), true, nil
}

// SuggestFields is the entity-keyed entry point behind POST /wiki/suggest
// (spec §7): it resolves the target entity through the injected JCE resolver
// registry (existence check), finds the entity's host wiki page best-effort
// via the resolved item's geo_space_id metadata, then delegates to
// SubmitFieldOps. Entities with no wiki page produce page-less revisions.
func (s *Service) SuggestFields(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, ops []FieldOp, summary string, canManage bool) (string, bool, error) {
	if entityType == "" || !entityID.Valid {
		return "", false, errors.New("wiki: suggest: entity_type and a valid entity_id are required")
	}

	if s.entityResolver == nil {
		return "", false, errors.New("wiki: suggest: entity resolver not configured")
	}

	item, err := s.entityResolver.Resolve(ctx, map[string]any{
		"type": entityType,
		"id":   entityID.String(),
	})
	if err != nil {
		return "", false, fmt.Errorf("%w: %s %s: %w", ErrEntityNotResolvable, entityType, entityID.String(), err)
	}

	// Best-effort host page: when the resolved item carries a geo_space_id,
	// attach the revision to that space's overview wiki page so reviewers see
	// page context. Failure to find one is fine — the revision is page-less.
	var pageID pgtype.UUID

	if raw, ok := item.Metadata["geo_space_id"].(string); ok && raw != "" {
		var spaceID pgtype.UUID
		if err := spaceID.Scan(raw); err == nil && spaceID.Valid {
			if id, ferr := s.store.FindHostPageID(ctx, spaceID, "overview"); ferr == nil {
				pageID = id
			}
		}
	}

	return s.SubmitFieldOps(ctx, userID, pageID, entityType, entityID, nil, ops, summary, canManage)
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// GetEntityFields returns the editable field schema for an entity (spec §8.1):
// the field groups assigned to its entity class, each field's current stored
// value, and the requesting user's per-field can_view/can_edit flags, plus
// the count of pending revisions targeting the entity.
func (s *Service) GetEntityFields(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (EntityFieldsDTO, error) {
	if entityType == "" || !entityID.Valid {
		return EntityFieldsDTO{}, errors.New("wiki: entity fields: entity_type and a valid entity_id are required")
	}

	if s.fields == nil {
		return EntityFieldsDTO{}, errors.New("wiki: entity fields: custom fields service not configured")
	}

	var spaceTypeID pgtype.UUID
	var spaceTypeSlug string

	if isSpaceBackedEntityType(entityType) {
		var err error

		spaceTypeID, spaceTypeSlug, err = s.store.GetSpaceTypeRef(ctx, entityID)
		if err != nil {
			return EntityFieldsDTO{}, fmt.Errorf("wiki: entity fields: resolve space type: %w", err)
		}
	}

	entityClasses := entityClassesFor(entityType, spaceTypeID, spaceTypeSlug)

	// Gather assignments across every applicable class. entity_id NULL marks
	// a class-wide assignment; a non-NULL entity_id scopes the group to that
	// entity only (§06). Groups are deduped by id — the same group may be
	// assigned under more than one class token.
	var assignments []gen.CustomFieldGroupAssignment
	seenGroups := make(map[string]bool)

	for _, class := range entityClasses {
		rows, err := s.fields.ListAssignmentsByEntityClass(ctx, class)
		if err != nil {
			return EntityFieldsDTO{}, fmt.Errorf("wiki: entity fields: list assignments for %q: %w", class, err)
		}

		for _, a := range rows {
			if a.EntityID.Valid && a.EntityID != entityID {
				continue
			}
			if seenGroups[a.GroupID.String()] {
				continue
			}
			seenGroups[a.GroupID.String()] = true
			assignments = append(assignments, a)
		}
	}

	values, err := s.fields.ListFieldValuesWithDefsByEntity(ctx, customfields.ValueEntityType(entityType), entityID)
	if err != nil {
		return EntityFieldsDTO{}, fmt.Errorf("wiki: entity fields: list values: %w", err)
	}

	valuesByField := make(map[string]gen.ListCustomFieldValuesWithDefsByEntityRow, len(values))
	for _, v := range values {
		valuesByField[v.FieldID.String()] = v
	}

	// wiki.fields.manage holders bypass field-level can_edit at submit, so
	// the schema reports their fields as editable/visible too (§10).
	canManage := s.perms != nil && s.perms.HasPermission(ctx, userID, PermFieldsManage)
	canPropose := canManage ||
		(s.perms != nil && s.perms.HasPermission(ctx, userID, PermFieldsPropose))

	// Batched field-level RBAC (§06): two set lookups instead of one
	// permission query per field. Both fail closed — a lookup error yields
	// empty sets, keeping every field non-visible/non-editable rather than
	// leaking values on a public route.
	var editableSet map[string]bool
	visibleSet := s.visibleFieldIDs(ctx, userID, canManage)

	if !canManage && userID.Valid {
		if ids, perr := s.fields.ListEditableFieldIDsForUser(ctx, userID); perr != nil {
			slog.Warn("wiki: entity fields: editable-perm lookup failed",
				"user_id", userID.String(), "error", perr)
		} else {
			editableSet = make(map[string]bool, len(ids))
			for _, id := range ids {
				editableSet[id.String()] = true
			}
		}
	}

	// Resolve the entity's country currency once for the whole read — the
	// field loops below reuse it for both the display code and the
	// default_source substitution.
	entityCurrency := s.resolveEntityCurrencyCode(ctx, entityType, entityID, isSpaceBackedEntityType(entityType))

	groups := make([]EntityFieldGroupDTO, 0, len(assignments))

	for _, a := range assignments {
		group, err := s.fields.GetFieldGroup(ctx, a.GroupID)
		if err != nil {
			return EntityFieldsDTO{}, fmt.Errorf("wiki: entity fields: get group %s: %w", a.GroupID.String(), err)
		}

		fields, err := s.fields.ListFieldsByGroup(ctx, a.GroupID)
		if err != nil {
			return EntityFieldsDTO{}, fmt.Errorf("wiki: entity fields: list fields for %q: %w", group.Slug, err)
		}

		dtos := make([]EntityFieldDTO, 0, len(fields))

		for _, f := range fields {
			// show_in_frontend gates display for every caller — including
			// wiki.fields.manage holders, whose bypass is edit-only
			// (CanEdit below). Admin-only fields stay out of the read
			// surface instead of leaking through the manage bypass.
			if !f.ShowInFrontend {
				continue
			}
			dto := EntityFieldDTO{
				ID:        f.ID.String(),
				Key:       f.Key,
				Label:     f.Label,
				FieldType: f.FieldType,
				System:    f.IsSystem,
			}
			if f.Section != nil {
				dto.Section = *f.Section
			}

			if len(f.Options) > 0 {
				dto.Options = json.RawMessage(f.Options)
			}

			if v, ok := valuesByField[f.ID.String()]; ok {
				dto.Value = s.entityFieldValueJSON(ctx, v)
			}

			dto.Visible = visibleSet[f.ID.String()] || canManage
			dto.CanEdit = editableSet[f.ID.String()] || canManage

			if !dto.Visible {
				// A field the caller cannot view must not leak its stored
				// value — the schema entry stays so the shape is stable, but
				// the value is withheld on this public route.
				dto.Value = nil
			}

			dtos = append(dtos, dto)
		}

		s.applyComputedFields(ctx, dtos, values, visibleSet, canManage)
		s.applySpaceCurrencyDefaults(ctx, dtos, entityCurrency)

		// A group with no field definitions contributes nothing to the
		// editable schema — skip it so class-wide placeholders don't render
		// empty sections on wiki pages.
		if len(dtos) == 0 {
			continue
		}

		groups = append(groups, EntityFieldGroupDTO{
			ID:       group.ID.String(),
			Slug:     group.Slug,
			Name:     group.Name,
			Fields:   dtos,
			Sections: fieldSections(dtos),
		})
	}

	pending, err := s.store.CountPendingByTarget(ctx, entityType, entityID)
	if err != nil {
		return EntityFieldsDTO{}, fmt.Errorf("wiki: entity fields: count pending: %w", err)
	}

	dto := EntityFieldsDTO{
		Groups:       groups,
		PendingCount: pending,
		CanPropose:   canPropose,
		CanManage:    canManage,
		CanDelete:    s.isSuperAdmin(ctx, userID),
	}
	if len(entityClasses) > 0 {
		dto.EntityClass = entityClasses[0]
	}
	dto.EntityCurrencyCode = entityCurrency
	dto.EntityCurrencyFx = s.currencyFx(ctx, entityCurrency)

	return dto, nil
}

// applySpaceCurrencyDefaults fills empty, visible text fields whose
// options carry {"default_source":"space_currency"} with the resolved
// currency's display name — cfv always wins, so only unset fields are
// touched.
func (s *Service) applySpaceCurrencyDefaults(ctx context.Context, dtos []EntityFieldDTO, currencyCode string) {
	if currencyCode == "" {
		return
	}
	name := ""
	resolved := false
	for i := range dtos {
		dto := &dtos[i]
		if dto.FieldType != "text" || dto.Value != nil || !dto.Visible {
			continue
		}
		if customfields.DefaultSourceFromOptions(dto.Options) != "space_currency" {
			continue
		}
		if !resolved {
			name, resolved = s.store.CurrencyDisplayName(ctx, currencyCode), true
		}
		if name != "" {
			v, _ := json.Marshal(name)
			dto.Value = v
		}
	}
}

// resolveEntityCurrencyCode best-effort resolves the entity's country
// currency (§58) for currency/price_range field display. Space-backed
// entity ids ARE space ids; other entity types reach their space via the
// JCE resolver's geo_space_id metadata (same pattern as SuggestFields).
// Never fails the read — misses and resolver gaps return "".
func (s *Service) resolveEntityCurrencyCode(ctx context.Context, entityType string, entityID pgtype.UUID, spaceBacked bool) string {
	spaceID := entityID

	if !spaceBacked {
		if s.entityResolver == nil {
			return ""
		}
		item, err := s.entityResolver.Resolve(ctx, map[string]any{
			"type": entityType,
			"id":   entityID.String(),
		})
		if err != nil {
			return ""
		}
		raw, ok := item.Metadata["geo_space_id"].(string)
		if !ok || raw == "" {
			return ""
		}
		if err := spaceID.Scan(raw); err != nil || !spaceID.Valid {
			return ""
		}
	}

	code, err := s.store.ResolveSpaceCurrencyCode(ctx, spaceID)
	if err != nil {
		slog.Warn("wiki: entity currency resolve failed",
			"entity_type", entityType, "entity_id", entityID.String(), "error", err)
		return ""
	}
	return code
}

// visibleFieldIDs resolves the caller's viewable-field set: membership-based
// for authenticated users, the guest group's can_view grants for anonymous
// readers (§06), nil for wiki.fields.manage holders who bypass field-level
// RBAC entirely. Fail-closed — a lookup error yields nil so every field
// stays non-visible rather than leaking values on a public route.
func (s *Service) visibleFieldIDs(ctx context.Context, userID pgtype.UUID, canManage bool) map[string]bool {
	if canManage || s.fields == nil {
		return nil
	}

	var (
		ids []pgtype.UUID
		err error
	)

	if userID.Valid {
		ids, err = s.fields.ListVisibleFieldIDsForUser(ctx, userID)
	} else {
		ids, err = s.fields.ListVisibleFieldIDsForGroupSlug(ctx, guestGroupSlug)
	}

	if err != nil {
		slog.Warn("wiki: visible-perm lookup failed",
			"user_id", userID.String(), "anonymous", !userID.Valid, "error", err)
		return nil
	}

	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id.String()] = true
	}

	return set
}

// GetPageFields builds the bound field group's schema + the space's stored
// values for a wiki page (§80 Wiki Types & Field-Group Pages): exactly one
// group — the one the page's category binds — rather than every group
// assigned to the entity's class. Field-level RBAC mirrors GetEntityFields
// (can_view/can_edit per user, fail-closed lookups, wiki.fields.manage
// bypass). Returns nil when the custom-fields service is not configured.
func (s *Service) GetPageFields(ctx context.Context, userID, spaceID, groupID pgtype.UUID) (*EntityFieldGroupDTO, error) {
	if s.fields == nil || !groupID.Valid {
		return nil, nil
	}

	canManage := s.perms != nil && s.perms.HasPermission(ctx, userID, PermFieldsManage)
	visibleSet := s.visibleFieldIDs(ctx, userID, canManage)

	var editableSet map[string]bool

	if !canManage && userID.Valid {
		if ids, perr := s.fields.ListEditableFieldIDsForUser(ctx, userID); perr != nil {
			slog.Warn("wiki: page fields: editable-perm lookup failed",
				"user_id", userID.String(), "error", perr)
		} else {
			editableSet = make(map[string]bool, len(ids))
			for _, id := range ids {
				editableSet[id.String()] = true
			}
		}
	}

	group, err := s.fields.GetFieldGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("wiki: page fields: get group: %w", err)
	}

	fields, err := s.fields.ListFieldsByGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("wiki: page fields: list fields: %w", err)
	}

	values, err := s.fields.ListFieldValuesWithDefsByEntity(ctx, customfields.ValueEntityType("space"), spaceID)
	if err != nil {
		return nil, fmt.Errorf("wiki: page fields: list values: %w", err)
	}

	valuesByField := make(map[string]gen.ListCustomFieldValuesWithDefsByEntityRow, len(values))
	for _, v := range values {
		valuesByField[v.FieldID.String()] = v
	}

	dtos := make([]EntityFieldDTO, 0, len(fields))
	for _, f := range fields {
		// show_in_frontend gates display for every caller — the manage
		// bypass is edit-only (CanEdit), never a display bypass.
		if !f.ShowInFrontend {
			continue
		}
		dto := EntityFieldDTO{
			ID:        f.ID.String(),
			Key:       f.Key,
			Label:     f.Label,
			FieldType: f.FieldType,
			System:    f.IsSystem,
		}
		if f.Section != nil {
			dto.Section = *f.Section
		}

		if len(f.Options) > 0 {
			dto.Options = json.RawMessage(f.Options)
		}

		if v, ok := valuesByField[f.ID.String()]; ok {
			dto.Value = s.entityFieldValueJSON(ctx, v)
		}

		dto.Visible = visibleSet[f.ID.String()] || canManage
		dto.CanEdit = editableSet[f.ID.String()] || canManage

		if !dto.Visible {
			dto.Value = nil
		}

		dtos = append(dtos, dto)
	}

	s.applyComputedFields(ctx, dtos, values, visibleSet, canManage)

	// §58 default_source substitution — the page's host space resolves
	// directly (no entity indirection needed).
	if spaceCurrency, cerr := s.store.ResolveSpaceCurrencyCode(ctx, spaceID); cerr == nil {
		s.applySpaceCurrencyDefaults(ctx, dtos, spaceCurrency)
	}

	if len(dtos) == 0 {
		return nil, nil
	}

	return &EntityFieldGroupDTO{
		ID:       group.ID.String(),
		Slug:     group.Slug,
		Name:     group.Name,
		Fields:   dtos,
		Sections: fieldSections(dtos),
	}, nil
}

// fieldSections groups fields by their section token preserving first-seen
// order — the "" bucket (section-less fields) sorts first so unsectioned
// rows render before the named item tables (§80 Itemized Pricing). Sections
// are always emitted (even a single one) so panel renderers can rely on the
// section label fallback instead of special-casing a flat fields list.
func fieldSections(fields []EntityFieldDTO) []EntityFieldSectionDTO {
	order := []string{}
	byKey := map[string][]EntityFieldDTO{}

	for _, f := range fields {
		if _, ok := byKey[f.Section]; !ok {
			order = append(order, f.Section)
		}
		byKey[f.Section] = append(byKey[f.Section], f)
	}

	// The "" bucket (unsectioned fields) renders first regardless of
	// where it first appeared in field order.
	if len(order) > 0 && order[0] != "" {
		for i, k := range order {
			if k == "" {
				copy(order[1:i+1], order[:i])
				order[0] = ""
				break
			}
		}
	}

	out := make([]EntityFieldSectionDTO, 0, len(order))
	for _, k := range order {
		out = append(out, EntityFieldSectionDTO{Key: k, Fields: byKey[k]})
	}

	return out
}

// StructuredEditsEnabled reports whether the wiki.structured_edits feature
// flag is on (§80 Community Field Ops — default false, requires
// wiki.enabled). Handlers gate ops-bearing submissions on this so the
// page-scoped revisions endpoint cannot bypass the flag when it is off.
// Falls back to system_settings when no feature checker is wired, matching
// Enabled().
func (s *Service) StructuredEditsEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "wiki.structured_edits")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "wiki.structured_edits", false)
}

// GetPageEntityTarget resolves a wiki page (by space slug + page slug) to
// the IDs an entity-targeted ops submission needs: the page ID the revision
// attaches to, the host space ID the ops apply to, and whether the page is
// locked to mod-only edits (§80 — ops submissions on a locked page are
// rejected with the same rule as prose submissions). A slug matching a
// page template but no wiki_pages row materializes the lazy template page
// on the spot (§80: template pages materialize on first submission).
// Returns ErrSpaceNotFound for unknown spaces and pgx.ErrNoRows for
// unknown/untemplated pages.
func (s *Service) GetPageEntityTarget(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string) (pageID, spaceID pgtype.UUID, locked bool, err error) {
	resolvedSpaceID, spaceName, err := s.store.GetSpaceBySlug(ctx, spaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgtype.UUID{}, false, ErrSpaceNotFound
		}

		return pgtype.UUID{}, pgtype.UUID{}, false, fmt.Errorf("wiki: entity target: resolve space: %w", err)
	}

	page, err := s.store.GetWikiPageBySlug(ctx, spaceSlug, pageSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			page, err = s.materializeLazyPage(ctx, resolvedSpaceID, spaceSlug, spaceName, userID, pageSlug)
		}
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, false, err
		}
	}

	return page.ID, page.SpaceID, page.IsLocked, nil
}

// GetRevision returns a single revision by ID — used by the revision-keyed
// admin approve route to fetch content for post-approval RAG indexing (B8).
func (s *Service) GetRevision(ctx context.Context, revID pgtype.UUID) (WikiRevision, error) {
	return s.store.GetRevision(ctx, revID)
}

// AdminListRevisions returns revisions for the admin review queue
// (GET /admin/wiki/revisions) with optional kind/status filters, a q
// search term matched against summary and submitter username, a
// whitelisted sort key ("created_at:asc|desc"), and a deleted toggle
// ("only|all"; default excludes soft-deleted rows) (§80).
func (s *Service) AdminListRevisions(ctx context.Context, kind, status, q, sort, deleted string, page, limit int32) ([]WikiRevisionDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	revs, total, err := s.store.AdminListRevisions(ctx, kind, status, q, sort, deleted, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin list revisions: %w", err)
	}

	dtos := make([]WikiRevisionDTO, 0, len(revs))
	for _, r := range revs {
		dtos = append(dtos, revisionToDTO(r))
	}

	return dtos, total, nil
}

// ListEntityRevisions returns the revision history targeting an entity
// (fields/mixed revisions) for GET /wiki/entities/:type/:id/revisions.
// Pending bodies are redacted for viewers other than the submitter and
// reviewers — same rule as GetRevisionHistory.
func (s *Service) ListEntityRevisions(ctx context.Context, viewerID pgtype.UUID, canReview bool, entityType string, entityID pgtype.UUID, page, limit int32) ([]WikiRevisionDTO, int64, error) {
	if entityType == "" || !entityID.Valid {
		return nil, 0, errors.New("wiki: entity revisions: entity_type and a valid entity_id are required")
	}

	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	revs, total, err := s.store.ListRevisionsByTarget(ctx, entityType, entityID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: entity revisions: %w", err)
	}

	dtos := make([]WikiRevisionDTO, 0, len(revs))
	for _, r := range revs {
		dto := revisionToDTO(r)
		if r.Status == "pending" && !canReview && !(viewerID.Valid && r.SubmittedBy == viewerID) {
			dto.Content = nil
			dto.Ops = nil
			dto.Summary = ""
		}
		dtos = append(dtos, dto)
	}

	return dtos, total, nil
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

// ApproveRevisionByID approves a revision by ID — the canonical review
// action for both page-bound and page-less revisions (spec §7). For
// fields/mixed revisions carrying ops, the status flip and the ops
// application share one transaction (all-or-nothing per §6 step 3); after
// commit, value-affecting ops trigger wiki:fields_propagate.
func (s *Service) ApproveRevisionByID(ctx context.Context, revID, reviewerID pgtype.UUID, notes string) error {
	rev, err := s.store.GetRevision(ctx, revID)
	if err != nil {
		return fmt.Errorf("wiki: approve by id: %w", err)
	}

	ops, err := ParseOps(rev.Ops)
	if err != nil {
		return fmt.Errorf("wiki: approve by id: parse ops: %w", err)
	}

	if rev.Kind != "content" && len(ops) > 0 {
		return s.approveOpsRevision(ctx, revID, reviewerID, notes, ops)
	}

	if _, err := s.store.ApproveRevisionByID(ctx, revID, reviewerID, notes); err != nil {
		return fmt.Errorf("wiki: approve by id: %w", err)
	}

	return nil
}

// RejectRevisionByID rejects a pending revision by ID (moderator+).
func (s *Service) RejectRevisionByID(ctx context.Context, revID, reviewerID pgtype.UUID, notes string) error {
	if err := s.store.RejectRevisionByID(ctx, revID, reviewerID, notes); err != nil {
		return fmt.Errorf("wiki: reject by id: %w", err)
	}

	return nil
}

// approveOpsRevision flips a revision to approved and applies its ops array
// inside one writer transaction — no partial application (§6 step 3). After
// commit it propagates JCE cache busts for the revision's target entity.
func (s *Service) approveOpsRevision(ctx context.Context, revID, reviewerID pgtype.UUID, notes string, ops []FieldOp) error {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("wiki: approve ops: begin tx: %w", err)
	}

	defer tx.Rollback(ctx)

	rev, err := s.store.ApproveRevisionTx(ctx, tx, revID, reviewerID, notes)
	if err != nil {
		return fmt.Errorf("wiki: approve ops: %w", err)
	}

	// Rebuild the target entity's assigned-group schema inside the
	// transaction — set_value/upsert_field resolve field_key within it,
	// and proposal ops register new keys as they apply. Stored target wins;
	// fall back to the ops array for revisions written before targets were
	// recorded.
	targetType, targetID := "", pgtype.UUID{}
	if rev.TargetEntityType != nil {
		targetType = *rev.TargetEntityType
	}
	if rev.TargetEntityID.Valid {
		targetID = rev.TargetEntityID
	}

	if targetType == "" || !targetID.Valid {
		if t, id, terr := resolveOpsTarget("", pgtype.UUID{}, ops); terr == nil {
			targetType, targetID = t, id
		}
	}

	schema, err := s.entitySchemaTx(ctx, tx, targetType, targetID)
	if err != nil {
		_ = tx.Rollback(ctx)

		if serr := s.store.SetApplyError(ctx, revID, err.Error()); serr != nil {
			slog.Warn("wiki: record apply error failed", "revision_id", revID.String(), "error", serr)
		}

		return fmt.Errorf("wiki: approve ops: %w", err)
	}

	if err := s.applyOps(ctx, tx, ops, schema); err != nil {
		// Roll back explicitly BEFORE recording the failure — the tx holds a
		// row lock on this revision (ApproveRevisionTx already UPDATEd it),
		// so SetApplyError's separate-pool write would deadlock waiting for
		// that lock. Then persist the reason so reviewers see why the
		// revision cannot be approved instead of a bare pending row (§80).
		_ = tx.Rollback(ctx)

		if serr := s.store.SetApplyError(ctx, revID, err.Error()); serr != nil {
			slog.Warn("wiki: record apply error failed", "revision_id", revID.String(), "error", serr)
		}

		return fmt.Errorf("wiki: approve ops: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("wiki: approve ops: commit: %w", err)
	}

	s.propagateFields(ctx, rev, ops)

	return nil
}

// PropagateFields busts JCE comparison caches for one entity — the body of
// the wiki:fields_propagate Asynq task, invoked through the worker adapter.
// It is also the synchronous fallback used by propagateFields when no Asynq
// enqueuer is wired (e.g. tests, development).
func (s *Service) PropagateFields(ctx context.Context, entityType, entityID string) error {
	if s.jceBuster == nil {
		return errors.New("wiki: propagate fields: JCE cache buster not configured")
	}

	return s.jceBuster.BustCachesForItemRef(ctx, entityType, entityID)
}

// propagateFields enqueues wiki:fields_propagate for the revision's target
// entity after a successful commit — when the ops array contains a
// value-affecting op (set_value or upsert_field, §6 step 4) OR a schema op
// (propose_field, propose_group, assign_group, delete_field — shape
// changes bust JCE freshness too). When no
// enqueuer is wired it falls back to a synchronous bust so correctness never
// depends on worker wiring order.
func (s *Service) propagateFields(ctx context.Context, rev WikiRevision, ops []FieldOp) {
	if rev.TargetEntityType == nil || !rev.TargetEntityID.Valid {
		return
	}

	affectsValues := false

	for _, op := range ops {
		switch op.Op {
		case OpSetValue, OpUpsertField, OpProposeField, OpProposeGroup, OpAssignGroup, OpDeleteField:
			affectsValues = true
		}
		if affectsValues {
			break
		}
	}

	if !affectsValues {
		return
	}

	entityType := *rev.TargetEntityType
	entityID := rev.TargetEntityID.String()

	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"entity_type": entityType,
			"entity_id":   entityID,
		})
		if err == nil {
			if err := s.enqueuer.Enqueue(TaskFieldsPropagate, payload); err != nil {
				slog.Warn("wiki: fields_propagate enqueue failed, falling back to direct bust",
					"entity_type", entityType, "entity_id", entityID, "error", err)
			} else {
				return
			}
		}
	}

	if s.jceBuster != nil {
		if err := s.jceBuster.BustCachesForItemRef(ctx, entityType, entityID); err != nil {
			slog.Warn("wiki: JCE cache bust failed",
				"entity_type", entityType, "entity_id", entityID, "error", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

// maxOpsPerRevision reads the submission bound from system_settings
// (wiki.suggest.max_ops_per_revision, default 50 — system-settings-catalog).
func (s *Service) maxOpsPerRevision(ctx context.Context) int {
	if s.settings == nil {
		return 50
	}

	return s.settings.GetIntOr(ctx, "wiki.suggest.max_ops_per_revision", 50)
}

// opPermissionNode maps an op name to the RBAC node covering it (§10).
func opPermissionNode(opName string) string {
	switch opName {
	case OpSetValue:
		return PermFieldsEdit
	case OpProposeField, OpProposeGroup:
		return PermFieldsPropose
	case OpUpsertField, OpAssignGroup:
		return PermFieldsManage
	default:
		return ""
	}
}

// checkOpsPermissions enforces the per-op grant map. wiki.fields.manage
// (canManage) covers every op and short-circuits the check — EXCEPT
// delete_field, which is a super_admin-group op and is gated on group
// membership before the manage short-circuit (§80 Super-Admin Direct
// Publish). A nil permission checker fails closed — submissions must
// never bypass the grant map.
func (s *Service) checkOpsPermissions(ctx context.Context, userID pgtype.UUID, ops []FieldOp, canManage bool) error {
	for i, op := range ops {
		if op.Op == OpDeleteField && !s.isSuperAdmin(ctx, userID) {
			return fmt.Errorf("%w: ops[%d] delete_field requires super_admin membership", ErrOpNotPermitted, i)
		}
	}

	if canManage {
		return nil
	}

	if s.perms == nil {
		return errors.New("wiki: permission checker not configured")
	}

	for i, op := range ops {
		node := opPermissionNode(op.Op)
		if node == "" {
			continue // unreachable after ValidateOps — unknown ops already rejected
		}

		if !s.perms.HasPermission(ctx, userID, node) {
			return fmt.Errorf("%w: ops[%d] %s requires %s", ErrOpNotPermitted, i, op.Op, node)
		}
	}

	return nil
}

// checkSetValueFields validates every set_value op against the entity's
// assigned-group field schema (entitySchemaFields — field keys resolve
// inside the schema, never globally): the field must exist in the schema or
// be created by an EARLIER proposal op in the same array (ops apply in
// order — a set_value cannot rely on a proposal that follows it), the value
// must coerce to the field's storage column and belong to its options, and
// the submitter's user group must hold custom_field_permissions.can_edit on
// the field — can_edit is skipped for canManage (§10) and for fields
// proposed earlier in the same array (their §8.1a permission rows are
// seeded at apply time).
func (s *Service) checkSetValueFields(ctx context.Context, userID pgtype.UUID, ops []FieldOp, canManage bool, schema map[string]schemaField) error {
	hasSetValue := false

	for _, op := range ops {
		if op.Op == OpSetValue {
			hasSetValue = true
			break
		}
	}

	if !hasSetValue {
		return nil
	}

	if s.fields == nil {
		// Fail closed — a set_value whose target field cannot be checked
		// must not enter the review queue.
		return errors.New("wiki: custom fields service not configured")
	}

	// One batched editable-set lookup covers every set_value op — avoids a
	// permission query per field (§06; mirrors GetEntityFields). Errors fail
	// closed: the submit is rejected rather than letting an unchecked value
	// into the queue.
	var editableSet map[string]bool

	if !canManage {
		ids, err := s.fields.ListEditableFieldIDsForUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("wiki: submit fields: list editable fields: %w", err)
		}

		editableSet = make(map[string]bool, len(ids))
		for _, id := range ids {
			editableSet[id.String()] = true
		}
	}

	// Keys a proposal op creates are tracked positionally: only an EARLIER
	// proposal counts, since ops apply in order. A set_value whose key was
	// proposed later — or never — is rejected at submit rather than landing
	// in the queue as an unapprovable pending revision.
	proposed := make(map[string]bool)

	for i, op := range ops {
		switch op.Op {
		case OpProposeField, OpUpsertField:
			if op.Field != nil && op.Field.Key != "" {
				proposed[op.Field.Key] = true
			}
			continue
		case OpProposeGroup:
			if op.Group != nil {
				for _, f := range op.Group.Fields {
					if f.Key != "" {
						proposed[f.Key] = true
					}
				}
			}
			continue
		case OpSetValue:
		default:
			continue
		}

		field, exists := schema[op.FieldKey]
		if !exists {
			if proposed[op.FieldKey] {
				continue
			}

			return fmt.Errorf("wiki: submit fields: ops[%d] field %q does not exist in the entity's field schema", i, op.FieldKey)
		}

		// Computed fields derive their value from options.computed
		// sources — manual set_value is rejected for every tier,
		// including wiki.fields.manage. Eligibility gates the same way
		// as the read path so a type-ineligible spec stays a plain
		// manual field rather than locking writes on a field that is
		// never computed.
		if customfields.ComputedEligibleField(field.fieldType, field.options) {
			return fmt.Errorf("%w: ops[%d] field %q is computed and cannot be set manually", ErrOpNotPermitted, i, op.FieldKey)
		}

		var eid pgtype.UUID
		if err := eid.Scan(op.EntityID); err != nil {
			return fmt.Errorf("wiki: submit fields: ops[%d] entity_id %q is not a UUID: %w", i, op.EntityID, err)
		}

		// CF2: sanitize rich_text HTML at submit time, before the same
		// coercion the apply path runs — the queued ops carry clean bytes.
		opValue := op.Value
		if field.fieldType == customfields.FieldTypeRichText && len(opValue) > 0 && wikiRichTextSanitizer != nil {
			opValue = customfields.SanitizeRichTextJSON(opValue, wikiRichTextSanitizer)
		}

		// CF3: canonical select matching is value-only — normalize a
		// submitted label to its option value before membership checks so
		// submit and apply agree with the REST value handler.
		if (field.fieldType == customfields.FieldTypeSelect || field.fieldType == customfields.FieldTypeMultiselect) && len(opValue) > 0 {
			opValue = customfields.CanonicalizeSelectJSON(field.fieldType, field.options, opValue)
		}

		if _, err := valueToUpsertParams(field, customfields.ValueEntityType(op.EntityType), eid, opValue); err != nil {
			return fmt.Errorf("wiki: submit fields: ops[%d]: %w", i, err)
		}

		if err := valueWithinOptions(field, opValue); err != nil {
			return fmt.Errorf("wiki: submit fields: ops[%d]: %w", i, err)
		}

		if canManage {
			continue
		}

		if !editableSet[field.id.String()] {
			return fmt.Errorf("%w: ops[%d] field %q is not editable by your user group", ErrOpNotPermitted, i, op.FieldKey)
		}
	}

	return nil
}

// checkSchemaOpTargets resolves the group targets of schema-changing ops at
// submit time so a revision that can never apply is rejected instead of
// queuing as an unapprovable pending row: a propose_field/upsert_field/
// assign_group group must exist (or be created by an earlier propose_group
// in the same array), a propose_field key must be free inside its group
// (the INSERT would violate the group_id+key unique constraint at apply),
// and a propose_group slug must be free.
func (s *Service) checkSchemaOpTargets(ctx context.Context, ops []FieldOp) error {
	if s.fields == nil {
		for _, op := range ops {
			switch op.Op {
			case OpProposeField, OpProposeGroup, OpUpsertField, OpAssignGroup, OpDeleteField:
				return errors.New("wiki: custom fields service not configured")
			}
		}

		return nil
	}

	// Groups and keys introduced by earlier proposal ops — valid targets for
	// subsequent ops even though they do not exist yet.
	pendingGroups := make(map[string]map[string]bool)

	for i, op := range ops {
		switch op.Op {
		case OpProposeGroup:
			slug := op.Group.Slug
			if _, dup := pendingGroups[slug]; dup {
				return fmt.Errorf("wiki: submit fields: ops[%d] proposes group %q twice", i, slug)
			}

			if _, err := s.fields.GetFieldGroupBySlug(ctx, slug); err == nil {
				return fmt.Errorf("wiki: submit fields: ops[%d] group slug %q already exists", i, slug)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("wiki: submit fields: ops[%d] lookup group %q: %w", i, slug, err)
			}

			pendingGroups[slug] = make(map[string]bool)
			for _, f := range op.Group.Fields {
				pendingGroups[slug][f.Key] = true
			}
		case OpProposeField:
			if pending, ok := pendingGroups[op.GroupSlug]; ok {
				if op.Field != nil {
					if pending[op.Field.Key] {
						return fmt.Errorf("wiki: submit fields: ops[%d] field %q is already proposed for group %q", i, op.Field.Key, op.GroupSlug)
					}
					pending[op.Field.Key] = true
				}

				continue
			}

			group, err := s.fields.GetFieldGroupBySlug(ctx, op.GroupSlug)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("wiki: submit fields: ops[%d] group %q does not exist", i, op.GroupSlug)
				}

				return fmt.Errorf("wiki: submit fields: ops[%d] lookup group %q: %w", i, op.GroupSlug, err)
			}

			if op.Field != nil && op.Field.Key != "" {
				fields, err := s.fields.ListFieldsByGroup(ctx, group.ID)
				if err != nil {
					return fmt.Errorf("wiki: submit fields: ops[%d] list fields for group %q: %w", i, op.GroupSlug, err)
				}

				for _, f := range fields {
					if f.Key == op.Field.Key {
						return fmt.Errorf("wiki: submit fields: ops[%d] field %q already exists in group %q", i, op.Field.Key, op.GroupSlug)
					}
				}
			}
		case OpUpsertField, OpAssignGroup:
			if _, ok := pendingGroups[op.GroupSlug]; ok {
				continue
			}

			if _, err := s.fields.GetFieldGroupBySlug(ctx, op.GroupSlug); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("wiki: submit fields: ops[%d] group %q does not exist", i, op.GroupSlug)
				}

				return fmt.Errorf("wiki: submit fields: ops[%d] lookup group %q: %w", i, op.GroupSlug, err)
			}
		case OpDeleteField:
			if pending, ok := pendingGroups[op.GroupSlug]; ok {
				if !pending[op.FieldKey] {
					return fmt.Errorf("wiki: submit fields: ops[%d] field %q is not proposed for group %q", i, op.FieldKey, op.GroupSlug)
				}

				continue
			}

			group, err := s.fields.GetFieldGroupBySlug(ctx, op.GroupSlug)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("wiki: submit fields: ops[%d] group %q does not exist", i, op.GroupSlug)
				}

				return fmt.Errorf("wiki: submit fields: ops[%d] lookup group %q: %w", i, op.GroupSlug, err)
			}

			// The delete target must resolve now — a revision whose field
			// is already gone would sit pending as unapprovable noise.
			fields, err := s.fields.ListFieldsByGroup(ctx, group.ID)
			if err != nil {
				return fmt.Errorf("wiki: submit fields: ops[%d] list fields for group %q: %w", i, op.GroupSlug, err)
			}

			var target *gen.ListCustomFieldsByGroupRow
			for j := range fields {
				if fields[j].Key == op.FieldKey {
					target = &fields[j]
					break
				}
			}

			if target == nil {
				return fmt.Errorf("wiki: submit fields: ops[%d] field %q does not exist in group %q", i, op.FieldKey, op.GroupSlug)
			}

			// Fail at submit, not apply — system fields are undeletable by
			// contract (applyDeleteField + DeleteCustomField both refuse).
			if target.IsSystem {
				return fmt.Errorf("wiki: submit fields: ops[%d] field %q is system-managed and cannot be deleted", i, op.FieldKey)
			}
		}
	}

	return nil
}

// resolveOpsTarget derives the revision's (target_entity_type,
// target_entity_id) from the explicit parameters, falling back to the first
// op that carries entity fields (set_value ops always do). Ops carrying
// entity fields must agree with the resolved target — a revision that edits
// entity A while claiming entity B would propagate cache busts to the wrong
// entity. Proposal-only revisions may carry no entity fields at all.
func resolveOpsTarget(entityType string, entityID pgtype.UUID, ops []FieldOp) (string, pgtype.UUID, error) {
	targetType := entityType
	targetID := entityID

	if targetType == "" || !targetID.Valid {
		for _, op := range ops {
			if op.EntityType == "" || op.EntityID == "" {
				continue
			}

			var id pgtype.UUID
			if err := id.Scan(op.EntityID); err != nil {
				continue
			}

			targetType, targetID = op.EntityType, id

			break
		}
	}

	for i, op := range ops {
		if op.EntityType == "" && op.EntityID == "" {
			continue
		}

		if targetType == "" || !targetID.Valid {
			return "", pgtype.UUID{}, fmt.Errorf("ops[%d] carries entity fields but no revision target could be resolved", i)
		}

		if op.EntityType != "" && op.EntityType != targetType {
			return "", pgtype.UUID{}, fmt.Errorf("ops[%d] entity_type %q does not match revision target %q", i, op.EntityType, targetType)
		}

		if op.EntityID != "" {
			var id pgtype.UUID
			if err := id.Scan(op.EntityID); err == nil && id != targetID {
				return "", pgtype.UUID{}, fmt.Errorf("ops[%d] entity_id %q does not match revision target %s", i, op.EntityID, targetID.String())
			}
		}
	}

	return targetType, targetID, nil
}

// valueWithinOptions enforces select/multiselect membership (CF3): a
// set_value on a select field must name one of the field's option values,
// and a multiselect must name a subset. Matching is value-only — callers
// normalize labels to values before invoking. The payload may be a bare
// JSON string/array or a {"value": …} / {"values": […]} envelope (CF4,
// via customfields.ChoiceStrings). Other field types (and select fields
// with no options configured) pass through.
func valueWithinOptions(field schemaField, v json.RawMessage) error {
	if field.fieldType != customfields.FieldTypeSelect && field.fieldType != customfields.FieldTypeMultiselect {
		return nil
	}

	if len(field.options) == 0 || string(v) == "null" {
		return nil
	}

	var opts []customfields.FieldOption
	if err := json.Unmarshal(field.options, &opts); err != nil || len(opts) == 0 {
		return nil // malformed/empty options — nothing to check against
	}

	allowed := make(map[string]bool, len(opts))
	for _, o := range opts {
		allowed[o.Value] = true
	}

	for _, val := range customfields.ChoiceStrings(v) {
		if !allowed[val] {
			return fmt.Errorf("field %q: value %q is not one of the field's options", field.key, val)
		}
	}

	// A payload with no readable choice strings but non-null bytes (e.g. a
	// JSON number) is a shape error for choice fields.
	if strings.TrimSpace(string(v)) != "" && !isChoiceShape(v) {
		return fmt.Errorf("field %q expects a JSON string value", field.key)
	}

	return nil
}

// isChoiceShape reports whether raw is a choice-shaped payload: a bare
// string/array or a {"value": …} / {"values": […]} envelope.
func isChoiceShape(raw json.RawMessage) bool {
	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	switch t := probe.(type) {
	case string:
		return true
	case []any:
		return true
	case map[string]any:
		_, hasValue := t["value"]
		_, hasValues := t["values"]
		return hasValue || hasValues
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Entity-class resolution (spec §8.1)
// ---------------------------------------------------------------------------

// spaceBackedEntityTypes are the item_ref entity types backed by the spaces
// table — their entity_class is space_type:{space_type_id} (§8.1).
var spaceBackedEntityTypes = map[string]bool{
	"space": true, "university": true, "country": true, "state": true, "city": true,
}

func isSpaceBackedEntityType(entityType string) bool {
	return spaceBackedEntityTypes[entityType]
}

// entityClassesFor maps an item_ref entity type to the entity_class tokens
// used in custom_field_group_assignments (§8.1 / CF17). This is the wiki-side
// half of the single entity-class resolver pair: customfields'
// EntityTypeFromClass resolves class tokens back to frontend entity_types,
// and CF handlers call that function (never a local copy) so both sides
// share the mapping matrix — see the matrix test in
// internal/customfields (CF17). Space-backed entities match
// the full seeded convention — per-type slug (space_type:university), per-type
// id (space_type:{uuid}, used by ops-created assignments), and the class-wide
// space_type token — so a wiki page surfaces every group associated with its
// space, not just one spelling of the class:
//   - space/university/country/state/city → ["space_type:{slug}",
//     "space_type:{id}", "space_type"]
//   - organization/bank                   → ["organization"]
//   - content, content:{slug}             → ["content_type[:{slug}]"]
//   - guide_category:{slug}               → ["content_type:guide:{slug}"] (§06)
//   - table-backed JCE types              → ["jce_entity:{type}"]
//
// Classes are returned most-specific first; callers dedupe groups by id.
func entityClassesFor(entityType string, spaceTypeID pgtype.UUID, spaceTypeSlug string) []string {
	switch {
	case isSpaceBackedEntityType(entityType):
		classes := make([]string, 0, 3)
		if spaceTypeSlug != "" {
			classes = append(classes, "space_type:"+spaceTypeSlug)
		}
		if spaceTypeID.Valid {
			classes = append(classes, "space_type:"+spaceTypeID.String())
		}
		return append(classes, "space_type")
	case entityType == "organization" || entityType == "bank":
		return []string{"organization"}
	case entityType == "content":
		return []string{"content_type"}
	case strings.HasPrefix(entityType, "content:"):
		return []string{"content_type:" + entityType[len("content:"):]}
	case strings.HasPrefix(entityType, "guide_category:"):
		// Guide sub-categories already have a documented entity_class
		// mapping (§06 EntityClassFromType).
		return []string{customfields.EntityClassFromType(entityType)}
	default:
		return []string{"jce_entity:" + entityType}
	}
}

// mediaFieldTypes carry value_json media references that resolve to
// {id,url,alt} objects on the field read path (§80 gallery fields).
var mediaFieldTypes = map[string]bool{
	customfields.FieldTypeImageSingle: true,
	customfields.FieldTypeImageMulti:  true,
	customfields.FieldTypeMedia:       true,
}

// mediaFieldItem is one resolved media reference inside an image/media
// field value — the frontend gets a usable URL + alt instead of a bare id.
type mediaFieldItem struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	Alt string `json:"alt,omitempty"`
}

// entityFieldValueJSON renders a cfv row to its JSON value, resolving
// image_single/image_multi/media value_json media references into
// {id,url,alt} objects (single fields emit the object, multi/media emit an
// array). Non-media types — and any resolution failure — pass value_json
// through verbatim so a resolver outage never blanks stored values.
func (s *Service) entityFieldValueJSON(ctx context.Context, v gen.ListCustomFieldValuesWithDefsByEntityRow) json.RawMessage {
	if !mediaFieldTypes[v.FieldType] || len(v.ValueJson) == 0 || s.media == nil {
		return cfvValueJSON(v)
	}

	ids, alts := parseMediaFieldRefs(v.ValueJson)
	if len(ids) == 0 {
		return cfvValueJSON(v)
	}

	rows, err := s.store.ListMediaByIDs(ctx, ids)
	if err != nil {
		slog.Warn("wiki: entity fields: media resolution failed", "error", err)
		return cfvValueJSON(v)
	}

	byID := make(map[pgtype.UUID]*gen.Medium, len(rows))
	for i := range rows {
		m := rows[i]
		byID[m.ID] = &m
	}

	items := make([]mediaFieldItem, 0, len(ids))
	for _, id := range ids {
		m := byID[id]
		if m == nil {
			continue
		}
		alt := alts[id.String()]
		if alt == "" && m.AltText != nil {
			alt = *m.AltText
		}
		items = append(items, mediaFieldItem{
			ID:  id.String(),
			URL: s.media.GetMediaURL(m, media.ImageProxyOpts{Width: 960, Format: "webp"}),
			Alt: alt,
		})
	}

	var raw []byte
	if v.FieldType == customfields.FieldTypeImageSingle {
		if len(items) == 0 {
			return nil
		}
		raw, err = json.Marshal(items[0])
	} else {
		raw, err = json.Marshal(items)
	}
	if err != nil {
		return cfvValueJSON(v)
	}

	return json.RawMessage(raw)
}

// parseMediaFieldRefs extracts ordered media IDs (and any authored alt
// text) from the accepted stored shapes:
//
//	"<uuid>"                          — bare id
//	{"value": "<uuid>"}               — wrapped single (select convention)
//	["<uuid>", …]                     — id array
//	{"value": ["<uuid>", …]}          — wrapped array
//	{"media_ids": ["<uuid>", …]}      — §80 documented envelope
//	[{"id": "<uuid>", "alt": "…"}, …] — object array (alt carried through)
func parseMediaFieldRefs(raw []byte) ([]pgtype.UUID, map[string]string) {
	alts := map[string]string{}
	var ids []pgtype.UUID

	addID := func(s string) {
		var id pgtype.UUID
		if err := id.Scan(s); err == nil && id.Valid {
			ids = append(ids, id)
		}
	}
	addObj := func(m map[string]any) {
		s, _ := m["id"].(string)
		addID(s)
		if alt, _ := m["alt"].(string); alt != "" && s != "" {
			alts[s] = alt
		}
	}

	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, nil
	}

	// Unwrap the {"value": …} envelope used by select-style writes and the
	// {"media_ids": […]} envelope the §80 design doc specifies.
	if m, ok := probe.(map[string]any); ok {
		if inner, exists := m["value"]; exists {
			probe = inner
		} else if inner, exists := m["media_ids"]; exists {
			probe = inner
		}
	}

	switch t := probe.(type) {
	case string:
		addID(t)
	case map[string]any:
		addObj(t)
	case []any:
		for _, el := range t {
			switch e := el.(type) {
			case string:
				addID(e)
			case map[string]any:
				addObj(e)
			}
		}
	}

	return ids, alts
}

// cfvValueJSON renders a stored custom_field_values row as the JSON value for
// its field type's storage column — text/number/bool/date marshal to JSON
// scalars and value_json passes through verbatim. Returns nil when no value
// is stored.
func cfvValueJSON(v gen.ListCustomFieldValuesWithDefsByEntityRow) json.RawMessage {
	column, ok := customfields.FieldTypeValueColumn(v.FieldType)
	if !ok {
		return nil
	}

	switch column {
	case "value_text":
		if v.ValueText != nil {
			if b, err := json.Marshal(*v.ValueText); err == nil {
				return b
			}
		}
	case "value_number":
		if v.ValueNumber.Valid {
			if f, err := v.ValueNumber.Float64Value(); err == nil && f.Valid {
				if b, err := json.Marshal(f.Float64); err == nil {
					return b
				}
			}
		}
	case "value_bool":
		if v.ValueBool != nil {
			if b, err := json.Marshal(*v.ValueBool); err == nil {
				return b
			}
		}
	case "value_date":
		if v.ValueDate.Valid {
			if b, err := json.Marshal(v.ValueDate.Time.UTC().Format(time.RFC3339)); err == nil {
				return b
			}
		}
	case "value_json":
		if len(v.ValueJson) > 0 {
			// §80 — writers disagree on the select/multiselect envelope:
			// field ops store the canonical scalar/array while the cfv
			// upsert API and attraction enrichment wrap it as
			// {"value": …} / {"values": […]}. The DTO emits the canonical
			// shape so option-label resolution and the SuggestEdit form
			// seeding work regardless of the writer.
			if v.FieldType == customfields.FieldTypeSelect ||
				v.FieldType == customfields.FieldTypeMultiselect {
				if unwrapped := choiceValueJSON(v.ValueJson, v.FieldType == customfields.FieldTypeMultiselect); unwrapped != nil {
					return unwrapped
				}
			}
			return json.RawMessage(v.ValueJson)
		}
	}

	return nil
}

// choiceValueJSON normalizes a stored select/multiselect value_json to the
// canonical write shape — a bare JSON string for select, a bare array of
// strings for multiselect. Wrapped envelopes ({"value": x}, {"values": […]},
// {"value": […]}) are unwrapped; bare scalars/arrays pass through. Returns
// nil when the value cannot be read as a choice — the caller then passes
// the raw value_json through unchanged.
func choiceValueJSON(raw []byte, multi bool) json.RawMessage {
	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}

	// Unwrap the select-style envelope; an object without either key is
	// not a choice value.
	if m, ok := probe.(map[string]any); ok {
		if inner, exists := m["values"]; exists {
			probe = inner
		} else if inner, exists := m["value"]; exists {
			probe = inner
		} else {
			return nil
		}
	}

	stringSlice := func(arr []any) ([]string, bool) {
		out := make([]string, 0, len(arr))
		for _, el := range arr {
			s, ok := el.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}

	if !multi {
		switch t := probe.(type) {
		case string:
			if out, err := json.Marshal(t); err == nil {
				return out
			}
		case []any:
			// A select wrapped as {"value": […]} degrades to its first
			// member — one value is all a select stores.
			if ss, ok := stringSlice(t); ok && len(ss) > 0 {
				if out, err := json.Marshal(ss[0]); err == nil {
					return out
				}
			}
		}
		return nil
	}

	var ss []string
	switch t := probe.(type) {
	case string:
		ss = []string{t}
	case []any:
		var ok bool
		ss, ok = stringSlice(t)
		if !ok {
			return nil
		}
	default:
		return nil
	}
	if out, err := json.Marshal(ss); err == nil {
		return out
	}

	return nil
}

// schemaFieldReader is the read shape entitySchemaFields needs — satisfied
// by fieldReadStore (submit path, via s.fields) and genSchemaSource (apply
// path, bound to the approval transaction).
type schemaFieldReader interface {
	ListAssignmentsByEntityClass(ctx context.Context, entityClass string) ([]gen.CustomFieldGroupAssignment, error)
	ListFieldsByGroup(ctx context.Context, groupID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error)
}

// genSchemaSource adapts a transaction-bound *gen.Queries to
// schemaFieldReader so the approval path reads the schema inside its own
// transaction.
type genSchemaSource struct{ q *gen.Queries }

func (g genSchemaSource) ListAssignmentsByEntityClass(ctx context.Context, class string) ([]gen.CustomFieldGroupAssignment, error) {
	return g.q.ListAssignmentsByEntityClass(ctx, class)
}

func (g genSchemaSource) ListFieldsByGroup(ctx context.Context, groupID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error) {
	return g.q.ListCustomFieldsByGroup(ctx, groupID)
}

// entitySchema builds the revision target's assigned-group field map — the
// §8.1 "editable schema" scope set_value and upsert_field resolve field_key
// inside. Reads go through s.fields (the same source GetEntityFields uses)
// so submit-time checks and display resolve keys identically. Returns nil
// for target-less revisions, unresolvable entities, or when the
// custom-fields service is not configured.
func (s *Service) entitySchema(ctx context.Context, entityType string, entityID pgtype.UUID) (map[string]schemaField, error) {
	if entityType == "" || !entityID.Valid || s.fields == nil {
		return nil, nil
	}

	var spaceTypeID pgtype.UUID
	var spaceTypeSlug string

	if isSpaceBackedEntityType(entityType) && s.store != nil {
		id, slug, err := s.store.GetSpaceTypeRef(ctx, entityID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}

			return nil, fmt.Errorf("wiki: schema fields: resolve space type: %w", err)
		}

		spaceTypeID, spaceTypeSlug = id, slug
	}

	return entitySchemaFields(ctx, s.fields, entityType, entityID, spaceTypeID, spaceTypeSlug)
}

// entitySchemaTx is the apply-path twin of entitySchema: it reads the schema
// inside the caller's transaction (no dependency on the injected read
// service) so the ops apply against the state the approval will commit.
func (s *Service) entitySchemaTx(ctx context.Context, tx pgx.Tx, entityType string, entityID pgtype.UUID) (map[string]schemaField, error) {
	if entityType == "" || !entityID.Valid {
		return nil, nil
	}

	q := gen.New(tx)

	var spaceTypeID pgtype.UUID
	var spaceTypeSlug string

	if isSpaceBackedEntityType(entityType) {
		ref, err := q.GetSpaceTypeRef(ctx, entityID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}

			return nil, fmt.Errorf("wiki: schema fields: resolve space type: %w", err)
		}

		spaceTypeID, spaceTypeSlug = ref.SpaceTypeID, ref.SpaceTypeSlug
	}

	return entitySchemaFields(ctx, genSchemaSource{q}, entityType, entityID, spaceTypeID, spaceTypeSlug)
}

// entitySchemaFields builds the field_key → field map over the entity's
// assigned groups. Field keys are unique only per group (custom_fields
// group_id+key), not globally — a global key lookup can return a field from
// an unrelated group and would check permissions on, or write values to,
// the wrong row. When a key exists in two assigned groups the first (in
// assignment order) wins — matching what GetEntityFields displays.
func entitySchemaFields(ctx context.Context, f schemaFieldReader, entityType string, entityID pgtype.UUID, spaceTypeID pgtype.UUID, spaceTypeSlug string) (map[string]schemaField, error) {
	seenGroups := make(map[string]bool)
	out := make(map[string]schemaField)

	for _, class := range entityClassesFor(entityType, spaceTypeID, spaceTypeSlug) {
		assignments, err := f.ListAssignmentsByEntityClass(ctx, class)
		if err != nil {
			return nil, fmt.Errorf("wiki: schema fields: list assignments for %q: %w", class, err)
		}

		for _, a := range assignments {
			if a.EntityID.Valid && a.EntityID != entityID {
				continue
			}
			if seenGroups[a.GroupID.String()] {
				continue
			}
			seenGroups[a.GroupID.String()] = true

			fields, err := f.ListFieldsByGroup(ctx, a.GroupID)
			if err != nil {
				return nil, fmt.Errorf("wiki: schema fields: list fields for group %s: %w", a.GroupID.String(), err)
			}

			for _, field := range fields {
				if _, dup := out[field.Key]; !dup {
					out[field.Key] = schemaField{
						id:        field.ID,
						key:       field.Key,
						fieldType: field.FieldType,
						options:   field.Options,
					}
				}
			}
		}
	}

	return out, nil
}
