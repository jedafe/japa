// Package mentorship — service.go provides business logic for mentorship
// program management (legacy admin CRUD) and §102 Mentorship Matching
// (ADR-110: pgvector matching, public discovery, DM auto-creation).
package mentorship

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/servicedir"
	"github.com/pgvector/pgvector-go"
)

// ErrProgramNotFound is returned when a mentorship program does not exist.
var ErrProgramNotFound = errors.New("mentorship program not found")

// Program represents a mentorship program.
type Program struct {
	ID          pgtype.UUID        `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	IsActive    bool               `json:"is_active"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
}

// ProgramWithCounts is a mentorship program with aggregate mentor/mentee/match counts.
type ProgramWithCounts struct {
	ID          pgtype.UUID        `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	IsActive    bool               `json:"is_active"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
	MentorCount int32              `json:"mentor_count"`
	MenteeCount int32              `json:"mentee_count"`
	MatchCount  int32              `json:"match_count"`
}

// Match represents a mentor-mentee match with resolved names.
type Match struct {
	ID pgtype.UUID `json:"id"`

	ProgramID      pgtype.UUID        `json:"program_id"`
	MentorID       pgtype.UUID        `json:"mentor_id"`
	MenteeID       pgtype.UUID        `json:"mentee_id"`
	Status         string             `json:"status"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
	UpdatedAt      pgtype.Timestamptz `json:"updated_at"`
	ProgramName    *string            `json:"program_name"`
	MentorUsername *string            `json:"mentor_username"`
	MenteeUsername *string            `json:"mentee_username"`
}

// Service coordinates mentorship program and match management.
type Service struct {
	read               *gen.Queries
	write              *gen.Queries
	db                 *db.DB
	enqueuer           TaskEnqueuer
	dmCreator          DMCreator
	settings           SettingsProvider
	embedder           Embedder
	meilisearchIndexer MeilisearchIndexer
	tracker            EventTracker
	// bookings is the servicedir compatibility seam (000824 unification):
	// connection lifecycle and the mentor shadow listing are servicedir
	// bookings/listings behind the scenes. Nil keeps program/match/
	// discovery paths working while connection calls fail fast.
	bookings BookingBackend
}

// BookingBackend is the narrow servicedir seam mentorship delegates to
// (000824 unification). *servicedir.Service satisfies it; the interface
// exists so mentorship doesn't bind to the concrete service and tests can
// fake the boundary. Method signatures deliberately reuse servicedir
// DTO/row types — the compat layer maps them onto the legacy Connection
// shape.
type BookingBackend interface {
	// MentorListingContentID resolves a mentor's live shadow listing —
	// the content id CreateBooking anchors a request to.
	MentorshipListingContentID(ctx context.Context, mentorID pgtype.UUID) (pgtype.UUID, error)
	// CreateBooking creates the service_bookings row behind a request.
	CreateBooking(ctx context.Context, bookerID, listingContentID pgtype.UUID, input servicedir.CreateBookingInput) (servicedir.BookingResult, error)
	// MentorshipBookingRow returns one kind-checked booking row.
	MentorshipBookingRow(ctx context.Context, bookingID pgtype.UUID) (servicedir.BookingRow, error)
	ListMentorshipBookingsByBooker(ctx context.Context, bookerID pgtype.UUID, status string, limit, offset int32) ([]servicedir.BookingRow, error)
	ListMentorshipBookingsByProvider(ctx context.Context, providerID pgtype.UUID, status string, limit, offset int32) ([]servicedir.BookingRow, error)
	ListMentorshipBookingsAdmin(ctx context.Context, status string, providerID, bookerID pgtype.UUID, limit, offset int32) ([]servicedir.BookingRow, error)
	// RespondToBooking applies the provider transitions
	// (accept|decline|complete).
	RespondToBooking(ctx context.Context, providerID, bookingID pgtype.UUID, action, note string) (servicedir.BookingDTO, error)
	// CompleteMyBooking is the mentee-side confirmed → fulfilled arm.
	CompleteMyBooking(ctx context.Context, bookingID, userID pgtype.UUID) (servicedir.BookingDTO, error)
	// CancelMyBooking is the booker-side cancel (pending|confirmed).
	CancelMyBooking(ctx context.Context, bookingID, userID pgtype.UUID, reason string) (servicedir.BookingDTO, error)
	// CancelProviderBooking is the mentor-side cancel (pending|confirmed).
	CancelProviderBooking(ctx context.Context, providerID, bookingID pgtype.UUID, reason string) (servicedir.BookingDTO, error)
	// AdminSetBookingStatus is the admin override transition.
	AdminSetBookingStatus(ctx context.Context, bookingID pgtype.UUID, input servicedir.AdminBookingActionInput) (servicedir.BookingDTO, error)
	// EnsureMentorShadowListing upserts the mentor's shadow listing and
	// links user_profiles.mentor_listing_content_id.
	EnsureMentorShadowListing(ctx context.Context, authorID pgtype.UUID, in servicedir.MentorShadowInput) (pgtype.UUID, error)
	// RetireMentorShadowListing soft-deletes the shadow and clears the link.
	RetireMentorShadowListing(ctx context.Context, authorID pgtype.UUID) error
	// MentorShadowContent reads the shadow listing's decoded page content
	// back for the profile GET — nil means no shadow exists.
	MentorShadowContent(ctx context.Context, mentorID pgtype.UUID) (*servicedir.MentorShadowContentDTO, error)
}

// errBackendUnavailable is returned when a connection/shadow path runs
// without the servicedir seam wired (unit tests, misconfigured server).
var errBackendUnavailable = errors.New("mentorship: booking backend unavailable")

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}

// ListPrograms returns all mentorship programs for the platform with aggregate counts.
func (s *Service) ListPrograms(ctx context.Context, limit, offset int32) ([]ProgramWithCounts, error) {
	rows, err := s.read.ListMentorshipProgramsWithCounts(ctx, gen.ListMentorshipProgramsWithCountsParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing mentorship programs: %w", err)
	}

	programs := make([]ProgramWithCounts, len(rows))
	for i, r := range rows {
		programs[i] = ProgramWithCounts{
			ID:          r.ID,
			Name:        r.Name,
			Description: r.Description,
			IsActive:    r.IsActive,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			MentorCount: r.MentorCount,
			MenteeCount: r.MenteeCount,
			MatchCount:  r.MatchCount,
		}
	}

	return programs, nil
}

// GetProgram returns a single mentorship program by ID.
func (s *Service) GetProgram(ctx context.Context, id pgtype.UUID) (Program, error) {
	row, err := s.read.GetMentorshipProgram(ctx,

		id,
	)
	if err != nil {
		return Program{}, fmt.Errorf("fetching mentorship program %s: %w", id, err)
	}

	return Program(row), nil
}

// CreateProgram creates a new mentorship program.
func (s *Service) CreateProgram(ctx context.Context, name, description string, isActive bool) (Program, error) {
	row, err := s.write.CreateMentorshipProgram(ctx, gen.CreateMentorshipProgramParams{
		Name:        name,
		Description: description,
		IsActive:    isActive,
	})
	if err != nil {
		return Program{}, fmt.Errorf("creating mentorship program: %w", err)
	}

	return Program(row), nil
}

// UpdateProgram updates an existing mentorship program.
func (s *Service) UpdateProgram(ctx context.Context, id pgtype.UUID, name, description *string, isActive *bool) (Program, error) {
	// Fetch current values for merge.
	current, err := s.GetProgram(ctx, id)
	if err != nil {
		return Program{}, err
	}

	n := current.Name
	if name != nil {
		n = *name
	}

	desc := current.Description
	if description != nil {
		desc = *description
	}

	ia := current.IsActive
	if isActive != nil {
		ia = *isActive
	}

	row, err := s.write.UpdateMentorshipProgram(ctx, gen.UpdateMentorshipProgramParams{
		ID:          id,
		Name:        n,
		Description: desc,
		IsActive:    ia,
	})
	if err != nil {
		return Program{}, fmt.Errorf("updating mentorship program %s: %w", id, err)
	}

	return Program(row), nil
}

// ListMatches returns all mentor-mentee matches for the platform with resolved names.
func (s *Service) ListMatches(ctx context.Context, limit, offset int32) ([]Match, error) {
	rows, err := s.read.ListMentorshipMatches(ctx, gen.ListMentorshipMatchesParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing mentorship matches: %w", err)
	}

	matches := make([]Match, len(rows))
	for i, r := range rows {
		matches[i] = Match{
			ID:             r.ID,
			ProgramID:      r.ProgramID,
			MentorID:       r.MentorID,
			MenteeID:       r.MenteeID,
			Status:         r.Status,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
			ProgramName:    r.ProgramName,
			MentorUsername: r.MentorUsername,
			MenteeUsername: r.MenteeUsername,
		}
	}

	return matches, nil
}

// UpdateMatch updates a mentor-mentee match status.
func (s *Service) UpdateMatch(ctx context.Context, id pgtype.UUID, status string) (Match, error) {
	row, err := s.write.UpdateMentorshipMatch(ctx, gen.UpdateMentorshipMatchParams{
		ID:     id,
		Status: status,
	})
	if err != nil {
		return Match{}, fmt.Errorf("updating mentorship match %s: %w", id, err)
	}

	return Match{
		ID: row.ID,

		ProgramID: row.ProgramID,
		MentorID:  row.MentorID,
		MenteeID:  row.MenteeID,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

// DeleteProgram deletes a mentorship program. The ON DELETE CASCADE on
// mentorship_matches.program_id will remove all associated matches.
func (s *Service) DeleteProgram(ctx context.Context, id pgtype.UUID) error {
	rowsAffected, err := s.write.DeleteMentorshipProgram(ctx,

		id,
	)
	if err != nil {
		return fmt.Errorf("deleting mentorship program %s: %w", id, err)
	}

	if rowsAffected == 0 {
		return ErrProgramNotFound
	}

	return nil
}

// =====================================================================
// §102 Mentorship Matching (ADR-110)
// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
// =====================================================================

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// The worker.Enqueuer satisfies this.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// SettingsProvider reads system_settings for runtime config (Rule 3).
// The config.Settings type satisfies this.
type SettingsProvider interface {
	GetSettingInt(ctx context.Context, key string) (int, error)
}

// DMCreator opens a DM conversation between two users (§48 integration).
// The websocket.DMStore satisfies this. Used to auto-open a DM thread when
// a mentorship connection is accepted (ADR-110).
type DMCreator interface {
	GetOrCreateConversation(ctx context.Context, participant1ID, participant2ID pgtype.UUID) (gen.DmConversation, error)
}

// Embedder generates pgvector embeddings for mentor bios via the AI gateway
// (§102, ADR-110). The ai.RAGEmbedder satisfies this.
type Embedder interface {
	GenerateEmbedding(ctx context.Context, text, model string) ([]float32, error)
}

// MeilisearchIndexer syncs a mentor's user document to the Meilisearch users
// index (§102). The search.Indexer satisfies this.
type MeilisearchIndexer interface {
	IndexUser(ctx context.Context, userID [16]byte) error
}

// EventTracker records platform analytics events (§102 analytics events).
// The analytics.Service satisfies this via an adapter.
type EventTracker interface {
	TrackEvent(ctx context.Context, userID pgtype.UUID, eventType, entityType string, entityID pgtype.UUID, meta map[string]any) error
}

// MentorProfile is the current user's mentor settings.
type MentorProfile struct {
	UserID pgtype.UUID `json:"user_id"`

	IsMentor          *bool              `json:"is_mentor"`
	MentorCapacity    *int16             `json:"mentor_capacity"`
	MentorMaxCapacity int32              `json:"mentor_max_capacity"`
	ActiveMenteeCount *int16             `json:"active_mentee_count"`
	MentorAreas       []string           `json:"mentor_areas"`
	MentorBio         *string            `json:"mentor_bio"`
	MentorSince       pgtype.Timestamptz `json:"mentor_since"`
	DisplayName       *string            `json:"display_name"`
	AvatarURL         *string            `json:"avatar_url"`

	// From the caller's user profile — profession drives the mentor card's
	// profession row; website_url links out from the mentor block.
	Profession *string `json:"profession,omitempty"`
	WebsiteURL *string `json:"website_url,omitempty"`

	// 000829 mentor card fields (§114 detail-page rebuild) — surfaced on
	// the mentorship listing's mentor block via the shadow listing join.
	Location        *string  `json:"location,omitempty"`
	MentorTitle     *string  `json:"mentor_title,omitempty"`
	ResponseTime    *string  `json:"mentor_response_time,omitempty"`
	Availability    *string  `json:"mentor_availability,omitempty"`
	SessionType     *string  `json:"mentor_session_type,omitempty"`
	Languages       []string `json:"mentor_languages,omitempty"`
	SessionFormats  []string `json:"mentor_session_formats,omitempty"`
	Countries       []string `json:"mentor_countries,omitempty"`
	Credentials     []string `json:"mentor_credentials,omitempty"`
	YearsExperience *int16   `json:"mentor_years_experience,omitempty"`
	SuccessLabel    *string  `json:"mentor_success_label,omitempty"`

	// ShadowContent carries the mentor's shadow-listing page content on
	// GET /me/mentor-profile so the settings editor seeds and PATCH
	// values round-trip (§114 2026-10-10). Absent when no shadow exists.
	ShadowContent *servicedir.MentorShadowContentDTO `json:"shadow_content,omitempty"`
}

// MentorCardInput carries the 000829 mentor-card fields plus the shadow
// listing's page content on PATCH /me/mentor-profile. Profile fields:
// nil preserves the stored value; "" clears a text field. The shadow
// content fields follow the servicedir replace-on-save contract (nil
// preserves, [] clears).
type MentorCardInput struct {
	Title           *string
	ResponseTime    *string
	Availability    *string
	SessionType     *string
	Languages       *[]string
	SessionFormats  *[]string
	Countries       *[]string
	Credentials     *[]string
	YearsExperience *int16
	SuccessLabel    *string

	SellingPoints *[]servicedir.ServiceIconEntryInput
	Facts         *[]servicedir.ServiceFactInput
	Highlights    *[]string
	Audience      *[]string
	Included      *[]servicedir.ServiceIconEntryInput
	Faqs          *[]servicedir.ServiceFaqInput
	Steps         *[]servicedir.ServiceStepInput
	Tiers         *[]servicedir.TierInput
}

// MentorSummary is a mentor's public card for discovery and comparison.
type MentorSummary struct {
	UserID            pgtype.UUID        `json:"user_id"`
	Username          string             `json:"username"`
	DisplayName       *string            `json:"display_name"`
	AvatarURL         *string            `json:"avatar_url"`
	MentorAreas       []string           `json:"mentor_areas"`
	MentorBio         *string            `json:"mentor_bio"`
	MentorSince       pgtype.Timestamptz `json:"mentor_since"`
	MentorCapacity    *int16             `json:"mentor_capacity"`
	ActiveMenteeCount *int16             `json:"active_mentee_count"`
	AvailableSlots    int32              `json:"available_slots"`
	Profession        *string            `json:"profession"`
	WebsiteURL        *string            `json:"website_url"`
}

// Connection is a mentorship connection with resolved user names.
type Connection struct {
	ID pgtype.UUID `json:"id"`

	MentorID           pgtype.UUID        `json:"mentor_id"`
	MenteeID           pgtype.UUID        `json:"mentee_id"`
	Status             string             `json:"status"`
	Goals              *string            `json:"goals"`
	DestinationSpaceID pgtype.UUID        `json:"destination_space_id"`
	RespondedAt        pgtype.Timestamptz `json:"responded_at"`
	CompletedAt        pgtype.Timestamptz `json:"completed_at"`
	CancelledAt        pgtype.Timestamptz `json:"cancelled_at"`
	CancelReason       *string            `json:"cancel_reason"`
	CreatedAt          pgtype.Timestamptz `json:"created_at"`
	UpdatedAt          pgtype.Timestamptz `json:"updated_at"`
	MentorUsername     *string            `json:"mentor_username,omitempty"`
	MenteeUsername     *string            `json:"mentee_username,omitempty"`
	MentorDisplayName  *string            `json:"mentor_display_name,omitempty"`
	MenteeDisplayName  *string            `json:"mentee_display_name,omitempty"`
	DMConversationID   pgtype.UUID        `json:"dm_conversation_id,omitempty"`
}

// Sentinel errors for §102.
var (
	ErrMentorNotFound      = errors.New("mentor not found")
	ErrConnectionNotFound  = errors.New("mentorship connection not found")
	ErrDuplicateRequest    = errors.New("duplicate pending request already exists")
	ErrCannotSelfMentor    = errors.New("cannot request mentorship from yourself")
	ErrInvalidStatusChange = errors.New("invalid status transition")
	ErrNotAuthorized       = errors.New("not authorized to modify this connection")
	ErrCapacityExceeded    = errors.New("mentor_capacity exceeds platform max_capacity setting")
	// ErrMentorAtCapacity surfaces servicedir's capacity gate — a mentor at
	// mentor_capacity can't take new requests (000824 unification added the
	// enforcement; the dropped connections table had none).
	ErrMentorAtCapacity = errors.New("mentor at capacity")
	// ErrInvalidDestination surfaces servicedir's destination_space_id
	// validation (malformed uuid or unknown space).
	ErrInvalidDestination = errors.New("invalid destination space")
	// ErrInvalidMentorField — a 000829 mentor-card field failed its range
	// check (400 invalid_input).
	ErrInvalidMentorField = errors.New("invalid mentor field")
)

// SetEnqueuer wires the Asynq enqueuer for mentor:generate_embedding tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetDMCreator wires the DM creator for auto-opening DM threads on accept.
func (s *Service) SetDMCreator(d DMCreator) { s.dmCreator = d }

// SetSettingsProvider wires the system_settings reader for capacity validation.
func (s *Service) SetSettingsProvider(sp SettingsProvider) { s.settings = sp }

// mentorMaxCapacity returns the platform cap for mentor capacity from system_settings.
func (s *Service) mentorMaxCapacity(ctx context.Context) int32 {
	maxCap := int32(10) // catalog default for mentorship.max_capacity (§102)

	if s.settings != nil {
		if v, err := s.settings.GetSettingInt(ctx, "mentorship.max_capacity"); err == nil && v > 0 && v <= math.MaxInt32 {
			maxCap = int32(v) //nolint:gosec // G115: v is checked against math.MaxInt32.
		}
	}

	return maxCap
}

// SetEmbedder wires the AI embedder for mentor:generate_embedding tasks.
func (s *Service) SetEmbedder(e Embedder) { s.embedder = e }

// SetMeilisearchIndexer wires the Meilisearch indexer for mentor:update_meilisearch tasks.
func (s *Service) SetMeilisearchIndexer(mi MeilisearchIndexer) { s.meilisearchIndexer = mi }

// SetEventTracker wires the analytics event tracker for §102 analytics events.
func (s *Service) SetEventTracker(t EventTracker) { s.tracker = t }

// SetBookingBackend wires the servicedir compatibility seam (000824):
// connection lifecycle and the mentor shadow listing delegate to
// servicedir bookings/listings.
func (s *Service) SetBookingBackend(b BookingBackend) { s.bookings = b }

// trackEvent is a best-effort analytics event emitter (§102). Failures are
// logged but never abort the caller's operation.
func (s *Service) trackEvent(ctx context.Context, userID pgtype.UUID, eventType, entityType string, entityID pgtype.UUID, meta map[string]any) {
	if s.tracker == nil {
		return
	}

	if err := s.tracker.TrackEvent(ctx, userID, eventType, entityType, entityID, meta); err != nil {
		slog.Warn("mentorship: track event", "event_type", eventType, "error", err)
	}
}

// GetMentorProfile returns the current user's mentor profile settings.
// GET /api/v1/me/mentor-profile.
func (s *Service) GetMentorProfile(ctx context.Context, userID pgtype.UUID) (MentorProfile, error) {
	row, err := s.read.GetMentorProfile(ctx,

		userID,
	)
	if err != nil {
		return MentorProfile{}, fmt.Errorf("getting mentor profile: %w", err)
	}

	// 000829: the settings editor needs the shadow listing's page content
	// to seed its structured-content fields — best-effort read through
	// the servicedir seam (nil backend keeps program/match surfaces).
	var shadow *servicedir.MentorShadowContentDTO

	if s.bookings != nil {
		var err error

		shadow, err = s.bookings.MentorShadowContent(ctx, userID)
		if err != nil {
			return MentorProfile{}, fmt.Errorf("getting mentor shadow content: %w", err)
		}
	}

	return MentorProfile{
		UserID: row.UserID,

		IsMentor:          row.IsMentor,
		MentorCapacity:    row.MentorCapacity,
		MentorMaxCapacity: s.mentorMaxCapacity(ctx),
		ActiveMenteeCount: row.ActiveMenteeCount,
		MentorAreas:       row.MentorAreas,
		MentorBio:         row.MentorBio,
		MentorSince:       row.MentorSince,
		DisplayName:       row.DisplayName,
		AvatarURL:         row.AvatarUrl,
		Profession:        row.Profession,
		WebsiteURL:        row.WebsiteUrl,
		Location:          row.Location,
		MentorTitle:       row.MentorTitle,
		ResponseTime:      row.MentorResponseTime,
		Availability:      row.MentorAvailability,
		SessionType:       row.MentorSessionType,
		Languages:         row.MentorLanguages,
		SessionFormats:    row.MentorSessionFormats,
		Countries:         row.MentorCountries,
		Credentials:       row.MentorCredentials,
		YearsExperience:   row.MentorYearsExperience,
		SuccessLabel:      row.MentorSuccessLabel,
		ShadowContent:     shadow,
	}, nil
}

// UpdateMentorProfile toggles is_mentor and sets capacity/areas/bio plus
// the 000829 mentor-card fields and the shadow listing's page content.
// PATCH /api/v1/me/mentor-profile
// When mentor_bio changes, enqueues the mentor:generate_embedding task.
func (s *Service) UpdateMentorProfile(ctx context.Context, userID pgtype.UUID, isMentor *bool, capacity *int16, areas []string, bio *string, card MentorCardInput) (MentorProfile, error) {
	// Fetch current values for merge.
	current, err := s.read.GetMentorProfile(ctx,

		userID,
	)
	if err != nil {
		return MentorProfile{}, fmt.Errorf("loading current mentor profile: %w", err)
	}

	// Load the platform-wide cap once for validation and the response.
	mentorMaxCap := s.mentorMaxCapacity(ctx)

	// Merge: use new values where provided, otherwise keep current.
	mergedIsMentor := current.IsMentor
	if isMentor != nil {
		mergedIsMentor = isMentor
	}

	mergedCapacity := current.MentorCapacity
	if capacity != nil {
		mergedCapacity = capacity
		// Validate against platform-wide cap (Rule 3, §102 system_settings).
		if *mergedCapacity > 0 && int(*mergedCapacity) > int(mentorMaxCap) {
			return MentorProfile{}, ErrCapacityExceeded
		}
	}

	mergedAreas := current.MentorAreas
	if areas != nil {
		mergedAreas = areas
	}

	mergedBio := current.MentorBio
	bioChanged := false

	if bio != nil {
		if current.MentorBio == nil || *bio != *current.MentorBio {
			bioChanged = true
		}

		mergedBio = bio
	}

	// 000829: bound years_experience to a sane range before the write.
	if card.YearsExperience != nil && (*card.YearsExperience < 0 || *card.YearsExperience > 150) {
		return MentorProfile{}, ErrInvalidMentorField
	}

	params := gen.UpdateMentorProfileParams{
		IsMentor:       mergedIsMentor,
		MentorCapacity: mergedCapacity,
		MentorAreas:    mergedAreas,
		MentorBio:      mergedBio,
		// Mentor card fields — nil preserves via the query's COALESCE;
		// a text "" clears, an empty array clears.
		MentorTitle:           card.Title,
		MentorResponseTime:    card.ResponseTime,
		MentorAvailability:    card.Availability,
		MentorSessionType:     card.SessionType,
		MentorYearsExperience: card.YearsExperience,
		MentorSuccessLabel:    card.SuccessLabel,

		UserID: userID,
	}
	if card.Languages != nil {
		params.MentorLanguages = *card.Languages
	}

	if card.SessionFormats != nil {
		params.MentorSessionFormats = *card.SessionFormats
	}

	if card.Countries != nil {
		params.MentorCountries = *card.Countries
	}

	if card.Credentials != nil {
		params.MentorCredentials = *card.Credentials
	}

	row, err := s.write.UpdateMentorProfile(ctx, params)
	if err != nil {
		return MentorProfile{}, fmt.Errorf("updating mentor profile: %w", err)
	}

	// Enqueue embedding generation if bio changed and mentor is opted in.
	if bioChanged && mergedIsMentor != nil && *mergedIsMentor && mergedBio != nil && *mergedBio != "" {
		s.enqueueGenerateEmbedding(ctx, userID)
	}

	// Shadow listing lifecycle (000824 unification): a public mentor's
	// directory presence is a servicedir contents + service_listing_meta
	// mirror — discovery and bookings run off the unified tables. The
	// ensure path is an idempotent upsert (also refreshes title/body on
	// bio/display-name changes while is_mentor stays true); a failure
	// surfaces so the caller retries rather than leaving the mentor
	// opted-in but undiscoverable.
	if s.bookings != nil && mergedIsMentor != nil {
		if *mergedIsMentor {
			displayName := ""
			if row.DisplayName != nil {
				displayName = *row.DisplayName
			}

			bio := ""
			if row.MentorBio != nil {
				bio = *row.MentorBio
			}

			if _, err := s.bookings.EnsureMentorShadowListing(ctx, userID, servicedir.MentorShadowInput{
				Username:    current.Username,
				DisplayName: displayName,
				Bio:         bio,
				// §114 2026-10-10: the shadow mirrors the structured page
				// content the mentorship detail page renders.
				SellingPoints: card.SellingPoints,
				Facts:         card.Facts,
				Highlights:    card.Highlights,
				Audience:      card.Audience,
				Included:      card.Included,
				Faqs:          card.Faqs,
				Steps:         card.Steps,
				Tiers:         card.Tiers,
			}); err != nil {
				return MentorProfile{}, fmt.Errorf("sync mentor shadow listing: %w", err)
			}
		} else if err := s.bookings.RetireMentorShadowListing(ctx, userID); err != nil {
			// Retire is a no-op when no shadow exists (non-mentors,
			// never-synced profiles) — still run it unconditionally so a
			// migrated orphan shadow can't outlive the opt-out.
			return MentorProfile{}, fmt.Errorf("retire mentor shadow listing: %w", err)
		}
	}

	// Read the shadow content back so the response mirrors what the
	// editor just saved (a retired shadow resolves to nil).
	var shadow *servicedir.MentorShadowContentDTO

	if s.bookings != nil {
		var err error

		shadow, err = s.bookings.MentorShadowContent(ctx, userID)
		if err != nil {
			slog.Warn("mentorship: shadow content read failed — returning profile without shadow", "user_id", userID.String(), "error", err)

			shadow = nil
		}
	}

	return MentorProfile{
		UserID: row.UserID,

		IsMentor:          row.IsMentor,
		MentorCapacity:    row.MentorCapacity,
		MentorMaxCapacity: mentorMaxCap,
		ActiveMenteeCount: row.ActiveMenteeCount,
		MentorAreas:       row.MentorAreas,
		MentorBio:         row.MentorBio,
		MentorSince:       row.MentorSince,
		DisplayName:       row.DisplayName,
		AvatarURL:         row.AvatarUrl,
		Profession:        row.Profession,
		WebsiteURL:        row.WebsiteUrl,
		Location:          current.Location,
		MentorTitle:       row.MentorTitle,
		ResponseTime:      row.MentorResponseTime,
		Availability:      row.MentorAvailability,
		SessionType:       row.MentorSessionType,
		Languages:         row.MentorLanguages,
		SessionFormats:    row.MentorSessionFormats,
		Countries:         row.MentorCountries,
		Credentials:       row.MentorCredentials,
		YearsExperience:   row.MentorYearsExperience,
		SuccessLabel:      row.MentorSuccessLabel,
		ShadowContent:     shadow,
	}, nil
}

// GetMentorByUsername returns a mentor's public profile by username.
// GET /api/v1/mentors/:username.
func (s *Service) GetMentorByUsername(ctx context.Context, username string) (MentorSummary, error) {
	row, err := s.read.GetMentorByUsername(ctx,

		username,
	)
	if err != nil {
		return MentorSummary{}, ErrMentorNotFound
	}

	// §102 analytics: mentor_view { mentor_username }
	s.trackEvent(ctx, row.UserID, "mentor_view", "user", row.UserID,
		map[string]any{"mentor_username": row.Username})

	return mentorSummaryFromRow(row.UserID, row.IsMentor, row.MentorCapacity,
		row.ActiveMenteeCount, row.MentorAreas, row.MentorBio, row.MentorSince,
		row.DisplayName, row.AvatarUrl, row.Username, 0, row.Profession, row.WebsiteUrl), nil
}

// ListMentors discovers mentors with optional destination and area filters.
// GET /api/v1/mentors?dest=&areas=&sort=match|newest|capacity&page=
// Returns the page of mentors plus the total matching count for pagination.
// When sort=match and a menteeEmbedding is provided, uses the combined
// 0.6 keyword + 0.4 bio cosine §102 score (ListMentorsByMatch). When
// sort=match without an embedding, falls back to keyword-only scoring
// via the same ListMentors query ordered by area_overlap.
// includeUnavailable=false (the default) hides at-capacity mentors
// (available_slots <= 0) from discovery (§102/§111 hardening amendment).
func (s *Service) ListMentors(ctx context.Context, destSpaceID pgtype.UUID, areas []string, sort string, menteeEmbedding *pgvector.Vector, includeUnavailable bool, limit, offset int32) ([]MentorSummary, int, error) {
	// sort=match with an embedding uses the pgvector query.
	if sort == "match" && menteeEmbedding != nil {
		rows, err := s.read.ListMentorsByMatch(ctx, gen.ListMentorsByMatchParams{
			MenteeEmbedding: menteeEmbedding,

			DestSpaceID:        destSpaceID,
			Areas:              areas,
			IncludeUnavailable: includeUnavailable,
			PageLimit:          limit,
			PageOffset:         offset,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("listing mentors by match: %w", err)
		}

		var total int

		mentors := make([]MentorSummary, len(rows))

		for i, r := range rows {
			if i == 0 {
				total = int(r.TotalCount)
			}

			mentors[i] = mentorSummaryFromRow(r.UserID, r.IsMentor, r.MentorCapacity,
				r.ActiveMenteeCount, r.MentorAreas, r.MentorBio, r.MentorSince,
				r.DisplayName, r.AvatarUrl, r.Username, r.AvailableSlots, r.Profession, r.WebsiteUrl)
		}

		return mentors, total, nil
	}

	// Default and sort=match-without-embedding use the same ListMentors query,
	// which now orders by area_overlap when sort='match'.
	rows, err := s.read.ListMentors(ctx, gen.ListMentorsParams{
		DestSpaceID:        destSpaceID,
		Areas:              areas,
		IncludeUnavailable: includeUnavailable,
		Sort:               sort,
		PageLimit:          limit,
		PageOffset:         offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing mentors: %w", err)
	}

	var total int

	mentors := make([]MentorSummary, len(rows))

	for i, r := range rows {
		if i == 0 {
			total = int(r.TotalCount)
		}

		mentors[i] = mentorSummaryFromRow(r.UserID, r.IsMentor, r.MentorCapacity,
			r.ActiveMenteeCount, r.MentorAreas, r.MentorBio, r.MentorSince,
			r.DisplayName, r.AvatarUrl, r.Username, r.AvailableSlots, r.Profession, r.WebsiteUrl)
	}

	return mentors, total, nil
}

// SearchMentors returns mentors whose username or display name matches the
// query, scoped to the platform. Same 000824 live-shadow gate as the other
// discovery queries — a mentor is findable iff their servicedir shadow
// listing is published.
func (s *Service) SearchMentors(ctx context.Context, query string, limit int32) ([]MentorSummary, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	rows, err := s.read.SearchMentors(ctx, gen.SearchMentorsParams{
		Q:         "%" + query + "%",
		PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching mentors: %w", err)
	}

	out := make([]MentorSummary, len(rows))
	for i, r := range rows {
		out[i] = mentorSummaryFromRow(
			r.UserID, r.IsMentor, r.MentorCapacity,
			r.ActiveMenteeCount, r.MentorAreas, r.MentorBio, r.MentorSince,
			r.DisplayName, r.AvatarUrl, r.Username, r.AvailableSlots,
			r.Profession, r.WebsiteUrl,
		)
	}

	return out, nil
}

// GetMenteeEmbedding fetches the authenticated user's mentor_embedding for
// use as the mentee_goal_embedding in match scoring (§102). Returns nil when
// the user has no embedding (non-mentor or mentor without a bio) — the caller
// falls back to keyword-only scoring (weight = 1.0).
func (s *Service) GetMenteeEmbedding(ctx context.Context, userID pgtype.UUID) (*pgvector.Vector, error) {
	row, err := s.read.GetUserEmbedding(ctx,

		userID,
	)
	if err != nil {
		return nil, nil // best-effort: fall back to keyword-only on error
	}

	return row.MentorEmbedding, nil
}

// GetMenteeDestinationSpaceID fetches the authenticated user's
// destination_space_id from user_profiles for use as the default ?dest=
// filter on the discovery page (§102 Geo Awareness: "The mentee's profile
// destination_country_id is injected as a default filter when logged in,
// with no UI action required"). Returns pgtype.UUID{} (invalid) when no
// destination is set.
func (s *Service) GetMenteeDestinationSpaceID(ctx context.Context, userID pgtype.UUID) (pgtype.UUID, error) {
	row, err := s.read.GetMenteeDestinationSpaceID(ctx,

		userID,
	)
	if err != nil {
		return pgtype.UUID{}, nil // best-effort: no default filter on error
	}

	return row.DestinationSpaceID, nil
}

// GetMentorsByIDs returns up to 4 mentors for the comparison endpoint.
// GET /api/v1/compare/mentors?ids=a,b,c,d.
func (s *Service) GetMentorsByIDs(ctx context.Context, userIDs []pgtype.UUID) ([]MentorSummary, error) {
	rows, err := s.read.GetMentorsByIDs(ctx,

		userIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("getting mentors by IDs: %w", err)
	}

	mentors := make([]MentorSummary, len(rows))
	for i, r := range rows {
		mentors[i] = mentorSummaryFromRow(r.UserID, r.IsMentor, r.MentorCapacity,
			r.ActiveMenteeCount, r.MentorAreas, r.MentorBio, r.MentorSince,
			r.DisplayName, r.AvatarUrl, r.Username, r.AvailableSlots, r.Profession, r.WebsiteUrl)
	}

	return mentors, nil
}

// ---------------------------------------------------------------------
// Connection lifecycle — the 000824 compatibility surface.
//
// mentorship_connections is gone; every method below delegates to the
// servicedir booking pipeline through the BookingBackend seam. Legacy
// status vocabulary maps at the boundary: a "connection" is a
// listing_kind='mentorship' booking with intent='inquiry' —
// provider_user_id is the mentor, booker_user_id is the mentee.
// ---------------------------------------------------------------------

// bookingStatus translates the legacy connection vocabulary into the
// service_bookings state machine: active → confirmed, completed →
// fulfilled; every other token passes through verbatim (pending,
// declined, cancelled share the name). Duplicate spellings from the
// pre-000824 enum surface are aliased to their canonical token at this
// compat boundary — the unified pipeline only ever sees canonical
// vocabulary downstream.
func bookingStatus(status string) string {
	switch status {
	case "active", "accepted":
		return servicedir.BookingStatusConfirmed
	case "completed", "fulfilled":
		return servicedir.BookingStatusFulfilled
	default:
		return status
	}
}

// connectionStatus maps a service_bookings status back onto the legacy
// mentorship_connections vocabulary the API contract exposes.
func connectionStatus(status string) string {
	switch status {
	case servicedir.BookingStatusConfirmed:
		return "active"
	case servicedir.BookingStatusFulfilled:
		return "completed"
	default:
		return status
	}
}

// mapBookingErr translates servicedir sentinels into the legacy mentorship
// vocabulary so handlers keep their existing error codes (Rule 5).
func mapBookingErr(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrConnectionNotFound
	case errors.Is(err, servicedir.ErrNotOwner):
		return ErrNotAuthorized
	case errors.Is(err, servicedir.ErrInvalidBookingTransition):
		return ErrInvalidStatusChange
	case errors.Is(err, servicedir.ErrDuplicateRequest):
		return ErrDuplicateRequest
	case errors.Is(err, servicedir.ErrMentorAtCapacity):
		return ErrMentorAtCapacity
	case errors.Is(err, servicedir.ErrSelfMentorship):
		return ErrCannotSelfMentor
	case errors.Is(err, servicedir.ErrBookingUnavailable):
		// The shadow listing exists but isn't bookable (unpublished,
		// no profile row) — surface like a non-existent mentor.
		return ErrMentorNotFound
	case errors.Is(err, servicedir.ErrInvalidID):
		// Malformed/unknown destination_space_id on the request.
		return ErrInvalidDestination
	default:
		// servicedir.ErrMentorshipDisabled passes through — the handler
		// maps it to the feature-off 404.
		return err
	}
}

// bookingMetaString reads a string key out of service_bookings.meta —
// the channel for legacy fields the bookings table has no column for
// (responded_at on declined rows; see DeclineServiceBooking).
func bookingMetaString(meta []byte, key string) string {
	if len(meta) == 0 {
		return ""
	}

	var m map[string]any
	if err := json.Unmarshal(meta, &m); err != nil {
		return ""
	}

	if v, ok := m[key].(string); ok {
		return v
	}

	return ""
}

// bookingRow re-reads one mentorship-kind booking, mapping errors to the
// legacy not-found sentinel.
func (s *Service) bookingRow(ctx context.Context, connID pgtype.UUID) (servicedir.BookingRow, error) {
	row, err := s.bookings.MentorshipBookingRow(ctx, connID)
	if err != nil {
		return servicedir.BookingRow{}, mapBookingErr(err)
	}

	return row, nil
}

// CreateConnection creates a new mentorship request (status = 'pending').
// POST /api/v1/mentorship/request.
// (000824) The request lands as an intent='inquiry' service_bookings row
// anchored to the mentor's shadow listing — servicedir owns dedupe (one
// open request per mentee+mentor, plus the 000825 unique index), the
// mentor-capacity gate, and the mentorship.enabled feature flag.
func (s *Service) CreateConnection(ctx context.Context, mentorID, menteeID pgtype.UUID, goals *string, destSpaceID pgtype.UUID) (Connection, error) {
	if mentorID == menteeID {
		return Connection{}, ErrCannotSelfMentor
	}

	if s.bookings == nil {
		return Connection{}, errBackendUnavailable
	}

	// Resolve the mentor's shadow listing — the booking's anchor. No live
	// listing means the mentor isn't requestable (never opted in, or the
	// shadow was retired on opt-out).
	contentID, err := s.bookings.MentorshipListingContentID(ctx, mentorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Connection{}, ErrMentorNotFound
		}

		return Connection{}, fmt.Errorf("resolving mentor listing: %w", err)
	}

	input := servicedir.CreateBookingInput{Intent: servicedir.BookingIntentInquiry}
	if goals != nil {
		input.Goals = *goals
	}

	if destSpaceID.Valid {
		input.DestinationSpaceID = destSpaceID.String()
	}

	res, err := s.bookings.CreateBooking(ctx, menteeID, contentID, input)
	if err != nil {
		return Connection{}, mapBookingErr(err)
	}

	var bookingID pgtype.UUID
	if err := bookingID.Scan(res.Booking.ID); err != nil {
		return Connection{}, fmt.Errorf("parsing booking id: %w", err)
	}

	// §102 analytics: mentorship_request_sent { mentor_id }
	// The mentee (menteeID) is the actor; the mentor is the entity.
	s.trackEvent(ctx, menteeID, "mentorship_request_sent", "mentorship_connection", bookingID,
		map[string]any{"mentor_id": mentorID.String()})

	row, err := s.bookingRow(ctx, bookingID)
	if err != nil {
		return Connection{}, fmt.Errorf("loading created connection: %w", err)
	}

	return connectionFromBookingRow(row), nil
}

// ListOutgoingConnections lists requests sent by the mentee.
// GET /api/v1/me/mentorship/outgoing.
func (s *Service) ListOutgoingConnections(ctx context.Context, menteeID pgtype.UUID, status string, limit, offset int32) ([]Connection, error) {
	if s.bookings == nil {
		return nil, errBackendUnavailable
	}

	rows, err := s.bookings.ListMentorshipBookingsByBooker(ctx, menteeID, bookingStatus(status), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing outgoing connections: %w", err)
	}

	conns := make([]Connection, len(rows))
	for i, r := range rows {
		conns[i] = connectionFromBookingRow(r)
	}

	return conns, nil
}

// CancelOutgoingConnection cancels a pending request by the mentee.
// DELETE /api/v1/me/mentorship/outgoing/:id.
func (s *Service) CancelOutgoingConnection(ctx context.Context, menteeID, connID pgtype.UUID) error {
	if s.bookings == nil {
		return errBackendUnavailable
	}

	row, err := s.bookingRow(ctx, connID)
	if err != nil {
		return err
	}

	// Legacy guard parity: only a still-pending request the mentee owns
	// is cancellable here — anything else reports not-found, same as the
	// old WHERE status='pending' AND mentee_id=$2 zero-row result.
	if row.BookerUserID != menteeID || row.Status != servicedir.BookingStatusPending {
		return ErrConnectionNotFound
	}

	// row.ID, not connID: the caller may hold a pre-migration connection
	// id resolved via meta->>'connection_id' — the backend only knows the
	// booking id.
	if _, err := s.bookings.CancelMyBooking(ctx, row.ID, menteeID, ""); err != nil {
		return mapBookingErr(err)
	}

	return nil
}

// ListIncomingConnections lists requests received by the mentor.
// GET /api/v1/me/mentorship/incoming.
func (s *Service) ListIncomingConnections(ctx context.Context, mentorID pgtype.UUID, status string, limit, offset int32) ([]Connection, error) {
	if s.bookings == nil {
		return nil, errBackendUnavailable
	}

	rows, err := s.bookings.ListMentorshipBookingsByProvider(ctx, mentorID, bookingStatus(status), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing incoming connections: %w", err)
	}

	conns := make([]Connection, len(rows))
	for i, r := range rows {
		conns[i] = connectionFromBookingRow(r)
	}

	return conns, nil
}

// AcceptConnection accepts a pending request (pending → active).
// Only the mentor of the connection can accept. Also auto-opens a DM thread
// between mentor and mentee (ADR-110 §48 integration).
// PATCH /api/v1/me/mentorship/incoming/:id/accept.
func (s *Service) AcceptConnection(ctx context.Context, mentorID, connID pgtype.UUID) (Connection, error) {
	if s.bookings == nil {
		return Connection{}, errBackendUnavailable
	}

	row, err := s.bookingRow(ctx, connID)
	if err != nil {
		return Connection{}, err
	}

	if row.ProviderUserID != mentorID {
		return Connection{}, ErrNotAuthorized
	}

	// Legacy guard parity: the old UPDATE filtered WHERE status='pending'
	// — a non-pending row surfaced as not-found, not an invalid transition.
	if row.Status != servicedir.BookingStatusPending {
		return Connection{}, ErrConnectionNotFound
	}

	// row.ID — connID may be a legacy connection id (meta fallback).
	if _, err := s.bookings.RespondToBooking(ctx, mentorID, row.ID, "accept", ""); err != nil {
		return Connection{}, mapBookingErr(err)
	}

	updated, err := s.bookingRow(ctx, row.ID)
	if err != nil {
		return Connection{}, fmt.Errorf("loading accepted connection: %w", err)
	}

	result := connectionFromBookingRow(updated)

	// Auto-open DM thread + report its id (legacy contract). servicedir
	// attempts the same GetOrCreate on its own seam — the call is
	// idempotent, so both paths converging is safe; the compat layer
	// repeats it because the legacy response carries dm_conversation_id.
	if s.dmCreator != nil {
		if dm, dmErr := s.dmCreator.GetOrCreateConversation(ctx, updated.ProviderUserID, updated.BookerUserID); dmErr != nil {
			slog.Warn("mentorship: auto-open DM thread on accept", "connection_id", connID, "error", dmErr)
			// Non-fatal — the connection is still accepted; the DM can be
			// opened manually later.
		} else {
			result.DMConversationID = dm.ID
		}
	}

	// active_mentee_count moved via the booking trigger — servicedir
	// already enqueued the mentor Meilisearch reindex on accept.

	// §102 analytics: mentorship_accepted { mentee_id }
	// The mentor (mentorID) is the actor.
	s.trackEvent(ctx, mentorID, "mentorship_accepted", "mentorship_connection", row.ID,
		map[string]any{"mentee_id": updated.BookerUserID.String()})

	return result, nil
}

// DeclineConnection declines a pending request (pending → declined).
// Only the mentor of the connection can decline.
// PATCH /api/v1/me/mentorship/incoming/:id/decline.
func (s *Service) DeclineConnection(ctx context.Context, mentorID, connID pgtype.UUID) (Connection, error) {
	if s.bookings == nil {
		return Connection{}, errBackendUnavailable
	}

	row, err := s.bookingRow(ctx, connID)
	if err != nil {
		return Connection{}, err
	}

	if row.ProviderUserID != mentorID {
		return Connection{}, ErrNotAuthorized
	}

	// Legacy guard parity: only pending rows were declinable.
	if row.Status != servicedir.BookingStatusPending {
		return Connection{}, ErrConnectionNotFound
	}

	// row.ID — connID may be a legacy connection id (meta fallback).
	if _, err := s.bookings.RespondToBooking(ctx, mentorID, row.ID, "decline", ""); err != nil {
		return Connection{}, mapBookingErr(err)
	}

	updated, err := s.bookingRow(ctx, row.ID)
	if err != nil {
		return Connection{}, fmt.Errorf("loading declined connection: %w", err)
	}

	return connectionFromBookingRow(updated), nil
}

// CompleteConnection completes an active connection (active → completed).
// Either party (mentor or mentee) can complete.
// PATCH /api/v1/mentorship/:id/complete.
func (s *Service) CompleteConnection(ctx context.Context, userID, connID pgtype.UUID) (Connection, error) {
	if s.bookings == nil {
		return Connection{}, errBackendUnavailable
	}

	row, err := s.bookingRow(ctx, connID)
	if err != nil {
		return Connection{}, err
	}

	isMentor := row.ProviderUserID == userID
	if !isMentor && row.BookerUserID != userID {
		return Connection{}, ErrNotAuthorized
	}

	// Legacy guard parity: the old query filtered WHERE status='active' —
	// a non-active row surfaced as not-found, not an invalid transition.
	if row.Status != servicedir.BookingStatusConfirmed {
		return Connection{}, ErrConnectionNotFound
	}

	// row.ID — connID may be a legacy connection id (meta fallback).
	if isMentor {
		_, err = s.bookings.RespondToBooking(ctx, userID, row.ID, "complete", "")
	} else {
		_, err = s.bookings.CompleteMyBooking(ctx, row.ID, userID)
	}

	if err != nil {
		return Connection{}, mapBookingErr(err)
	}

	// §102 analytics: mentorship_completed { connection_id }
	s.trackEvent(ctx, userID, "mentorship_completed", "mentorship_connection", row.ID, nil)

	updated, err := s.bookingRow(ctx, row.ID)
	if err != nil {
		return Connection{}, fmt.Errorf("loading completed connection: %w", err)
	}

	return connectionFromBookingRow(updated), nil
}

// CancelConnection cancels an active connection (active → cancelled).
// Either party (mentor or mentee) can cancel.
// PATCH /api/v1/mentorship/:id/cancel.
func (s *Service) CancelConnection(ctx context.Context, userID, connID pgtype.UUID, reason *string) (Connection, error) {
	if s.bookings == nil {
		return Connection{}, errBackendUnavailable
	}

	row, err := s.bookingRow(ctx, connID)
	if err != nil {
		return Connection{}, err
	}

	isMentor := row.ProviderUserID == userID
	if !isMentor && row.BookerUserID != userID {
		return Connection{}, ErrNotAuthorized
	}

	// Legacy guard parity: only 'active' connections were cancellable via
	// this route (pending requests go through CancelOutgoingConnection).
	if row.Status != servicedir.BookingStatusConfirmed {
		return Connection{}, ErrConnectionNotFound
	}

	reasonStr := ""
	if reason != nil {
		reasonStr = *reason
	}

	// row.ID — connID may be a legacy connection id (meta fallback).
	if isMentor {
		_, err = s.bookings.CancelProviderBooking(ctx, userID, row.ID, reasonStr)
	} else {
		_, err = s.bookings.CancelMyBooking(ctx, row.ID, userID, reasonStr)
	}

	if err != nil {
		return Connection{}, mapBookingErr(err)
	}

	updated, err := s.bookingRow(ctx, row.ID)
	if err != nil {
		return Connection{}, fmt.Errorf("loading cancelled connection: %w", err)
	}

	return connectionFromBookingRow(updated), nil
}

// ListAllConnections lists all connections for admin (with filters).
// GET /api/v1/admin/mentorship/connections.
func (s *Service) ListAllConnections(ctx context.Context, status string, mentorID, menteeID pgtype.UUID, limit, offset int32) ([]Connection, error) {
	if s.bookings == nil {
		return nil, errBackendUnavailable
	}

	// The ?status= filter speaks connection vocabulary only — booking-only
	// tokens (disputed, refunded) are invalid input, not an empty page.
	if status != "" && !isConnectionStatus(status) {
		return nil, ErrInvalidStatusChange
	}

	rows, err := s.bookings.ListMentorshipBookingsAdmin(ctx, bookingStatus(status), mentorID, menteeID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing all connections: %w", err)
	}

	conns := make([]Connection, len(rows))
	for i, r := range rows {
		conns[i] = connectionFromBookingRow(r)
	}

	return conns, nil
}

// isConnectionStatus reports whether s belongs to the legacy
// mentorship_connections vocabulary — the only statuses the compat
// admin/filter surfaces accept. Booking-only statuses (disputed,
// refunded) are unreachable on mentorship rows and reject as invalid
// input rather than silently filtering to an empty set or writing a
// meaningless state.
func isConnectionStatus(s string) bool {
	switch s {
	case "pending", "active", "declined", "completed", "cancelled":
		return true
	}

	return false
}

// AdminUpdateConnection allows an admin to override a connection's status.
// The status token is in legacy vocabulary — 'active'/'completed' map to
// the booking's 'confirmed'/'fulfilled'; anything outside the connection
// enum (incl. booking-only 'disputed'/'refunded') is a 400.
// PATCH /api/v1/admin/mentorship/connections/:id.
func (s *Service) AdminUpdateConnection(ctx context.Context, connID pgtype.UUID, status string) (Connection, error) {
	if s.bookings == nil {
		return Connection{}, errBackendUnavailable
	}

	if !isConnectionStatus(status) {
		return Connection{}, ErrInvalidStatusChange
	}

	// Kind-check: the admin route may only touch mentorship-kind bookings.
	// row carries the resolved booking id — connID may be a legacy
	// connection id (meta->>'connection_id' fallback).
	row, err := s.bookingRow(ctx, connID)
	if err != nil {
		return Connection{}, err
	}

	target := bookingStatus(status)
	if !servicedir.IsValidBookingStatus(target) {
		return Connection{}, ErrInvalidStatusChange
	}

	if _, err := s.bookings.AdminSetBookingStatus(ctx, row.ID, servicedir.AdminBookingActionInput{Status: target}); err != nil {
		return Connection{}, mapBookingErr(err)
	}

	updated, err := s.bookingRow(ctx, row.ID)
	if err != nil {
		return Connection{}, fmt.Errorf("loading updated connection: %w", err)
	}

	return connectionFromBookingRow(updated), nil
}

// UpdateMentorEmbedding sets the pgvector embedding for a mentor's bio.
// Called by the mentor:generate_embedding Asynq task handler.
func (s *Service) UpdateMentorEmbedding(ctx context.Context, userID pgtype.UUID, embedding []float32) error {
	vec := pgvector.NewVector(embedding)

	return s.write.UpdateMentorEmbedding(ctx, gen.UpdateMentorEmbeddingParams{
		UserID:    userID,
		Embedding: &vec,
	})
}

// GenerateEmbedding generates a pgvector embedding for a mentor's bio and
// stores it in user_profiles.mentor_embedding. Implements the
// worker.MentorProcessor interface — called by the mentor:generate_embedding
// Asynq task handler (§102, ADR-110).
func (s *Service) GenerateEmbedding(ctx context.Context, userID pgtype.UUID) error {
	if s.embedder == nil {
		slog.Warn("mentorship: GenerateEmbedding called with no embedder wired")
		return nil
	}

	// Fetch the mentor's bio text.
	profile, err := s.read.GetMentorProfile(ctx,

		userID,
	)
	if err != nil {
		return fmt.Errorf("fetching mentor profile for embedding: %w", err)
	}

	if profile.MentorBio == nil || *profile.MentorBio == "" {
		// No bio — nothing to embed. Clear any stale embedding.
		return s.write.UpdateMentorEmbedding(ctx, gen.UpdateMentorEmbeddingParams{
			UserID:    userID,
			Embedding: nil,
		})
	}

	embedding, err := s.embedder.GenerateEmbedding(ctx, *profile.MentorBio, "text-embedding-3-small")
	if err != nil {
		return fmt.Errorf("generating mentor bio embedding: %w", err)
	}

	return s.UpdateMentorEmbedding(ctx, userID, embedding)
}

// UpdateMeilisearch syncs a mentor's user document to the Meilisearch users
// index. Implements the worker.MentorProcessor interface — called by the
// mentor:update_meilisearch Asynq task handler (§102, ADR-110).
func (s *Service) UpdateMeilisearch(ctx context.Context, userID pgtype.UUID) error {
	if s.meilisearchIndexer == nil {
		slog.Warn("mentorship: UpdateMeilisearch called with no indexer wired")
		return nil
	}

	return s.meilisearchIndexer.IndexUser(ctx, userID.Bytes)
}

// enqueueGenerateEmbedding fires the mentor:generate_embedding Asynq task.
func (s *Service) enqueueGenerateEmbedding(ctx context.Context, userID pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"user_id": userID.String(),
	})
	if err != nil {
		slog.Warn("mentorship: marshal embedding task payload", "error", err)
		return
	}

	if err := s.enqueuer.Enqueue("mentor:generate_embedding", payload); err != nil {
		slog.Warn("mentorship: enqueue generate_embedding task", "error", err)
	}
}

// mentorSummaryFromRow builds a MentorSummary from common field values.
func mentorSummaryFromRow(userID pgtype.UUID, isMentor *bool, capacity, activeCount *int16, areas []string, bio *string, since pgtype.Timestamptz, displayName, avatarURL *string, username string, availableSlots int32, profession, websiteURL *string) MentorSummary {
	return MentorSummary{
		UserID:            userID,
		Username:          username,
		DisplayName:       displayName,
		AvatarURL:         avatarURL,
		MentorAreas:       areas,
		MentorBio:         bio,
		MentorSince:       since,
		MentorCapacity:    capacity,
		ActiveMenteeCount: activeCount,
		AvailableSlots:    availableSlots,
		Profession:        profession,
		WebsiteURL:        websiteURL,
	}
}

// connectionFromBookingRow maps a mentorship-kind service_bookings row to
// the legacy Connection shape (000824). Status vocabulary translates back
// (confirmed→active, fulfilled→completed); responded_at comes from
// confirmed_at on accepted rows and meta->>'responded_at' on declined
// rows (DeclineServiceBooking stamps it — the bookings table has no
// responded_at column). completed_at = fulfilled_at.
func connectionFromBookingRow(r servicedir.BookingRow) Connection {
	conn := Connection{
		ID:                 r.ID,
		MentorID:           r.ProviderUserID,
		MenteeID:           r.BookerUserID,
		Status:             connectionStatus(r.Status),
		DestinationSpaceID: r.DestinationSpaceID,
		RespondedAt:        r.ConfirmedAt,
		CompletedAt:        r.FulfilledAt,
		CancelledAt:        r.CancelledAt,
		CancelReason:       r.CancelReason,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
		MentorUsername:     r.ProviderUsername,
		MenteeUsername:     r.BookerUsername,
		MentorDisplayName:  r.ProviderDisplayName,
		MenteeDisplayName:  r.BookerDisplayName,
	}

	// goals: meta->>'goals' wins; the note column is the fallback for
	// migrated rows that only carry goals in note (same rule as
	// servicedir's bookingToDTO).
	goals := bookingMetaString(r.Meta, "goals")
	if goals == "" && r.Note != nil {
		goals = *r.Note
	}

	if goals != "" {
		conn.Goals = &goals
	}

	// responded_at on declined rows rides meta; accept already landed via
	// ConfirmedAt.
	if !conn.RespondedAt.Valid {
		conn.RespondedAt = bookingMetaTimestamp(r.Meta, "responded_at")
	}

	return conn
}

// bookingMetaTimestamp reads a timestamptz-ish key out of
// service_bookings.meta. jsonb_build_object renders timestamptz as ISO
// 8601 ("2026-10-09T12:34:56.789+00:00") — RFC3339Nano covers that shape;
// the pgtype scan is the fallback for the Postgres text layout.
func bookingMetaTimestamp(meta []byte, key string) pgtype.Timestamptz {
	raw := bookingMetaString(meta, key)
	if raw == "" {
		return pgtype.Timestamptz{}
	}

	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return pgtype.Timestamptz{Time: t, Valid: true}
	}

	var ts pgtype.Timestamptz
	if err := ts.Scan(raw); err == nil {
		return ts
	}

	return pgtype.Timestamptz{}
}
