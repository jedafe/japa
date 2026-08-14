// Package personas — service.go holds the business logic for the AI Persona
// System: transactional mass provisioning, CRUD, activity-log retrieval, and
// the high-stakes content gate (ADR-068 §5).
//
// Ref: japaconnect-engineering/sections/68-ai-personas.md
//
//	japaconnect-engineering/adr/ADR-068-ai-persona-system.md
package personas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Disclosure / autonomy / status constants — mirror the CHECK constraints on
// ai_personas (migration 000246).
const (
	DisclosureModeDisclosed   = "disclosed"
	DisclosureModeUndisclosed = "undisclosed"

	AutonomyModeAutonomous = "autonomous"
	AutonomyModeQueued     = "queued"

	StatusActive  = "active"
	StatusPaused  = "paused"
	StatusRetired = "retired"
)

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// The worker.Enqueuer satisfies this.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// ActivityPolicy is the JSON config stored in ai_personas.activity_policy.
// It governs how the orchestration engine schedules persona actions.
//
// Ref: japaconnect-engineering/sections/68-ai-personas.md
type ActivityPolicy struct {
	ActionsPerDay        int            `json:"actions_per_day"`
	ActionBudget         map[string]int `json:"action_budget"`
	EligibleContentTypes []string       `json:"eligible_content_types"`
	EligibleSpaces       []string       `json:"eligible_spaces"`
	MinContentAgeHours   int            `json:"min_content_age_hours"`
	AvoidOwnContent      bool           `json:"avoid_own_content"`
	AvoidRecentAuthors   []string       `json:"avoid_recent_authors"`
	ActiveHoursUTC       [2]int         `json:"active_hours_utc"`
}

// PersonaInput is a single persona in a mass-provisioning request.
type PersonaInput struct {
	Name           string         `json:"name"`
	Slug           string         `json:"slug"`
	Tagline        string         `json:"tagline"`
	VoicePrompt    string         `json:"voice_prompt"`
	DisclosureMode string         `json:"disclosure_mode"`
	AutonomyMode   string         `json:"autonomy_mode"`
	ActivityPolicy ActivityPolicy `json:"activity_policy"`
	TypeID         string         `json:"type_id"` // optional; defaults to "contributor"/"columnist" lookup
}

// personaEmail builds a deterministic, tenant-scoped internal email for a
// persona backing account. These accounts never log in (is_bot = true), so
// the email is an internal identifier, not a deliverable address. The
// tenant_id is embedded to avoid UNIQUE(tenant_id, email) collisions when
// two tenants create personas with the same slug (BUG-008).
func personaEmail(slug string, tenantID pgtype.UUID) string {
	return fmt.Sprintf("persona+%s+%s@bot.internal", slug, tenantID.String())
}

// PersonaDetail is the API-facing persona record with joined user + profile
// names, the decoded activity_policy, and recent actions (§68 spec:
// "persona detail + activity_policy + recent actions").
type PersonaDetail struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	UserID          string          `json:"user_id"`
	ProfileID       string          `json:"profile_id"`
	DisclosureMode  string          `json:"disclosure_mode"`
	AutonomyMode    string          `json:"autonomy_mode"`
	VoicePrompt     *string         `json:"voice_prompt,omitempty"`
	ActivityPolicy  json.RawMessage `json:"activity_policy"`
	Status          string          `json:"status"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
	BackingUsername string          `json:"backing_username"`
	BackingEmail    string          `json:"backing_email"`
	ProfileName     string          `json:"profile_name"`
	ProfileSlug     string          `json:"profile_slug"`
	ProfileTagline  *string         `json:"profile_tagline,omitempty"`
	RecentActivity  []ActivityEntry `json:"recent_activity"`
}

// PersonaListItem is the list-view persona record (fewer joined fields).
// Includes activity stats (total_actions, last_action_at) per §68 spec
// ("list all personas + activity stats").
type PersonaListItem struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	UserID          string          `json:"user_id"`
	ProfileID       string          `json:"profile_id"`
	DisclosureMode  string          `json:"disclosure_mode"`
	AutonomyMode    string          `json:"autonomy_mode"`
	VoicePrompt     *string         `json:"voice_prompt,omitempty"`
	ActivityPolicy  json.RawMessage `json:"activity_policy"`
	Status          string          `json:"status"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
	BackingUsername string          `json:"backing_username"`
	ProfileName     string          `json:"profile_name"`
	TotalActions    int64           `json:"total_actions"`
	LastActionAt    *string         `json:"last_action_at,omitempty"`
}

