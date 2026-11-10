// Package personas — service.go holds the business logic for the AI Persona
// System: transactional mass provisioning, CRUD, activity-log retrieval, and
// the high-stakes content gate (ADR-068 §5).
// // Ref: japaconnect-engineering/sections/68-ai-personas.md
// //	japaconnect-engineering/adr/ADR-068-ai-persona-system.md
package personas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
// // Ref: japaconnect-engineering/sections/68-ai-personas.md.
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
	VoiceID        string         `json:"voice_id"`
	VoicePrompt    string         `json:"voice_prompt"`
	DisclosureMode string         `json:"disclosure_mode"`
	AutonomyMode   string         `json:"autonomy_mode"`
	ActivityPolicy ActivityPolicy `json:"activity_policy"`
	TypeID         string         `json:"type_id"`   // optional; defaults to "contributor"/"columnist" lookup
	GeoSpace       string         `json:"geo_space"` // optional space UUID or slug (§68 Geo Space Assignment)
}

// personaEmail builds a deterministic internal email for a persona backing
// account. These accounts never log in (is_bot = true), so the email is an
// internal identifier, not a deliverable address. Slug uniqueness against
// existing accounts is pre-validated in ProvisionPersonas (BUG-001) so a
// collision surfaces as a validation error, not a mid-tx UNIQUE failure.
func personaEmail(slug string) string {
	return fmt.Sprintf("persona+%s+@bot.internal", slug)
}

// PersonaDetail is the API-facing persona record with joined user + profile
// names, the decoded activity_policy, and recent actions (§68 spec:
// "persona detail + activity_policy + recent actions").
type PersonaDetail struct {
	ID string `json:"id"`

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
	GeoSpaceID          string          `json:"geo_space_id"`
	GeoSpaceName        *string         `json:"geo_space_name,omitempty"`
	GeoSpaceSlug        *string         `json:"geo_space_slug,omitempty"`
	GeoSpaceLevel       *string         `json:"geo_space_level,omitempty"`

	// Profile fields (user_profiles) — Plan B admin editing surface.
	AvatarURL     *string  `json:"avatar_url,omitempty"`
	CoverURL      *string  `json:"cover_url,omitempty"`
	AvatarMediaID string   `json:"avatar_media_id"`
	HeaderMediaID string   `json:"header_media_id"`
	Profession    *string  `json:"profession,omitempty"`
	Location      *string  `json:"location,omitempty"`
	WebsiteURL    *string  `json:"website_url,omitempty"`
	Education     *string  `json:"education,omitempty"`
	Interests     []string `json:"interests,omitempty"`

	// Economy — tips_enabled from tip_settings(owner_type='user'), points
	// balance from wallet_ledger(ledger_type='points') on the backing user.
	TipsEnabled   bool  `json:"tips_enabled"`
	PointsBalance int64 `json:"points_balance"`

	RecentActivity []ActivityEntry `json:"recent_activity"`
}

// PersonaListItem is the list-view persona record (fewer joined fields).
// Includes activity stats (total_actions, last_action_at) per §68 spec
// ("list all personas + activity stats").
type PersonaListItem struct {
	ID string `json:"id"`

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
	ProfileTagline      *string         `json:"profile_tagline,omitempty"`
	GeoSpaceID          string          `json:"geo_space_id"`
	GeoSpaceName        *string         `json:"geo_space_name,omitempty"`
	GeoSpaceSlug        *string         `json:"geo_space_slug,omitempty"`
	GeoSpaceLevel       *string         `json:"geo_space_level,omitempty"`

	// Profile + economy fields mirror PersonaDetail so the admin table can
	// render avatars/tips/points without a per-row detail fetch.
	AvatarURL     *string  `json:"avatar_url,omitempty"`
	CoverURL      *string  `json:"cover_url,omitempty"`
	AvatarMediaID string   `json:"avatar_media_id"`
	HeaderMediaID string   `json:"header_media_id"`
	Profession    *string  `json:"profession,omitempty"`
	Location      *string  `json:"location,omitempty"`
	WebsiteURL    *string  `json:"website_url,omitempty"`
	Education     *string  `json:"education,omitempty"`
	Interests     []string `json:"interests,omitempty"`
	TipsEnabled   bool     `json:"tips_enabled"`
	PointsBalance int64    `json:"points_balance"`

	TotalActions int64   `json:"total_actions"`
	LastActionAt *string `json:"last_action_at,omitempty"`
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
	store         *Store
	settings      *config.Settings
	enqueuer      TaskEnqueuer
	mediaResolver MediaResolver
}

// MediaResolver resolves a media UUID to a presigned URL and its stored MIME
// type — same contract as users.MediaResolver; wired at server time. A nil
// resolver stores media ids without resolved URLs and skips MIME checks.
type MediaResolver interface {
	ResolveMediaURL(ctx context.Context, mediaID pgtype.UUID) (string, error)
	ResolveMediaMIME(ctx context.Context, mediaID pgtype.UUID) (string, error)
}

// NewService creates a personas service.
func NewService(store *Store, settings *config.Settings, enqueuer TaskEnqueuer) *Service {
	return &Service{store: store, settings: settings, enqueuer: enqueuer}
}

