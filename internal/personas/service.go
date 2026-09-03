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
	Name              string         `json:"name"`
	Slug              string         `json:"slug"`
	Tagline           string         `json:"tagline"`
	VoiceID           string         `json:"voice_id"`
	VoicePrompt       string         `json:"voice_prompt"`
	DisclosureMode    string         `json:"disclosure_mode"`
	AutonomyMode      string         `json:"autonomy_mode"`
	ActivityPolicy    ActivityPolicy `json:"activity_policy"`
	TypeID            string         `json:"type_id"` // optional; defaults to "contributor"/"columnist" lookup
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
	ID                  string          `json:"id"`
	TenantID            string          `json:"tenant_id"`
	UserID              string          `json:"user_id"`
	ProfileID           string          `json:"profile_id"`
	DisclosureMode      string          `json:"disclosure_mode"`
	AutonomyMode        string          `json:"autonomy_mode"`
	VoiceID             string          `json:"voice_id"`
	VoiceName           *string         `json:"voice_name,omitempty"`
	VoicePrompt         *string         `json:"voice_prompt,omitempty"`
	VoicePromptOverride *string         `json:"voice_prompt_override,omitempty"`
	ActivityPolicy      json.RawMessage `json:"activity_policy"`
	Status              string          `json:"status"`
	CreatedBy           string          `json:"created_by"`
	CreatedAt           string          `json:"created_at"`
	UpdatedAt           string          `json:"updated_at"`
	BackingUsername     string          `json:"backing_username"`
	BackingEmail        string          `json:"backing_email"`
	ProfileName         string          `json:"profile_name"`
	ProfileSlug         string          `json:"profile_slug"`
	ProfileTagline      *string         `json:"profile_tagline,omitempty"`
	RecentActivity      []ActivityEntry `json:"recent_activity"`
}