// ActivityEntry is a single platform_events row in the persona activity log.
type ActivityEntry struct {
	ID          int64           `json:"id"`
	EventType   string          `json:"event_type"`
	EntityType  *string         `json:"entity_type,omitempty"`
	EntityID    string          `json:"entity_id,omitempty"`
	Meta        json.RawMessage `json:"meta"`
	IsSynthetic bool            `json:"is_synthetic"`
	CreatedAt   string          `json:"created_at"`
}

// Service holds the business logic for the AI Persona System. It owns the
// transactional provisioning boundary (ADR-068 §7: partial provision is a
// data integrity failure) and reads persona.* settings via config.Settings.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates a personas service.
func NewService(store *Store, settings *config.Settings, enqueuer TaskEnqueuer) *Service {
	return &Service{store: store, settings: settings, enqueuer: enqueuer}
}

// SetEnqueuer wires (or rewires) the Asynq enqueuer after construction.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// ---------------------------------------------------------------------------
// Transactional mass provisioning (ADR-068 §7)
// ---------------------------------------------------------------------------

// ProvisionResult is the outcome for a single persona in a mass-provisioning
// batch. On success, ID holds the new ai_personas.id; on failure, Error holds
// the reason. The transaction is all-or-nothing, so a non-empty Errors slice
// means nothing was committed.
type ProvisionResult struct {
	Input PersonaInput `json:"input"`
	ID    string       `json:"id,omitempty"`
	Error string       `json:"error,omitempty"`
}