// SetEnqueuer wires (or rewires) the Asynq enqueuer after construction.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetMediaResolver wires the media resolver used for avatar/cover assignment.
func (s *Service) SetMediaResolver(r MediaResolver) { s.mediaResolver = r }

// mergeMediaID merges a PATCH-supplied media id into the profile row: ""
// clears the reference (uuid zero-value), a value is validated as a UUID then
// MIME-checked (image; avatars additionally obey users.allowed_avatar_types)
// and its URL resolved for the sibling *_url column. The returned URL is ""
// when clearing or when resolution is unavailable — callers leave the merged
// url column untouched in that case.
func (s *Service) mergeMediaID(ctx context.Context, raw, field string, isAvatar bool) (pgtype.UUID, string, error) {
	var zero pgtype.UUID
	if strings.TrimSpace(raw) == "" {
		return zero, "", nil
	}
	var id pgtype.UUID
	if err := id.Scan(strings.TrimSpace(raw)); err != nil || !id.Valid {
		return zero, "", fmt.Errorf("personas: %s must be a media UUID: %w", field, errpkg.ErrInvalid)
	}
	if mime := s.resolveMediaMIME(ctx, id); mime != "" {
		if !strings.HasPrefix(mime, "image/") {
			return zero, "", fmt.Errorf("personas: %s must be an image: %w", field, errpkg.ErrInvalid)
		}
		if isAvatar {
			if err := s.checkAvatarMIMEAllowed(ctx, mime); err != nil {
				return zero, "", err
			}
		}
	}
	url, err := s.resolveMediaURL(ctx, id)
	if err != nil {
		return zero, "", fmt.Errorf("personas: resolve %s url: %w", field, err)
	}
	return id, url, nil
}

// resolveMediaURL resolves a media row's presigned URL. A nil resolver leaves
// the media id stored with an empty url column — the FK still validates the id.
func (s *Service) resolveMediaURL(ctx context.Context, mediaID pgtype.UUID) (string, error) {
	if s.mediaResolver == nil {
		return "", nil
	}
	return s.mediaResolver.ResolveMediaURL(ctx, mediaID)
}

// resolveMediaMIME fetches a media row's stored MIME type; "" means "cannot
// determine" (nil resolver, missing row, unset type) and fails open.
func (s *Service) resolveMediaMIME(ctx context.Context, mediaID pgtype.UUID) string {
	if s.mediaResolver == nil {
		return ""
	}
	mimeType, err := s.mediaResolver.ResolveMediaMIME(ctx, mediaID)
	if err != nil || mimeType == "" {
		return ""
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "image/jpg" || mimeType == "image/pjpeg" {
		mimeType = "image/jpeg"
	}
	return mimeType
}

// avatarExtMIME maps the extension tokens allowed by users.allowed_avatar_types
// to the canonical media MIME type they permit — mirrors users.Service.
var avatarExtMIME = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png",
	"gif": "image/gif", "webp": "image/webp", "svg": "image/svg+xml",
}

