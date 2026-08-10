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
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Disclosure / autonomy / status constants — mirror the CHECK constraints on
// ai_personas (migration 000246).
const (
	DisclosureModeDisclosed   = "disclosed"
	DisclosureModeUndisclosed = "undisclosed"

	AutonomyModeAutonomous = "autonomous"
	AutonomyModeQueued     = "queued"

	StatusActive = "active"
	StatusPaused = "paused"
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
// names and the decoded activity_policy.
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
}

// PersonaListItem is the list-view persona record (fewer joined fields).
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
func (s *Service) ProvisionPersonas(ctx context.Context, tenantID, adminUserID pgtype.UUID, inputs []PersonaInput) ([]ProvisionResult, error) {
	if len(inputs) == 0 {
		return nil, errors.New("personas: no personas to provision")
	}

	// Validate every input up front so we fail fast before opening a tx.
	for i, in := range inputs {
		if in.Name == "" || in.Slug == "" {
			return nil, fmt.Errorf("personas: input %d: name and slug are required", i)
		}
		if err := validateMode(in.DisclosureMode, "disclosure_mode",
			DisclosureModeDisclosed, DisclosureModeUndisclosed); err != nil {
			return nil, fmt.Errorf("personas: input %d: %w", i, err)
		}
		if err := validateMode(in.AutonomyMode, "autonomy_mode",
			AutonomyModeAutonomous, AutonomyModeQueued); err != nil {
			return nil, fmt.Errorf("personas: input %d: %w", i, err)
		}
	}

	pool := s.store.DB().Writer
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("personas: begin provisioning tx: %w", err)
	}
	defer tx.Rollback(ctx) // safe to call after Commit (no-op)

	q := gen.New(tx)

	// Resolve the default profile type once (used when an input omits type_id).
	defaultTypeID, err := s.resolveDefaultTypeID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("personas: resolve default profile type: %w", err)
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

// ---------------------------------------------------------------------------
// Persona read / update
// ---------------------------------------------------------------------------

// GetPersona returns a single persona by ID with joined user + profile names
// and the decoded activity_policy.
func (s *Service) GetPersona(ctx context.Context, tenantID, id pgtype.UUID) (PersonaDetail, error) {
	row, err := s.store.GetAIPersonaByID(ctx, tenantID, id)
	if err != nil {
		return PersonaDetail{}, err
	}
	return personaDetailFromRow(row), nil
}

// ListPersonas returns a paginated list of personas for a tenant. An empty
// status returns all statuses.
func (s *Service) ListPersonas(ctx context.Context, tenantID pgtype.UUID, status string, limit, offset int32) ([]PersonaListItem, int64, error) {
	rows, err := s.store.ListAIPersonas(ctx, tenantID, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountAIPersonas(ctx, tenantID, status)
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
	DisclosureMode  *string
	AutonomyMode    *string
	VoicePrompt     *string
	ActivityPolicy  *ActivityPolicy
	Status          *string
}

// UpdatePersona updates a persona's config (activity_policy, modes, status,
// voice_prompt). Nil/empty fields are left unchanged.
func (s *Service) UpdatePersona(ctx context.Context, tenantID, id pgtype.UUID, in UpdatePersonaInput) (gen.AiPersona, error) {
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
			return gen.AiPersona{}, fmt.Errorf("personas: marshal activity policy: %w", err)
		}
		params.ActivityPolicy = b
	}
	return s.store.UpdateAIPersona(ctx, params)
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
	return PersonaListItem{
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
	}
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