// ProvisionPersonas transactionally provisions a batch of AI personas. For
// each persona, in ONE transaction: create the users row (is_bot=true),
// create the professional_profiles byline, create the ai_personas config row.
// If any step fails for any persona, the whole transaction rolls back —
// partial provision is a data integrity failure (ADR-068 §7).
//
// The admin user ID is recorded as created_by on every ai_personas row.
//
// The batch size is capped by the persona.max_batch_size system setting
// (BUG-006: an unbounded batch holds a long write transaction and can DoS
// the writer pool). Default cap is 100 when the setting is unset.
func (s *Service) ProvisionPersonas(ctx context.Context, tenantID, adminUserID pgtype.UUID, inputs []PersonaInput) ([]ProvisionResult, error) {
	if len(inputs) == 0 {
		return nil, errors.New("personas: no personas to provision")
	}

	// BUG-006: cap the batch size via system_settings (Rule 3: configurable).
	maxBatch := 100
	if s.settings != nil {
		maxBatch = s.settings.GetIntOr(ctx, tenantID, "persona.max_batch_size", 100)
	}
	if maxBatch <= 0 {
		maxBatch = 100 // defensive: a misconfigured zero/negative falls back
	}
	if len(inputs) > maxBatch {
		return nil, fmt.Errorf("personas: batch size %d exceeds persona.max_batch_size %d: %w", len(inputs), maxBatch, errpkg.ErrInvalid)
	}

	// Validate every input up front so we fail fast before opening a tx.
	for i, in := range inputs {
		if in.Name == "" || in.Slug == "" {
			return nil, fmt.Errorf("personas: input %d: name and slug are required: %w", i, errpkg.ErrInvalid)
		}
		if err := validateMode(in.DisclosureMode, "disclosure_mode",
			DisclosureModeDisclosed, DisclosureModeUndisclosed); err != nil {
			return nil, fmt.Errorf("personas: input %d: %w: %w", i, errpkg.ErrInvalid, err)
		}
		if err := validateMode(in.AutonomyMode, "autonomy_mode",
			AutonomyModeAutonomous, AutonomyModeQueued); err != nil {
			return nil, fmt.Errorf("personas: input %d: %w: %w", i, errpkg.ErrInvalid, err)
		}
		if err := validateActivityPolicy(in.ActivityPolicy); err != nil {
			return nil, fmt.Errorf("personas: input %d: %w: %w", i, errpkg.ErrInvalid, err)
		}
	}

	// Pre-validate duplicate slugs within the batch (BUG-013). Without this,
	// the second CreatePersonaUser hits a UNIQUE(tenant_id, email) constraint
	// violation inside the tx and the admin gets a generic "provisioning
	// failed" error with no indication of which slug collided.
	seenSlugs := make(map[string]int, len(inputs))
	for i, in := range inputs {
		if prev, dup := seenSlugs[in.Slug]; dup {
			return nil, fmt.Errorf("personas: duplicate slug %q in batch (inputs %d and %d): %w", in.Slug, prev, i, errpkg.ErrInvalid)
		}
		seenSlugs[in.Slug] = i
	}

	pool := s.store.DB().Writer
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("personas: begin provisioning tx: %w", err)
	}
	defer tx.Rollback(ctx) // safe to call after Commit (no-op)

	q := gen.New(tx)

	// Resolve the default profile type only if at least one input omits
	// type_id (BUG-007: resolving unconditionally blocked provisioning on
	// tenants that lack 'contributor'/'columnist' types even when every
	// input supplied an explicit type_id).
	needsDefault := false
	for _, in := range inputs {
		if in.TypeID == "" {
			needsDefault = true
			break
		}
	}
	var defaultTypeID pgtype.UUID
	if needsDefault {
		defaultTypeID, err = s.resolveDefaultTypeID(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("personas: resolve default profile type: %w", err)
		}
	}

	results := make([]ProvisionResult, len(inputs))

	for i, in := range inputs {
		// Apply defaults for empty modes.
		disclosure := in.DisclosureMode
		if disclosure == "" {
			disclosure = DisclosureModeDisclosed
		}
		autonomy := in.AutonomyMode
		if autonomy == "" {
			autonomy = AutonomyModeQueued
		}

		// Resolve the profile type_id for this input.
		typeID := defaultTypeID
		if in.TypeID != "" {
			var tid pgtype.UUID
			if err := tid.Scan(in.TypeID); err != nil {
				results[i] = ProvisionResult{Input: in, Error: "invalid type_id"}
				return results, fmt.Errorf("personas: input %d: invalid type_id", i)
			}
			typeID = tid
		}

		// 1. Create the backing user (is_bot = true).
		email := personaEmail(in.Slug, tenantID)
		user, err := s.store.CreatePersonaUser(ctx, q, tenantID, email, in.Slug)
		if err != nil {
			results[i] = ProvisionResult{Input: in, Error: err.Error()}
			return results, fmt.Errorf("personas: input %d: create user: %w", i, err)
		}

		// 2. Create the professional_profiles byline.
		var tagline *string
		if in.Tagline != "" {
			t := in.Tagline
			tagline = &t
		}
		var desc *string
		profile, err := s.store.CreateProfessionalProfile(ctx, q, gen.CreateProfessionalProfileParams{
			TenantID:    tenantID,
			OwnerUserID: user.ID,
			TypeID:      typeID,
			Name:        in.Name,
			Slug:        in.Slug,
			Tagline:     tagline,
			Description: desc,
			FieldValues: []byte("{}"),
		})
		if err != nil {
			results[i] = ProvisionResult{Input: in, Error: err.Error()}
			return results, fmt.Errorf("personas: input %d: create profile: %w", i, err)
		}

		// 3. Marshal the activity policy and create the ai_personas config row.
		policyBytes, err := json.Marshal(in.ActivityPolicy)
		if err != nil {
			results[i] = ProvisionResult{Input: in, Error: err.Error()}
			return results, fmt.Errorf("personas: input %d: marshal policy: %w", i, err)
		}
		var voicePrompt *string
		if in.VoicePrompt != "" {
			vp := in.VoicePrompt
			voicePrompt = &vp
		}
		persona, err := s.store.CreateAIPersona(ctx, q, gen.CreateAIPersonaParams{
			TenantID:       tenantID,
			UserID:         user.ID,
			ProfileID:      profile.ID,
			DisclosureMode: disclosure,
			AutonomyMode:   autonomy,
			VoicePrompt:    voicePrompt,
			ActivityPolicy: policyBytes,
			Status:         StatusActive,
			CreatedBy:      adminUserID,
		})
		if err != nil {
			results[i] = ProvisionResult{Input: in, Error: err.Error()}
			return results, fmt.Errorf("personas: input %d: create persona: %w", i, err)
		}

		results[i] = ProvisionResult{Input: in, ID: persona.ID.String()}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("personas: commit provisioning tx: %w", err)
	}
	return results, nil
}