// checkAvatarMIMEAllowed enforces the users.allowed_avatar_types CSV when
// configured; an empty/unset list permits every image MIME.
func (s *Service) checkAvatarMIMEAllowed(ctx context.Context, mimeType string) error {
	if s.settings == nil {
		return nil
	}
	csv := strings.TrimSpace(s.settings.GetStringOr(ctx, "users.allowed_avatar_types", ""))
	if csv == "" {
		return nil
	}
	for _, ext := range strings.Split(csv, ",") {
		ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
		if m, ok := avatarExtMIME[ext]; ok && m == mimeType {
			return nil
		}
	}
	return fmt.Errorf("personas: avatar type %s is not in users.allowed_avatar_types: %w", mimeType, errpkg.ErrInvalid)
}

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
// // The admin user ID is recorded as created_by on every ai_personas row.
// // The batch size is capped by the persona.max_batch_size system setting
// (BUG-006: an unbounded batch holds a long write transaction and can DoS
// the writer pool). Default cap is 100 when the setting is unset.
func (s *Service) ProvisionPersonas(ctx context.Context, adminUserID pgtype.UUID, inputs []PersonaInput) ([]ProvisionResult, error) {
	if len(inputs) == 0 {
		return nil, errors.New("personas: no personas to provision")
	}

	// BUG-PER-006: audit the mass-provisioning action — a privileged bulk
	// identity creation must leave an audit_logs trail (mirrors the OAuth
	// grant lifecycle audit posture).
	defer s.writeAudit(ctx, adminUserID, "persona.provision", "", map[string]any{"batch_size": len(inputs)})

	// BUG-006: cap the batch size via system_settings (Rule 3: configurable).
	maxBatch := 100
	if s.settings != nil {
		maxBatch = s.settings.GetIntOr(ctx, "persona.max_batch_size", 100)
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
	// the second CreatePersonaUser hits a UNIQUE( email) constraint
	// violation inside the tx and the admin gets a generic "provisioning
	// failed" error with no indication of which slug collided.
	seenSlugs := make(map[string]int, len(inputs))
	for i, in := range inputs {
		if prev, dup := seenSlugs[in.Slug]; dup {
			return nil, fmt.Errorf("personas: duplicate slug %q in batch (inputs %d and %d): %w", in.Slug, prev, i, errpkg.ErrInvalid)
		}

		seenSlugs[in.Slug] = i
	}

	// BUG-001: pre-validate slugs against existing rows. Without this, a slug
	// that already exists in users hits UNIQUE(email)/UNIQUE(username)
	// mid-transaction and the admin gets a generic "provisioning_failed"
	// with no indication of which slug collided.
	slugs := make([]string, 0, len(inputs))
	for _, in := range inputs {
		slugs = append(slugs, in.Slug)
	}

	rows, err := s.store.DB().Reader.Query(ctx,
		`SELECT username FROM users WHERE username = ANY($1)`, slugs)
	if err != nil {
		return nil, fmt.Errorf("personas: checking existing slugs: %w", err)
	}

	var taken []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return nil, fmt.Errorf("personas: scanning existing slugs: %w", err)
		}
		taken = append(taken, u)
	}
	rows.Close()

	if len(taken) > 0 {
		return nil, fmt.Errorf("personas: slug(s) already in use: %s: %w", strings.Join(taken, ", "), errpkg.ErrInvalid)
	}

	// Resolve geo_space refs up front so an unknown slug / non-geo space
	// fails fast as validation_error instead of a mid-tx
	// provisioning_failed (mirrors the slug pre-check above).
	resolvedGeo := make([]pgtype.UUID, len(inputs))
	for i, in := range inputs {
		if strings.TrimSpace(in.GeoSpace) == "" {
			continue
		}

		spaceID, err := s.resolveGeoSpace(ctx, gen.New(s.store.DB().Reader), in.GeoSpace)
		if err != nil {
			return nil, fmt.Errorf("personas: input %d: %w", i, err)
		}

		resolvedGeo[i] = spaceID
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
		// Apply defaults for empty modes. Personas are staff-curated bot
		// accounts — disclosure defaults OFF (undisclosed); opt back into a
		// visible AI label with disclosure_mode='disclosed'.
		disclosure := in.DisclosureMode
		if disclosure == "" {
			disclosure = DisclosureModeUndisclosed
		}

		autonomy := in.AutonomyMode
		if autonomy == "" {
			autonomy = AutonomyModeQueued
		}

		// 1. Create the backing user (is_bot = true).
		email := personaEmail(in.Slug)

		user, err := s.store.CreatePersonaUser(ctx, q, email, in.Slug)
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
			UserID:      user.ID,
			DisplayName: &displayName,
			Bio:         tagline,
		}); err != nil {
			results[i] = ProvisionResult{Input: in, Error: err.Error()}
			return results, fmt.Errorf("personas: input %d: create user profile: %w", i, err)
		}

		// 2b. Geo space assignment (§68): join the backing user to the geo
		// chain and set current_geo_space_id so the persona profile shows
		// Spaces + the primary badge like a real profile.
		if resolvedGeo[i].Valid {
			chain, err := geoChain(ctx, q, resolvedGeo[i])
			if err != nil {
				results[i] = ProvisionResult{Input: in, Error: err.Error()}
				return results, fmt.Errorf("personas: input %d: geo chain: %w", i, err)
			}

			if err := s.applyGeoSpace(ctx, q, user.ID, chain); err != nil {
				results[i] = ProvisionResult{Input: in, Error: err.Error()}
				return results, fmt.Errorf("personas: input %d: apply geo space: %w", i, err)
			}
		}

		// 3. Resolve voice: if a voice_id is supplied it is a shared voice; otherwise
		// if a voice_prompt is supplied, create one shared voice for this persona.
		var voiceID pgtype.UUID
		if in.VoiceID != "" {
			if err := voiceID.Scan(in.VoiceID); err != nil {
				results[i] = ProvisionResult{Input: in, Error: "invalid voice_id"}
				return results, fmt.Errorf("personas: input %d: invalid voice_id", i)
			}

			if _, err := q.GetPersonaVoiceByID(ctx, voiceID); err != nil {
				results[i] = ProvisionResult{Input: in, Error: "voice not found"}
				return results, fmt.Errorf("personas: input %d: voice not found: %w", i, err)
			}
		} else if in.VoicePrompt != "" {
			voiceSlug := in.Slug + "-voice"
			// If the derived slug already exists in this platform, append a short
			// numeric suffix to avoid the unique violation (BUG-PV-001).
			for attempt := 1; attempt <= 100; attempt++ {
				if attempt > 1 {
					voiceSlug = fmt.Sprintf("%s-voice-%d", in.Slug, attempt)
				}

				voice, err := q.GetPersonaVoiceBySlug(ctx, voiceSlug)
				if err != nil {
					if !errors.Is(err, pgx.ErrNoRows) {
						results[i] = ProvisionResult{Input: in, Error: err.Error()}
						return results, fmt.Errorf("personas: input %d: check voice slug: %w", i, err)
					}
					// Slug is free; create the voice.
					voiceName := in.Name + " Voice"

					newVoice, err := q.CreatePersonaVoice(ctx, gen.CreatePersonaVoiceParams{
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
	if (start != 0 || end != 0) && start >= end {
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
func (s *Service) GetPersona(ctx context.Context, id pgtype.UUID) (PersonaDetail, error) {
	row, err := s.store.GetAIPersonaByID(ctx, id)
	if err != nil {
		return PersonaDetail{}, err
	}

	detail := personaDetailFromRow(row)

	// Fetch the 5 most recent synthetic platform_events for the persona's
	// backing user (§68 "recent actions" in the detail response). Use the
	// raw UUID from the row (detail.UserID is a string for JSON output).
	recentRows, err := s.store.ListPersonaActivity(ctx, row.UserID, 5, 0)
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

// ListPersonas returns a paginated list of personas for a platform. An empty
// status returns all statuses; an empty q matches all rows; sort/dir default
// to created_at DESC.
func (s *Service) ListPersonas(ctx context.Context, status, q, sort, dir string, limit, offset int32) ([]PersonaListItem, int64, error) {
	rows, err := s.store.ListAIPersonas(ctx, status, q, sort, dir, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountAIPersonas(ctx, status, q)
	if err != nil {
		return nil, 0, err
	}

	items := make([]PersonaListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, personaListItemFromRow(r))
	}

	return items, total, nil
}

// DrivablePersona is the slim /me/personas payload — the fields the
// frontend "Interact as" picker needs (spec §1.8).
type DrivablePersona struct {
	ID           pgtype.UUID `json:"id"`
	UserID       pgtype.UUID `json:"user_id"`
	Name         string      `json:"name"`
	Username     string      `json:"username"`
	AvatarURL    *string     `json:"avatar_url,omitempty"`
	GeoSpaceID   pgtype.UUID `json:"geo_space_id"`
	GeoSpaceName *string     `json:"geo_space_name,omitempty"`
}

// ListDrivablePersonas returns the active personas an operator may drive
// via author_id/actor_id delegation.
func (s *Service) ListDrivablePersonas(ctx context.Context) ([]DrivablePersona, error) {
	rows, err := s.store.ListActivePersonas(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]DrivablePersona, 0, len(rows))
	for _, r := range rows {
		items = append(items, DrivablePersona{
			ID:           r.ID,
			UserID:       r.UserID,
			Name:         r.Name,
			Username:     r.Username,
			AvatarURL:    r.AvatarUrl,
			GeoSpaceID:   r.GeoSpaceID,
			GeoSpaceName: r.GeoSpaceName,
		})
	}

	return items, nil
}

// UpdatePersonaInput holds the optional fields for a persona update.
type UpdatePersonaInput struct {
	DisclosureMode *string
	AutonomyMode   *string
	VoiceID        *string
	VoicePrompt    *string
	ActivityPolicy *ActivityPolicy
	Status         *string
	// Identity fields — land on the backing bot account, not ai_personas:
	// Name/Tagline write user_profiles.display_name/bio (the public byline);
	// Username renames users.username (the handle intel-* bots and
	// provisioned persona+ slugs derive from). Empty-string Name/Tagline
	// clears the field (display_name NULL → byline falls back to username).
	Name     *string
	Tagline  *string
	Username *string
	// GeoSpace replaces the backing user's geo membership chain (§68 Geo
	// Space Assignment). Nil = unchanged, "" = clear, value = set/replace.
	GeoSpace *string
	// Profile fields — land on user_profiles like Name/Tagline. Nil =
	// unchanged, "" = clear, value = set. AvatarMediaID/HeaderMediaID take
	// media UUIDs ("" = clear); the resolver fills the *_url columns.
	AvatarURL     *string
	AvatarMediaID *string
	CoverURL      *string
	HeaderMediaID *string
	Profession    *string
	Location      *string
	WebsiteURL    *string
	Education     *string
	// Interests nil = unchanged, [] = clear, [values] = replace.
	Interests *[]string
	// TipsEnabled upserts tip_settings for the backing user in the same tx —
	// existing accept_* configuration is preserved via read-merge-write.
	TipsEnabled *bool
}

// UpdatePersona updates a persona's config (activity_policy, modes, status,
// voice_prompt). Nil/empty fields are left unchanged. Mode and status values
// are validated up front so invalid input returns a clean 400 validation_error
// instead of a raw CHECK-constraint 500 (BUG-001, Rule 5).
// // Returns a PersonaDetail (with joined user + profile names) so the response
// shape matches HandleGet (BUG-004: previously returned the raw
// gen.AiPersona row, inconsistent with the rest of the API).
func (s *Service) UpdatePersona(ctx context.Context, id pgtype.UUID, in UpdatePersonaInput, actors ...pgtype.UUID) (PersonaDetail, error) {
	// Audit trail — provisioning writes persona.provision; updates deserve
	// the same posture (renames are privileged identity changes). Records
	// the field names touched, not their values.
	changed := make([]string, 0, 9)
	for field, set := range map[string]bool{
		"disclosure_mode": in.DisclosureMode != nil,
		"autonomy_mode":   in.AutonomyMode != nil,
		"voice_id":        in.VoiceID != nil,
		"voice_prompt":    in.VoicePrompt != nil,
		"activity_policy": in.ActivityPolicy != nil,
		"status":          in.Status != nil,
		"name":            in.Name != nil,
		"tagline":         in.Tagline != nil,
		"username":        in.Username != nil,
		"geo_space":       in.GeoSpace != nil,
		"avatar_url":      in.AvatarURL != nil,
		"avatar_media_id": in.AvatarMediaID != nil,
		"cover_url":       in.CoverURL != nil,
		"header_media_id": in.HeaderMediaID != nil,
		"profession":      in.Profession != nil,
		"location":        in.Location != nil,
		"website_url":     in.WebsiteURL != nil,
		"education":       in.Education != nil,
		"interests":       in.Interests != nil,
		"tips_enabled":    in.TipsEnabled != nil,
	} {
		if set {
			changed = append(changed, field)
		}
	}
	if len(actors) > 0 {
		defer s.writeAudit(ctx, actors[0], "persona.update", id.String(), map[string]any{"fields": changed})
	}

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

			if _, err := s.store.GetPersonaVoiceByID(ctx, voiceID); err != nil {
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

		ID: id,
	}

	if in.ActivityPolicy != nil {
		b, err := json.Marshal(*in.ActivityPolicy)
		if err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: marshal activity policy: %w", err)
		}

		params.ActivityPolicy = b
	}

	// Identity edits (byline + handle + profile fields + tips) ride one tx
	// with the config update so a persona can never end up half-renamed —
	// mirrors the provisioning transaction boundary (ADR-068 §7).
	profileTouched := in.Name != nil || in.Tagline != nil ||
		in.AvatarURL != nil || in.AvatarMediaID != nil ||
		in.CoverURL != nil || in.HeaderMediaID != nil ||
		in.Profession != nil || in.Location != nil ||
		in.WebsiteURL != nil || in.Education != nil || in.Interests != nil
	identityTouched := profileTouched || in.Username != nil || in.GeoSpace != nil || in.TipsEnabled != nil
	if !identityTouched {
		if _, err := s.store.UpdateAIPersona(ctx, params); err != nil {
			return PersonaDetail{}, err
		}

		return s.GetPersona(ctx, id)
	}

	// Resolve the backing user — the detail fetch doubles as the not-found
	// check and supplies the current handle for the rename comparison.
	detail, err := s.store.GetAIPersonaByID(ctx, id)
	if err != nil {
		return PersonaDetail{}, err
	}

	newUsername := ""
	if in.Username != nil {
		if un := strings.TrimSpace(*in.Username); un != "" && un != detail.BackingUsername {
			if !personaUsernamePattern.MatchString(un) {
				return PersonaDetail{}, fmt.Errorf("personas: username must be 3-30 characters of letters, numbers, and underscores: %w", errpkg.ErrInvalid)
			}
			newUsername = un
		}
	}

	tx, err := s.store.DB().Writer.Begin(ctx)
	if err != nil {
		return PersonaDetail{}, fmt.Errorf("personas: begin update tx: %w", err)
	}
	defer tx.Rollback(ctx) // safe to call after Commit (no-op)

	q := gen.New(tx)

	if _, err := q.UpdateAIPersona(ctx, params); err != nil {
		return PersonaDetail{}, fmt.Errorf("personas: update ai persona: %w", err)
	}

	if newUsername != "" {
		taken, err := q.UsernameExists(ctx, gen.UsernameExistsParams{Username: newUsername, ID: detail.UserID})
		if err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: username availability check: %w", err)
		}
		if taken > 0 {
			return PersonaDetail{}, fmt.Errorf("personas: username %q is taken: %w", newUsername, errpkg.ErrDuplicate)
		}
		if err := q.UpdateUserUsername(ctx, gen.UpdateUserUsernameParams{ID: detail.UserID, Username: newUsername}); err != nil {
			// The UsernameExists check above is advisory — the UNIQUE index is
			// the real guard. Map a raced/case-collided 23505 to the same 409
			// so callers get a clean already_exists instead of a 500.
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return PersonaDetail{}, fmt.Errorf("personas: username %q is taken: %w", newUsername, errpkg.ErrDuplicate)
			}
			return PersonaDetail{}, fmt.Errorf("personas: rename backing user: %w", err)
		}

		// Email follows the handle — the persona+<slug>+@bot.internal
		// convention would otherwise diverge on every rename (legacy intel-*
		// emails migrate to the convention on first rename).
		newEmail := personaEmail(newUsername)
		if newEmail != detail.BackingEmail {
			etaken, err := q.EmailExists(ctx, gen.EmailExistsParams{Email: newEmail, ID: detail.UserID})
			if err != nil {
				return PersonaDetail{}, fmt.Errorf("personas: email availability check: %w", err)
			}
			if etaken > 0 {
				return PersonaDetail{}, fmt.Errorf("personas: backing email %q is taken: %w", newEmail, errpkg.ErrDuplicate)
			}
			if err := q.UpdateUserEmail(ctx, gen.UpdateUserEmailParams{ID: detail.UserID, Email: newEmail}); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					return PersonaDetail{}, fmt.Errorf("personas: backing email %q is taken: %w", newEmail, errpkg.ErrDuplicate)
				}
				return PersonaDetail{}, fmt.Errorf("personas: rename backing email: %w", err)
			}
		}
	}

	if profileTouched {
		// Read-merge-write against the row-locked profile — ErrNoRows means
		// the bot predates the byline write (legacy intel_* seeds) and the
		// upsert mints the row. Every profile column is merged here so the
		// unconditional UpsertPersonaProfile write never clobbers fields the
		// PATCH didn't touch.
		var params gen.UpsertPersonaProfileParams
		profile, err := q.GetUserProfileForUpdate(ctx, detail.UserID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return PersonaDetail{}, fmt.Errorf("personas: lock byline profile: %w", err)
		default:
			params = gen.UpsertPersonaProfileParams{
				DisplayName:   profile.DisplayName,
				Bio:           profile.Bio,
				AvatarUrl:     profile.AvatarUrl,
				AvatarMediaID: profile.AvatarMediaID,
				HeaderMediaID: profile.HeaderMediaID,
				CoverUrl:      profile.CoverUrl,
				Location:      profile.Location,
				WebsiteUrl:    profile.WebsiteUrl,
				Education:     profile.Education,
				Profession:    profile.Profession,
				Interests:     profile.Interests,
			}
		}

		mergeText := func(dst **string, in *string) {
			*dst = nil
			if v := strings.TrimSpace(*in); v != "" {
				*dst = &v
			}
		}
		if in.Name != nil {
			mergeText(&params.DisplayName, in.Name)
		}
		if in.Tagline != nil {
			mergeText(&params.Bio, in.Tagline)
		}
		if in.Profession != nil {
			mergeText(&params.Profession, in.Profession)
		}
		if in.Location != nil {
			mergeText(&params.Location, in.Location)
		}
		if in.WebsiteURL != nil {
			mergeText(&params.WebsiteUrl, in.WebsiteURL)
		}
		if in.Education != nil {
			mergeText(&params.Education, in.Education)
		}
		if in.AvatarURL != nil {
			mergeText(&params.AvatarUrl, in.AvatarURL)
		}
		if in.CoverURL != nil {
			mergeText(&params.CoverUrl, in.CoverURL)
		}
		if in.Interests != nil {
			params.Interests = *in.Interests
		}

		if in.AvatarMediaID != nil {
			mid, url, err := s.mergeMediaID(ctx, *in.AvatarMediaID, "avatar_media_id", true)
			if err != nil {
				return PersonaDetail{}, err
			}
			params.AvatarMediaID = mid
			if url != "" {
				params.AvatarUrl = &url
			}
		}
		if in.HeaderMediaID != nil {
			mid, url, err := s.mergeMediaID(ctx, *in.HeaderMediaID, "header_media_id", false)
			if err != nil {
				return PersonaDetail{}, err
			}
			params.HeaderMediaID = mid
			if url != "" {
				params.CoverUrl = &url
			}
		}

		params.UserID = detail.UserID
		if err := q.UpsertPersonaProfile(ctx, params); err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: upsert profile: %w", err)
		}
	}

	if in.TipsEnabled != nil {
		// Read-merge-write: UpsertTipSettings writes accept_* unconditionally,
		// so an existing row's configuration must be carried forward —
		// first-upsert defaults mirror the tips admin defaults (credits only).
		acceptCredits, acceptFiat, acceptExternal := true, false, false
		var minTip int64
		var ocSlug, ocToken, thanks *string
		if existing, err := q.GetTipSettings(ctx, gen.GetTipSettingsParams{OwnerType: "user", OwnerID: detail.UserID}); err == nil {
			acceptCredits = existing.AcceptCredits
			acceptFiat = existing.AcceptFiat
			acceptExternal = existing.AcceptExternal
			minTip = existing.MinTipAmount
			ocSlug = existing.OcCollectiveSlug
			ocToken = existing.OcApiTokenEncrypted
			thanks = existing.ThankYouMessage
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return PersonaDetail{}, fmt.Errorf("personas: load tip settings: %w", err)
		}
		if _, err := q.UpsertTipSettings(ctx, gen.UpsertTipSettingsParams{
			OwnerType: "user", OwnerID: detail.UserID, TipsEnabled: *in.TipsEnabled,
			AcceptCredits: acceptCredits, AcceptFiat: acceptFiat, AcceptExternal: acceptExternal,
			OcCollectiveSlug: ocSlug, OcApiTokenEncrypted: ocToken,
			MinTipAmount: minTip, ThankYouMessage: thanks,
		}); err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: upsert tip settings: %w", err)
		}
	}

	if in.GeoSpace != nil {
		if err := s.replaceGeoSpace(ctx, q, detail.UserID, *in.GeoSpace); err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: replace geo space: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return PersonaDetail{}, fmt.Errorf("personas: commit update tx: %w", err)
	}

	// Re-fetch the joined row so the response includes user + profile names
	// (BUG-004: consistent with HandleGet's PersonaDetail shape).
	return s.GetPersona(ctx, id)
}