// PersonaListItem is the list-view persona record (fewer joined fields).
// Includes activity stats (total_actions, last_action_at) per §68 spec
// ("list all personas + activity stats").
type PersonaListItem struct {
	ID                  string          `json:"id"`
	TenantID            string          `json:"tenant_id"`
	UserID              string          `json:"user_id"`
	ProfileID           string          `json:"profile_id"`
	DisclosureMode      string          `json:"disclosure_mode"`
	AutonomyMode        string          `json:"autonomy_mode"`
	VoiceID             string          `json:"voice_id"`
	VoiceName           *string         `json:"voice_name,omitempty"`
	VoicePrompt         *string         `json:"voice_prompt,omitempty"`
	VoicePromptOverride *string         `json:"voice_prompt_override,omitempty"`
	ActivityPolicy      json.RawMessage `json:"activity_policy"`
	Status              string          `json:"status"`
	CreatedBy           string          `json:"created_by"`
	CreatedAt           string          `json:"created_at"`
	UpdatedAt           string          `json:"updated_at"`
	BackingUsername     string          `json:"backing_username"`
	ProfileName         string          `json:"profile_name"`
	TotalActions        int64           `json:"total_actions"`
	LastActionAt        *string         `json:"last_action_at,omitempty"`
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
// create the user profile byline, create the ai_personas config row.
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
		if err := validateVoicePrompt(&in.VoicePrompt); err != nil {
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

		// 1. Create the backing user (is_bot = true).
		email := personaEmail(in.Slug, tenantID)
		user, err := s.store.CreatePersonaUser(ctx, q, tenantID, email, in.Slug)
		if err != nil {
			results[i] = ProvisionResult{Input: in, Error: err.Error()}
			return results, fmt.Errorf("personas: input %d: create user: %w", i, err)
		}

		// 2. Create the user profile byline (display_name + bio) for the bot.
		// Professional profiles were removed; the persona byline now comes
		// from the bot's user profile (§33 removal, §68 update).
		var tagline *string
		if in.Tagline != "" {
			t := in.Tagline
			tagline = &t
		}
		displayName := in.Name
		if err := q.UpsertUserProfile(ctx, gen.UpsertUserProfileParams{
			TenantID:    tenantID,
			UserID:      user.ID,
			DisplayName: &displayName,
			Bio:         tagline,
		}); err != nil {
			results[i] = ProvisionResult{Input: in, Error: err.Error()}
			return results, fmt.Errorf("personas: input %d: create user profile: %w", i, err)
		}

		// 3. Resolve voice: if a voice_id is supplied it is a shared voice; otherwise
		// if a voice_prompt is supplied, create one shared voice for this persona.
		var voiceID pgtype.UUID
		if in.VoiceID != "" {
			if err := voiceID.Scan(in.VoiceID); err != nil {
				results[i] = ProvisionResult{Input: in, Error: "invalid voice_id"}
				return results, fmt.Errorf("personas: input %d: invalid voice_id", i)
			}
			if _, err := q.GetPersonaVoiceByID(ctx, gen.GetPersonaVoiceByIDParams{TenantID: tenantID, ID: voiceID}); err != nil {
				results[i] = ProvisionResult{Input: in, Error: "voice not found"}
				return results, fmt.Errorf("personas: input %d: voice not found: %w", i, err)
			}
		} else if in.VoicePrompt != "" {
			voiceSlug := in.Slug + "-voice"
			// If the derived slug already exists in this tenant, append a short
			// numeric suffix to avoid the unique violation (BUG-PV-001).
			for attempt := 1; attempt <= 100; attempt++ {
				if attempt > 1 {
					voiceSlug = fmt.Sprintf("%s-voice-%d", in.Slug, attempt)
				}
				voice, err := q.GetPersonaVoiceBySlug(ctx, gen.GetPersonaVoiceBySlugParams{TenantID: tenantID, Slug: voiceSlug})
				if err != nil {
					if !errors.Is(err, pgx.ErrNoRows) {
						results[i] = ProvisionResult{Input: in, Error: err.Error()}
						return results, fmt.Errorf("personas: input %d: check voice slug: %w", i, err)
					}
					// Slug is free; create the voice.
					voiceName := in.Name + " Voice"
					newVoice, err := q.CreatePersonaVoice(ctx, gen.CreatePersonaVoiceParams{
						TenantID:    tenantID,
						Name:        voiceName,
						Slug:        voiceSlug,
						Description: nil,
						VoicePrompt: in.VoicePrompt,
						IsSystem:    false,
						CreatedBy:   adminUserID,
					})
					if err != nil {
						results[i] = ProvisionResult{Input: in, Error: err.Error()}
						return results, fmt.Errorf("personas: input %d: create voice: %w", i, err)
					}
					voiceID = newVoice.ID
					break
				}
				_ = voice
			}
		}

		// 4. Marshal the activity policy and create the ai_personas config row.
		policyBytes, err := json.Marshal(in.ActivityPolicy)
		if err != nil {
			results[i] = ProvisionResult{Input: in, Error: err.Error()}
			return results, fmt.Errorf("personas: input %d: marshal policy: %w", i, err)
		}
		var voicePromptOverride *string
		if in.VoicePrompt != "" && voiceID.Valid {
			// When a shared voice was created from the prompt, the prompt lives
			// on the voice; do not also store it as the per-persona override.
			voicePromptOverride = nil
		} else if in.VoicePrompt != "" {
			// No shared voice was selected or created, so treat the prompt as
			// the per-persona override.
			vp := in.VoicePrompt
			voicePromptOverride = &vp
		}
		persona, err := s.store.CreateAIPersona(ctx, q, gen.CreateAIPersonaParams{
			TenantID:       tenantID,
			UserID:         user.ID,
			ProfileID:      pgtype.UUID{},
			DisclosureMode: disclosure,
			AutonomyMode:   autonomy,
			VoiceID:        voiceID,
			VoicePrompt:    voicePromptOverride,
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

// maxVoicePromptLen is the maximum allowed length for a persona voice_prompt.
// The §68 schema declares the column as unbounded `text`, but an extremely
// long prompt bloats the row and the AI generation context. This is a
// defensive guard — admins hold persona.manage, but a length cap prevents
// accidental paste-of-document submissions (BUG-PER-010).
const maxVoicePromptLen = 5000

// validateVoicePrompt returns an error if the voice_prompt exceeds the
// maximum allowed length (BUG-PER-010). Empty/nil is allowed.
func validateVoicePrompt(vp *string) error {
	if vp == nil {
		return nil
	}
	if len(*vp) > maxVoicePromptLen {
		return fmt.Errorf("voice_prompt length %d exceeds max %d", len(*vp), maxVoicePromptLen)
	}
	return nil
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
	VoiceID        *string
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
	if err := validateVoicePrompt(in.VoicePrompt); err != nil {
		return PersonaDetail{}, fmt.Errorf("personas: %w: %w", errpkg.ErrInvalid, err)
	}

	var voiceID pgtype.UUID
	if in.VoiceID != nil {
		if *in.VoiceID != "" {
			if err := voiceID.Scan(*in.VoiceID); err != nil {
				return PersonaDetail{}, fmt.Errorf("personas: %w: invalid voice_id: %w", errpkg.ErrInvalid, err)
			}
			if _, err := s.store.GetPersonaVoiceByID(ctx, tenantID, voiceID); err != nil {
				return PersonaDetail{}, fmt.Errorf("personas: %w: voice not found: %w", errpkg.ErrInvalid, err)
			}
		} else {
			// Empty string means "clear the voice reference". Use an all-zeros
			// valid UUID as the sentinel that the UpdateAIPersona query maps to
			// NULL.
			voiceID = pgtype.UUID{Bytes: [16]byte{}, Valid: true}
		}
	}

	params := gen.UpdateAIPersonaParams{
		DisclosureMode: in.DisclosureMode,
		AutonomyMode:   in.AutonomyMode,
		VoiceID:        voiceID,
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
// Shared persona voices (ADR-114)
// ---------------------------------------------------------------------------

// PersonaVoice is the API-facing shared persona voice / tone record.
type PersonaVoice struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	VoicePrompt string `json:"voice_prompt"`
	IsSystem    bool   `json:"is_system"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CreatePersonaVoiceInput is the payload for creating a shared voice.
type CreatePersonaVoiceInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	VoicePrompt string `json:"voice_prompt"`
	IsSystem    bool   `json:"is_system"`
}

// UpdatePersonaVoiceInput is the payload for updating a shared voice.
type UpdatePersonaVoiceInput struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description"`
	VoicePrompt *string `json:"voice_prompt,omitempty"`
	IsSystem    *bool   `json:"is_system,omitempty"`
}

// CreatePersonaVoice creates a new shared persona voice after validating
// required fields and slug length.
func (s *Service) CreatePersonaVoice(ctx context.Context, tenantID, createdBy pgtype.UUID, in CreatePersonaVoiceInput) (PersonaVoice, error) {
	if in.Name == "" || in.Slug == "" {
		return PersonaVoice{}, fmt.Errorf("personas: %w: name and slug are required", errpkg.ErrInvalid)
	}
	if in.VoicePrompt == "" {
		return PersonaVoice{}, fmt.Errorf("personas: %w: voice_prompt is required", errpkg.ErrInvalid)
	}
	if err := validateVoicePrompt(&in.VoicePrompt); err != nil {
		return PersonaVoice{}, fmt.Errorf("personas: %w: %w", errpkg.ErrInvalid, err)
	}
	var desc *string
	if in.Description != "" {
		d := in.Description
		desc = &d
	}
	row, err := s.store.CreatePersonaVoice(ctx, nil, gen.CreatePersonaVoiceParams{
		TenantID:    tenantID,
		Name:        in.Name,
		Slug:        in.Slug,
		Description: desc,
		VoicePrompt: in.VoicePrompt,
		IsSystem:    in.IsSystem,
		CreatedBy:   createdBy,
	})
	if err != nil {
		return PersonaVoice{}, err
	}
	return personaVoiceFromRow(row), nil
}

func personaDetailFromRow(r gen.GetAIPersonaByIDRow) PersonaDetail {
	d := PersonaDetail{
		ID:                  r.ID.String(),
		TenantID:            r.TenantID.String(),
		UserID:              r.UserID.String(),
		ProfileID:           r.ProfileID.String(),
		DisclosureMode:      r.DisclosureMode,
		AutonomyMode:        r.AutonomyMode,
		VoiceID:             uuidOrEmpty(r.VoiceID),
		VoiceName:           r.VoiceName,
		VoicePrompt:         nonEmptyString(r.VoicePrompt),
		VoicePromptOverride: r.VoicePromptOverride,
		ActivityPolicy:      rawJSON(r.ActivityPolicy),
		Status:              r.Status,
		CreatedBy:           r.CreatedBy.String(),
		CreatedAt:           r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		BackingUsername:     r.BackingUsername,
		BackingEmail:        r.BackingEmail,
		ProfileName:         r.ProfileName,
		ProfileSlug:         r.ProfileSlug,
		ProfileTagline:      r.ProfileTagline,
	}
	return d
}

func personaListItemFromRow(r gen.ListAIPersonasRow) PersonaListItem {
	item := PersonaListItem{
		ID:                  r.ID.String(),
		TenantID:            r.TenantID.String(),
		UserID:              r.UserID.String(),
		ProfileID:           r.ProfileID.String(),
		DisclosureMode:      r.DisclosureMode,
		AutonomyMode:        r.AutonomyMode,
		VoiceID:             uuidOrEmpty(r.VoiceID),
		VoiceName:           r.VoiceName,
		VoicePrompt:         nonEmptyString(r.VoicePrompt),
		VoicePromptOverride: r.VoicePromptOverride,
		ActivityPolicy:      rawJSON(r.ActivityPolicy),
		Status:              r.Status,
		CreatedBy:           r.CreatedBy.String(),
		CreatedAt:           r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		BackingUsername:     r.BackingUsername,
		ProfileName:         r.ProfileName,
		TotalActions:        r.TotalActions,
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

// nonEmptyString returns a pointer to s when s is non-empty, otherwise nil.
// Useful for optional text fields that should be omitted from JSON when empty.
func nonEmptyString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// uuidOrEmpty returns the UUID string when valid, otherwise an empty string.
// Used for optional uuid fields (voice_id) so the API emits "" instead of the
// all-zeros UUID literal when no value is set.
func uuidOrEmpty(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return u.String()
}

// personaVoiceFromRow maps a generated persona_voices row to the API DTO.
func personaVoiceFromRow(r gen.PersonaVoice) PersonaVoice {
	return PersonaVoice{
		ID:          r.ID.String(),
		TenantID:    r.TenantID.String(),
		Name:        r.Name,
		Slug:        r.Slug,
		Description: derefOrEmpty(r.Description),
		VoicePrompt: r.VoicePrompt,
		IsSystem:    r.IsSystem,
		CreatedBy:   r.CreatedBy.String(),
		CreatedAt:   r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// derefOrEmpty returns the string pointed to by p, or an empty string if p is nil.
func derefOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// GetPersonaVoice returns a single shared voice by ID.
func (s *Service) GetPersonaVoice(ctx context.Context, tenantID, id pgtype.UUID) (PersonaVoice, error) {
	row, err := s.store.GetPersonaVoiceByID(ctx, tenantID, id)
	if err != nil {
		return PersonaVoice{}, err
	}
	return personaVoiceFromRow(row), nil
}

// GetVoicePrompt returns the voice prompt for a shared voice, but only if the
// voice is system-owned or owned by the calling user. It is exposed as an
// interface method so the AI core handlers can inject the service as a
// PersonaVoiceResolver without importing personas types in the ai package.
// Invisible voices return an empty string with a nil error so the AI handler
// treats them as a no-op override (ADR-114, BUG-009).
func (s *Service) GetVoicePrompt(ctx context.Context, tenantID, userID, voiceID pgtype.UUID) (string, error) {
	voice, err := s.GetPersonaVoice(ctx, tenantID, voiceID)
	if err != nil {
		return "", err
	}
	if !voice.IsSystem && voice.CreatedBy != userID.String() {
		return "", nil
	}
	return voice.VoicePrompt, nil
}

// ListPersonaVoices returns a paginated list of shared voices for a tenant.
func (s *Service) ListPersonaVoices(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]PersonaVoice, int64, error) {
	rows, err := s.store.ListPersonaVoices(ctx, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountPersonaVoices(ctx, tenantID)
	if err != nil {
		return nil, 0, err
	}
	items := make([]PersonaVoice, 0, len(rows))
	for _, r := range rows {
		items = append(items, personaVoiceFromRow(r))
	}
	return items, total, nil
}

// UpdatePersonaVoice updates a shared voice and returns the updated record.
func (s *Service) UpdatePersonaVoice(ctx context.Context, tenantID, id pgtype.UUID, in UpdatePersonaVoiceInput) (PersonaVoice, error) {
	if in.VoicePrompt != nil {
		if err := validateVoicePrompt(in.VoicePrompt); err != nil {
			return PersonaVoice{}, fmt.Errorf("personas: %w: %w", errpkg.ErrInvalid, err)
		}
	}

	var description *string
	if in.Description != nil {
		if *in.Description == "" {
			description = &emptyString
		} else {
			description = in.Description
		}
	}

	params := gen.UpdatePersonaVoiceParams{
		TenantID:    tenantID,
		ID:          id,
		Name:        in.Name,
		Slug:        in.Slug,
		Description: description,
		VoicePrompt: in.VoicePrompt,
		IsSystem:    in.IsSystem,
	}
	row, err := s.store.UpdatePersonaVoice(ctx, params)
	if err != nil {
		return PersonaVoice{}, err
	}
	return personaVoiceFromRow(row), nil
}

// emptyString is a pre-allocated empty string pointer for sqlc COALESCE.
var emptyString = ""

// DeletePersonaVoice deletes a shared voice by ID.
func (s *Service) DeletePersonaVoice(ctx context.Context, tenantID, id pgtype.UUID) error {
	return s.store.DeletePersonaVoice(ctx, tenantID, id)
}