// resolveDefaultTypeID looks up a "contributor" profile type, falling back to
// "columnist". Returns an error if neither exists — the tenant must seed at
// least one of these types before provisioning personas.
func (s *Service) resolveDefaultTypeID(ctx context.Context, tenantID pgtype.UUID) (pgtype.UUID, error) {
	for _, slug := range []string{"contributor", "columnist"} {
		pt, err := s.store.GetProfileTypeBySlug(ctx, tenantID, slug)
		if err == nil {
			return pt.ID, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, err
		}
	}
	return pgtype.UUID{}, errors.New("personas: no 'contributor' or 'columnist' profile type found — seed one or pass type_id")
}

// validateMode returns an error if mode is non-empty and not one of allowed.
func validateMode(mode, field string, allowed ...string) error {
	if mode == "" {
		return nil // empty means "use default" — handled by the caller
	}
	for _, a := range allowed {
		if mode == a {
			return nil
		}
	}
	return fmt.Errorf("invalid %s %q (allowed: %s)", field, mode, strings.Join(allowed, ", "))
}

// validateActivityPolicy validates an ActivityPolicy for sensible values
// (BUG-001). The policy governs the orchestration engine's scheduling, so
// nonsensical values (negative actions, reversed hours, out-of-range hours)
// would produce broken schedules.
func validateActivityPolicy(p ActivityPolicy) error {
	if p.ActionsPerDay < 0 {
		return fmt.Errorf("activity_policy.actions_per_day must be >= 0, got %d", p.ActionsPerDay)
	}
	for action, budget := range p.ActionBudget {
		if budget < 0 {
			return fmt.Errorf("activity_policy.action_budget[%q] must be >= 0, got %d", action, budget)
		}
	}
	if p.MinContentAgeHours < 0 {
		return fmt.Errorf("activity_policy.min_content_age_hours must be >= 0, got %d", p.MinContentAgeHours)
	}
	start, end := p.ActiveHoursUTC[0], p.ActiveHoursUTC[1]
	if start < 0 || start > 23 || end < 0 || end > 23 {
		return fmt.Errorf("activity_policy.active_hours_utc values must be 0-23, got [%d,%d]", start, end)
	}
	// Allow [0,0] as "no restriction" (zero-value default). Otherwise the
	// start must be strictly less than end so the window is non-empty.
	if !(start == 0 && end == 0) && start >= end {
		return fmt.Errorf("activity_policy.active_hours_utc start must be < end, got [%d,%d]", start, end)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Persona read / update
// ---------------------------------------------------------------------------

// GetPersona returns a single persona by ID with joined user + profile names,
// the decoded activity_policy, and the 5 most recent synthetic activity
// entries (§68 spec: "persona detail + activity_policy + recent actions").
func (s *Service) GetPersona(ctx context.Context, tenantID, id pgtype.UUID) (PersonaDetail, error) {
	row, err := s.store.GetAIPersonaByID(ctx, tenantID, id)
	if err != nil {
		return PersonaDetail{}, err
	}
	detail := personaDetailFromRow(row)

	// Fetch the 5 most recent synthetic platform_events for the persona's
	// backing user (§68 "recent actions" in the detail response). Use the
	// raw UUID from the row (detail.UserID is a string for JSON output).
	recentRows, err := s.store.ListPersonaActivity(ctx, tenantID, row.UserID, 5, 0)
	if err != nil {
		// Don't fail the whole request if activity fetch fails — the persona
		// config is the primary payload. Log and return empty recent_activity.
		slog.Warn("personas: get recent activity failed", "error", err, "id", id.String())
		detail.RecentActivity = []ActivityEntry{}
		return detail, nil
	}
	recent := make([]ActivityEntry, 0, len(recentRows))
	for _, r := range recentRows {
		recent = append(recent, activityEntryFromRow(r))
	}
	detail.RecentActivity = recent
	return detail, nil
}

// ListPersonas returns a paginated list of personas for a tenant. An empty
// status returns all statuses; an empty q matches all rows; sort/dir default
// to created_at DESC.
func (s *Service) ListPersonas(ctx context.Context, tenantID pgtype.UUID, status, q, sort, dir string, limit, offset int32) ([]PersonaListItem, int64, error) {
	rows, err := s.store.ListAIPersonas(ctx, tenantID, status, q, sort, dir, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountAIPersonas(ctx, tenantID, status, q)
	if err != nil {
		return nil, 0, err
	}
	items := make([]PersonaListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, personaListItemFromRow(r))
	}
	return items, total, nil
}

// UpdatePersonaInput holds the optional fields for a persona update.
type UpdatePersonaInput struct {
	DisclosureMode *string
	AutonomyMode   *string
	VoicePrompt    *string
	ActivityPolicy *ActivityPolicy
	Status         *string
}

// UpdatePersona updates a persona's config (activity_policy, modes, status,
// voice_prompt). Nil/empty fields are left unchanged. Mode and status values
// are validated up front so invalid input returns a clean 400 validation_error
// instead of a raw CHECK-constraint 500 (BUG-001, Rule 5).
//
// Returns a PersonaDetail (with joined user + profile names) so the response
// shape matches HandleGet (BUG-004: previously returned the raw
// gen.AiPersona row, inconsistent with the rest of the API).
func (s *Service) UpdatePersona(ctx context.Context, tenantID, id pgtype.UUID, in UpdatePersonaInput) (PersonaDetail, error) {
	if in.DisclosureMode != nil {
		if err := validateMode(*in.DisclosureMode, "disclosure_mode",
			DisclosureModeDisclosed, DisclosureModeUndisclosed); err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: %w: %w", errpkg.ErrInvalid, err)
		}
	}
	if in.AutonomyMode != nil {
		if err := validateMode(*in.AutonomyMode, "autonomy_mode",
			AutonomyModeAutonomous, AutonomyModeQueued); err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: %w: %w", errpkg.ErrInvalid, err)
		}
	}
	if in.Status != nil {
		if err := validateMode(*in.Status, "status",
			StatusActive, StatusPaused, StatusRetired); err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: %w: %w", errpkg.ErrInvalid, err)
		}
	}
	if in.ActivityPolicy != nil {
		if err := validateActivityPolicy(*in.ActivityPolicy); err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: %w: %w", errpkg.ErrInvalid, err)
		}
	}

	params := gen.UpdateAIPersonaParams{
		DisclosureMode: in.DisclosureMode,
		AutonomyMode:   in.AutonomyMode,
		VoicePrompt:    in.VoicePrompt,
		Status:         in.Status,
		TenantID:       tenantID,
		ID:             id,
	}
	if in.ActivityPolicy != nil {
		b, err := json.Marshal(*in.ActivityPolicy)
		if err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: marshal activity policy: %w", err)
		}
		params.ActivityPolicy = b
	}
	if _, err := s.store.UpdateAIPersona(ctx, params); err != nil {
		return PersonaDetail{}, err
	}
	// Re-fetch the joined row so the response includes user + profile names
	// (BUG-004: consistent with HandleGet's PersonaDetail shape).
	return s.GetPersona(ctx, tenantID, id)
}