// personaUsernamePattern extends auth.ValidateUsername with hyphens — the
// seeded bot conventions (intel-visa, ci-changelog, persona+ slugs) are
// hyphenated, so the admin rename rule must accept them (members still
// can't pick hyphens via the stricter signup validator).
var personaUsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,30}$`)

// GetPersonaActivity returns a paginated activity log (platform_events) for a
// persona's backing user.
func (s *Service) GetPersonaActivity(ctx context.Context, personaID pgtype.UUID, limit, offset int32) ([]ActivityEntry, int64, error) {
	detail, err := s.store.GetAIPersonaByID(ctx, personaID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.store.ListPersonaActivity(ctx, detail.UserID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountPersonaActivity(ctx, detail.UserID)
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
// // The canonical implementation lives in the participation package
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
	ID string `json:"id"`

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

// voiceSlugPattern mirrors the space-slug convention — lowercase letters,
// digits, and hyphens, 2–64 chars.
var voiceSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// CreatePersonaVoice creates a new shared persona voice after validating
// required fields and slug length.
func (s *Service) CreatePersonaVoice(ctx context.Context, createdBy pgtype.UUID, in CreatePersonaVoiceInput) (PersonaVoice, error) {
	if in.Name == "" || in.Slug == "" {
		return PersonaVoice{}, fmt.Errorf("personas: %w: name and slug are required", errpkg.ErrInvalid)
	}

	if !voiceSlugPattern.MatchString(in.Slug) {
		return PersonaVoice{}, fmt.Errorf("personas: %w: slug must be 2-64 characters of lowercase letters, numbers, and hyphens", errpkg.ErrInvalid)
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
		Name:        in.Name,
		Slug:        in.Slug,
		Description: desc,
		VoicePrompt: in.VoicePrompt,
		IsSystem:    in.IsSystem,
		CreatedBy:   createdBy,
	})
	if err != nil {
		if errpkg.IsUniqueViolation(err) {
			return PersonaVoice{}, fmt.Errorf("personas: %w: a voice with slug %q already exists", errpkg.ErrDuplicate, in.Slug)
		}

		return PersonaVoice{}, err
	}

	s.writeAudit(ctx, createdBy, "persona.voice.create", row.ID.String(), map[string]any{"slug": in.Slug})

	return personaVoiceFromRow(row), nil
}

func personaDetailFromRow(r gen.GetAIPersonaByIDRow) PersonaDetail {
	d := PersonaDetail{
		ID: r.ID.String(),

		UserID:              r.UserID.String(),
		ProfileID:           uuidOrEmpty(r.ProfileID), // BUG-004: emit "" not a zero UUID
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
		GeoSpaceID:          uuidOrEmpty(r.GeoSpaceID),
		GeoSpaceName:        r.GeoSpaceName,
		GeoSpaceSlug:        r.GeoSpaceSlug,
		GeoSpaceLevel:       r.GeoSpaceLevel,
		AvatarURL:           r.AvatarUrl,
		CoverURL:            r.CoverUrl,
		AvatarMediaID:       uuidOrEmpty(r.AvatarMediaID),
		HeaderMediaID:       uuidOrEmpty(r.HeaderMediaID),
		Profession:          r.Profession,
		Location:            r.Location,
		WebsiteURL:          r.WebsiteUrl,
		Education:           r.Education,
		Interests:           r.Interests,
		TipsEnabled:         r.TipsEnabled,
		PointsBalance:       r.PointsBalance,
	}

	return d
}

func personaListItemFromRow(r gen.ListAIPersonasRow) PersonaListItem {
	item := PersonaListItem{
		ID: r.ID.String(),

		UserID:              r.UserID.String(),
		ProfileID:           uuidOrEmpty(r.ProfileID), // BUG-004: emit "" not a zero UUID
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
		ProfileTagline:      r.ProfileTagline,
		GeoSpaceID:          uuidOrEmpty(r.GeoSpaceID),
		GeoSpaceName:        r.GeoSpaceName,
		GeoSpaceSlug:        r.GeoSpaceSlug,
		GeoSpaceLevel:       r.GeoSpaceLevel,
		AvatarURL:           r.AvatarUrl,
		CoverURL:            r.CoverUrl,
		AvatarMediaID:       uuidOrEmpty(r.AvatarMediaID),
		HeaderMediaID:       uuidOrEmpty(r.HeaderMediaID),
		Profession:          r.Profession,
		Location:            r.Location,
		WebsiteURL:          r.WebsiteUrl,
		Education:           r.Education,
		Interests:           r.Interests,
		TipsEnabled:         r.TipsEnabled,
		PointsBalance:       r.PointsBalance,
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
		ID: r.ID.String(),

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
func (s *Service) GetPersonaVoice(ctx context.Context, id pgtype.UUID) (PersonaVoice, error) {
	row, err := s.store.GetPersonaVoiceByID(ctx, id)
	if err != nil {
		return PersonaVoice{}, err
	}

	return personaVoiceFromRow(row), nil
}

// GetPersonaVoiceBySlug returns a single shared voice by slug.
func (s *Service) GetPersonaVoiceBySlug(ctx context.Context, slug string) (PersonaVoice, error) {
	row, err := s.store.GetPersonaVoiceBySlug(ctx, slug)
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
func (s *Service) GetVoicePrompt(ctx context.Context, userID, voiceID pgtype.UUID) (string, error) {
	voice, err := s.GetPersonaVoice(ctx, voiceID)
	if err != nil {
		return "", err
	}

	if !voice.IsSystem && voice.CreatedBy != userID.String() {
		return "", nil
	}

	return voice.VoicePrompt, nil
}

// PersonaVoiceOption is the slim {id, name} picker shape — no voice_prompt.
type PersonaVoiceOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListPersonaVoiceOptions returns the slim picker projection of shared
// voices — the multi-KB voice_prompt rows picker consumers never read are
// skipped at the query level (GET /ai/voices?slim=1).
func (s *Service) ListPersonaVoiceOptions(ctx context.Context, limit, offset int32) ([]PersonaVoiceOption, int64, error) {
	rows, err := s.store.ListPersonaVoiceOptions(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountPersonaVoices(ctx)
	if err != nil {
		return nil, 0, err
	}

	items := make([]PersonaVoiceOption, 0, len(rows))
	for _, r := range rows {
		items = append(items, PersonaVoiceOption{ID: r.ID.String(), Name: r.Name})
	}

	return items, total, nil
}

// ListPersonaVoices returns a paginated list of shared voices for a platform.
func (s *Service) ListPersonaVoices(ctx context.Context, limit, offset int32) ([]PersonaVoice, int64, error) {
	rows, err := s.store.ListPersonaVoices(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountPersonaVoices(ctx)
	if err != nil {
		return nil, 0, err
	}

	items := make([]PersonaVoice, 0, len(rows))
	for _, r := range rows {
		items = append(items, personaVoiceFromRow(r))
	}

	return items, total, nil
}

// SearchPersonaVoices returns a paginated admin list of shared voices with an
// optional ILIKE search (name/slug/description) and a whitelisted sort key.
// Used by GET /admin/persona-voices; sort/dir are pre-validated in the handler.
func (s *Service) SearchPersonaVoices(ctx context.Context, q, sort, dir string, limit, offset int32) ([]PersonaVoice, int64, error) {
	rows, err := s.store.SearchPersonaVoices(ctx, q, sort, dir, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountPersonaVoicesMatching(ctx, q)
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
func (s *Service) UpdatePersonaVoice(ctx context.Context, id pgtype.UUID, in UpdatePersonaVoiceInput, actors ...pgtype.UUID) (PersonaVoice, error) {
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

	if in.Slug != nil && !voiceSlugPattern.MatchString(*in.Slug) {
		return PersonaVoice{}, fmt.Errorf("personas: %w: slug must be 2-64 characters of lowercase letters, numbers, and hyphens", errpkg.ErrInvalid)
	}

	params := gen.UpdatePersonaVoiceParams{
		ID:          id,
		Name:        in.Name,
		Slug:        in.Slug,
		Description: description,
		VoicePrompt: in.VoicePrompt,
		IsSystem:    in.IsSystem,
	}

	row, err := s.store.UpdatePersonaVoice(ctx, params)
	if err != nil {
		if errpkg.IsUniqueViolation(err) {
			return PersonaVoice{}, fmt.Errorf("personas: %w: a voice with slug %q already exists", errpkg.ErrDuplicate, *in.Slug)
		}

		return PersonaVoice{}, err
	}

	s.writeAudit(ctx, firstActor(actors), "persona.voice.update", id.String(), nil)

	return personaVoiceFromRow(row), nil
}

// emptyString is a pre-allocated empty string pointer for sqlc COALESCE.
var emptyString = ""

// firstActor returns the first variadic actor ID or an invalid UUID.
func firstActor(actors []pgtype.UUID) pgtype.UUID {
	if len(actors) > 0 {
		return actors[0]
	}
	return pgtype.UUID{}
}

// writeAudit inserts an audit_logs row for a privileged persona mutation
// (BUG-PER-006). Best-effort — audit failures are logged, never propagated.
func (s *Service) writeAudit(ctx context.Context, actor pgtype.UUID, action, targetID string, meta map[string]any) {
	if s.store == nil || s.store.DB() == nil {
		return
	}
	payload, _ := json.Marshal(meta)
	var actorArg, targetArg any
	if actor.Valid {
		actorArg = actor
	}
	if targetID != "" {
		targetArg = targetID
	}
	if _, err := s.store.DB().Writer.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action_type, target_type, target_id, metadata)
		VALUES ($1, $2, 'persona', $3::uuid, $4::jsonb)
	`, actorArg, action, targetArg, string(payload)); err != nil {
		slog.Warn("personas: audit write failed", "action", action, "error", err)
	}
}

// DeletePersonaVoice deletes a shared voice by ID. System voices (the seeded
// role × tone catalog, is_system = true) cannot be deleted — they are
// platform-managed defaults; personas referencing a deleted voice would lose
// their tone anchor (BUG-005).
func (s *Service) DeletePersonaVoice(ctx context.Context, id pgtype.UUID, actors ...pgtype.UUID) error {
	voice, err := s.store.GetPersonaVoiceByID(ctx, id)
	if err != nil {
		return err
	}

	if voice.IsSystem {
		return fmt.Errorf("personas: %w: cannot delete a system voice", errpkg.ErrForbidden)
	}

	if err := s.store.DeletePersonaVoice(ctx, id); err != nil {
		return err
	}

	s.writeAudit(ctx, firstActor(actors), "persona.voice.delete", id.String(), nil)
	return nil
}