// GetPersonaActivity returns a paginated activity log (platform_events) for a
// persona's backing user.
func (s *Service) GetPersonaActivity(ctx context.Context, tenantID, personaID pgtype.UUID, limit, offset int32) ([]ActivityEntry, int64, error) {
	detail, err := s.store.GetAIPersonaByID(ctx, tenantID, personaID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.store.ListPersonaActivity(ctx, tenantID, detail.UserID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountPersonaActivity(ctx, tenantID, detail.UserID)
	if err != nil {
		return nil, 0, err
	}
	entries := make([]ActivityEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, activityEntryFromRow(r))
	}
	return entries, total, nil
}

// ---------------------------------------------------------------------------
// High-stakes content gate (ADR-068 §5 — hardcoded, NOT configurable)
// ---------------------------------------------------------------------------

// IsHighStakesContent returns true if the content is in a high-stakes category
// or carries a high-stakes tag. This gate is hardcoded — it is NOT
// configurable via system_settings (ADR-068 §5). A persona acting on
// high-stakes content must route through the human-approval queue regardless
// of its autonomy_mode.
//
// The canonical implementation lives in the participation package
// (internal/participation/service.go) and lowercases inputs for case-
// insensitive matching. This wrapper delegates to it so there is a single
// source of truth for the high-stakes gate (BUG-015). We use a function
// variable to avoid an import cycle (participation imports personas; personas
// must not import participation).
var highStakesGateFn = func(guideCategory string, tags []string) bool {
	// Fallback: case-sensitive match. Replaced at server wiring time by
	// personas.SetHighStakesGate so all three packages share one
	// case-insensitive implementation.
	cat := map[string]bool{
		"visa": true, "banking": true, "grants": true, "health_insurance": true,
	}
	tagSet := map[string]bool{"immigration-advice": true, "financial-advice": true}
	if cat[guideCategory] {
		return true
	}
	for _, t := range tags {
		if tagSet[t] {
			return true
		}
	}
	return false
}

// SetHighStakesGate injects the canonical high-stakes gate function from the
// participation package. Called once at server wiring time to replace the
// fallback case-sensitive implementation with the canonical case-insensitive
// one (BUG-015).
func SetHighStakesGate(fn func(string, []string) bool) {
	highStakesGateFn = fn
}

// IsHighStakesContent delegates to the canonical high-stakes gate.
func IsHighStakesContent(guideCategory string, tags []string) bool {
	return highStakesGateFn(guideCategory, tags)
}

// ---------------------------------------------------------------------------
// DTO mapping helpers
// ---------------------------------------------------------------------------

func personaDetailFromRow(r gen.GetAIPersonaByIDRow) PersonaDetail {
	d := PersonaDetail{
		ID:              r.ID.String(),
		TenantID:        r.TenantID.String(),
		UserID:          r.UserID.String(),
		ProfileID:       r.ProfileID.String(),
		DisclosureMode:  r.DisclosureMode,
		AutonomyMode:    r.AutonomyMode,
		VoicePrompt:     r.VoicePrompt,
		ActivityPolicy:  rawJSON(r.ActivityPolicy),
		Status:          r.Status,
		CreatedBy:       r.CreatedBy.String(),
		CreatedAt:       r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		BackingUsername: r.BackingUsername,
		BackingEmail:    r.BackingEmail,
		ProfileName:     r.ProfileName,
		ProfileSlug:     r.ProfileSlug,
		ProfileTagline:  r.ProfileTagline,
	}
	return d
}

func personaListItemFromRow(r gen.ListAIPersonasRow) PersonaListItem {
	item := PersonaListItem{
		ID:              r.ID.String(),
		TenantID:        r.TenantID.String(),
		UserID:          r.UserID.String(),
		ProfileID:       r.ProfileID.String(),
		DisclosureMode:  r.DisclosureMode,
		AutonomyMode:    r.AutonomyMode,
		VoicePrompt:     r.VoicePrompt,
		ActivityPolicy:  rawJSON(r.ActivityPolicy),
		Status:          r.Status,
		CreatedBy:       r.CreatedBy.String(),
		CreatedAt:       r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		BackingUsername: r.BackingUsername,
		ProfileName:     r.ProfileName,
		TotalActions:    r.TotalActions,
	}
	// LastActionAt is interface{} from sqlc (MAX(timestamptz) is nullable).
	// Type-assert to pgtype.Timestamptz and format only if valid.
	if ts, ok := r.LastActionAt.(pgtype.Timestamptz); ok && ts.Valid {
		s := ts.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		item.LastActionAt = &s
	}
	return item
}

func activityEntryFromRow(r gen.ListPersonaActivityRow) ActivityEntry {
	e := ActivityEntry{
		ID:          r.ID,
		EventType:   r.EventType,
		EntityType:  r.EntityType,
		Meta:        rawJSON(r.Meta),
		IsSynthetic: r.IsSynthetic,
		CreatedAt:   r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if r.EntityID.Valid {
		e.EntityID = r.EntityID.String()
	}
	return e
}

// rawJSON normalizes a sqlc jsonb []byte into a non-nil json.RawMessage so
// the API always emits a JSON object (never null) for jsonb columns.
func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("{}")
	}
	return json.RawMessage(b)
}
