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
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ErrProgramNotFound is returned when a mentorship program does not exist.
var ErrProgramNotFound = errors.New("mentorship program not found")

// Program represents a mentorship program.
type Program struct {
	ID          pgtype.UUID        `json:"id"`
	TenantID    pgtype.UUID        `json:"tenant_id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	IsActive    bool               `json:"is_active"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
}

// ProgramWithCounts is a mentorship program with aggregate mentor/mentee/match counts.
type ProgramWithCounts struct {
	ID          pgtype.UUID        `json:"id"`
	TenantID    pgtype.UUID        `json:"tenant_id"`
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
	ID             pgtype.UUID        `json:"id"`
	TenantID       pgtype.UUID        `json:"tenant_id"`
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
	read              *gen.Queries
	write             *gen.Queries
	enqueuer          TaskEnqueuer
	dmCreator         DMCreator
	settings          SettingsProvider
	embedder          Embedder
	meilisearchIndexer MeilisearchIndexer
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// ListPrograms returns all mentorship programs for the tenant with aggregate counts.
func (s *Service) ListPrograms(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]ProgramWithCounts, error) {
	rows, err := s.read.ListMentorshipProgramsWithCounts(ctx, gen.ListMentorshipProgramsWithCountsParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing mentorship programs: %w", err)
	}

	programs := make([]ProgramWithCounts, len(rows))
	for i, r := range rows {
		programs[i] = ProgramWithCounts(r)
	}

	return programs, nil
}

// GetProgram returns a single mentorship program by ID.
func (s *Service) GetProgram(ctx context.Context, tenantID, id pgtype.UUID) (Program, error) {
	row, err := s.read.GetMentorshipProgram(ctx, gen.GetMentorshipProgramParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return Program{}, fmt.Errorf("fetching mentorship program %s: %w", id, err)
	}

	return Program(row), nil
}

// CreateProgram creates a new mentorship program.
func (s *Service) CreateProgram(ctx context.Context, tenantID pgtype.UUID, name, description string, isActive bool) (Program, error) {
	row, err := s.write.CreateMentorshipProgram(ctx, gen.CreateMentorshipProgramParams{
		TenantID:    tenantID,
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
func (s *Service) UpdateProgram(ctx context.Context, tenantID, id pgtype.UUID, name, description *string, isActive *bool) (Program, error) {
	// Fetch current values for merge.
	current, err := s.GetProgram(ctx, tenantID, id)
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
		TenantID:    tenantID,
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

// ListMatches returns all mentor-mentee matches for the tenant with resolved names.
func (s *Service) ListMatches(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Match, error) {
	rows, err := s.read.ListMentorshipMatches(ctx, gen.ListMentorshipMatchesParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing mentorship matches: %w", err)
	}

	matches := make([]Match, len(rows))
	for i, r := range rows {
		matches[i] = Match(r)
	}

	return matches, nil
}

// UpdateMatch updates a mentor-mentee match status.
func (s *Service) UpdateMatch(ctx context.Context, tenantID, id pgtype.UUID, status string) (Match, error) {
	row, err := s.write.UpdateMentorshipMatch(ctx, gen.UpdateMentorshipMatchParams{
		TenantID: tenantID,
		ID:       id,
		Status:   status,
	})
	if err != nil {
		return Match{}, fmt.Errorf("updating mentorship match %s: %w", id, err)
	}

	return Match{
		ID:        row.ID,
		TenantID:  row.TenantID,
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
func (s *Service) DeleteProgram(ctx context.Context, tenantID, id pgtype.UUID) error {
	rowsAffected, err := s.write.DeleteMentorshipProgram(ctx, gen.DeleteMentorshipProgramParams{
		TenantID: tenantID,
		ID:       id,
	})
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
	GetSettingInt(ctx context.Context, tenantID pgtype.UUID, key string) (int, error)
}

// DMCreator opens a DM conversation between two users (§48 integration).
// The websocket.DMStore satisfies this. Used to auto-open a DM thread when
// a mentorship connection is accepted (ADR-110).
type DMCreator interface {
	GetOrCreateConversation(ctx context.Context, tenantID, participant1ID, participant2ID pgtype.UUID) (gen.DmConversation, error)
}

// Embedder generates pgvector embeddings for mentor bios via the AI gateway
// (§102, ADR-110). The ai.RAGEmbedder satisfies this.
type Embedder interface {
	GenerateEmbedding(ctx context.Context, text, model string) ([]float32, error)
}

// MeilisearchIndexer syncs a mentor's user document to the Meilisearch users
// index (§102). The search.Indexer satisfies this.
type MeilisearchIndexer interface {
	IndexUser(ctx context.Context, userID, tenantID [16]byte) error
}

// MentorProfile is the current user's mentor settings.
type MentorProfile struct {
	UserID            pgtype.UUID        `json:"user_id"`
	TenantID          pgtype.UUID        `json:"tenant_id"`
	IsMentor          *bool              `json:"is_mentor"`
	MentorCapacity    *int16             `json:"mentor_capacity"`
	ActiveMenteeCount *int16             `json:"active_mentee_count"`
	MentorAreas       []string           `json:"mentor_areas"`
	MentorBio         *string            `json:"mentor_bio"`
	MentorSince       pgtype.Timestamptz `json:"mentor_since"`
	DisplayName       *string            `json:"display_name"`
	AvatarURL         *string            `json:"avatar_url"`
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
}

// Connection is a mentorship connection with resolved user names.
type Connection struct {
	ID                 pgtype.UUID        `json:"id"`
	TenantID           pgtype.UUID        `json:"tenant_id"`
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
)

// SetEnqueuer wires the Asynq enqueuer for mentor:generate_embedding tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetDMCreator wires the DM creator for auto-opening DM threads on accept.
func (s *Service) SetDMCreator(d DMCreator) { s.dmCreator = d }

// SetSettingsProvider wires the system_settings reader for capacity validation.
func (s *Service) SetSettingsProvider(sp SettingsProvider) { s.settings = sp }

// SetEmbedder wires the AI embedder for mentor:generate_embedding tasks.
func (s *Service) SetEmbedder(e Embedder) { s.embedder = e }

// SetMeilisearchIndexer wires the Meilisearch indexer for mentor:update_meilisearch tasks.
func (s *Service) SetMeilisearchIndexer(mi MeilisearchIndexer) { s.meilisearchIndexer = mi }

// GetMentorProfile returns the current user's mentor profile settings.
// GET /api/v1/me/mentor-profile
func (s *Service) GetMentorProfile(ctx context.Context, tenantID, userID pgtype.UUID) (MentorProfile, error) {
	row, err := s.read.GetMentorProfile(ctx, gen.GetMentorProfileParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return MentorProfile{}, fmt.Errorf("getting mentor profile: %w", err)
	}

	return MentorProfile{
		UserID:            row.UserID,
		TenantID:          row.TenantID,
		IsMentor:          row.IsMentor,
		MentorCapacity:    row.MentorCapacity,
		ActiveMenteeCount: row.ActiveMenteeCount,
		MentorAreas:       row.MentorAreas,
		MentorBio:         row.MentorBio,
		MentorSince:       row.MentorSince,
		DisplayName:       row.DisplayName,
		AvatarURL:         row.AvatarUrl,
	}, nil
}

// UpdateMentorProfile toggles is_mentor and sets capacity/areas/bio.
// PATCH /api/v1/me/mentor-profile
// When mentor_bio changes, enqueues the mentor:generate_embedding task.
func (s *Service) UpdateMentorProfile(ctx context.Context, tenantID, userID pgtype.UUID, isMentor *bool, capacity *int16, areas []string, bio *string) (MentorProfile, error) {
	// Fetch current values for merge.
	current, err := s.read.GetMentorProfile(ctx, gen.GetMentorProfileParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return MentorProfile{}, fmt.Errorf("loading current mentor profile: %w", err)
	}

	// Merge: use new values where provided, otherwise keep current.
	mergedIsMentor := current.IsMentor
	if isMentor != nil {
		mergedIsMentor = isMentor
	}

	mergedCapacity := current.MentorCapacity
	if capacity != nil {
		mergedCapacity = capacity
		// Validate against platform-wide cap (Rule 3, §102 system_settings).
		if s.settings != nil && *mergedCapacity > 0 {
			maxCap, err := s.settings.GetSettingInt(ctx, tenantID, "mentorship.max_capacity")
			if err == nil && maxCap > 0 && int(*mergedCapacity) > maxCap {
				return MentorProfile{}, ErrCapacityExceeded
			}
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

	row, err := s.write.UpdateMentorProfile(ctx, gen.UpdateMentorProfileParams{
		IsMentor:       mergedIsMentor,
		MentorCapacity: mergedCapacity,
		MentorAreas:    mergedAreas,
		MentorBio:      mergedBio,
		TenantID:       tenantID,
		UserID:         userID,
	})
	if err != nil {
		return MentorProfile{}, fmt.Errorf("updating mentor profile: %w", err)
	}

	// Enqueue embedding generation if bio changed and mentor is opted in.
	if bioChanged && mergedIsMentor != nil && *mergedIsMentor && mergedBio != nil && *mergedBio != "" {
		s.enqueueGenerateEmbedding(ctx, tenantID, userID)
	}

	return MentorProfile{
		UserID:            row.UserID,
		TenantID:          row.TenantID,
		IsMentor:          row.IsMentor,
		MentorCapacity:    row.MentorCapacity,
		ActiveMenteeCount: row.ActiveMenteeCount,
		MentorAreas:       row.MentorAreas,
		MentorBio:         row.MentorBio,
		MentorSince:       row.MentorSince,
		DisplayName:       row.DisplayName,
		AvatarURL:         row.AvatarUrl,
	}, nil
}

// GetMentorByUsername returns a mentor's public profile by username.
// GET /api/v1/mentors/:username
func (s *Service) GetMentorByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (MentorSummary, error) {
	row, err := s.read.GetMentorByUsername(ctx, gen.GetMentorByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err != nil {
		return MentorSummary{}, ErrMentorNotFound
	}

	return mentorSummaryFromRow(row.UserID, row.TenantID, row.IsMentor, row.MentorCapacity,
		row.ActiveMenteeCount, row.MentorAreas, row.MentorBio, row.MentorSince,
		row.DisplayName, row.AvatarUrl, row.Username, 0), nil
}

// ListMentors discovers mentors with optional destination and area filters.
// GET /api/v1/mentors?dest=&areas=&sort=match|newest|capacity&page=
// When sort=match and a menteeEmbedding is provided, uses pgvector cosine
// similarity for bio-based match scoring (§102 ADR-110).
func (s *Service) ListMentors(ctx context.Context, tenantID pgtype.UUID, destSpaceID pgtype.UUID, areas []string, sort string, menteeEmbedding *pgvector.Vector, limit, offset int32) ([]MentorSummary, error) {
	// sort=match with an embedding uses the pgvector query.
	if sort == "match" && menteeEmbedding != nil {
		rows, err := s.read.ListMentorsByMatch(ctx, gen.ListMentorsByMatchParams{
			MenteeEmbedding: menteeEmbedding,
			TenantID:        tenantID,
			DestSpaceID:     destSpaceID,
			Areas:           areas,
			PageLimit:       limit,
			PageOffset:      offset,
		})
		if err != nil {
			return nil, fmt.Errorf("listing mentors by match: %w", err)
		}

		mentors := make([]MentorSummary, len(rows))
		for i, r := range rows {
			mentors[i] = mentorSummaryFromRow(r.UserID, r.TenantID, r.IsMentor, r.MentorCapacity,
				r.ActiveMenteeCount, r.MentorAreas, r.MentorBio, r.MentorSince,
				r.DisplayName, r.AvatarUrl, r.Username, r.AvailableSlots)
		}
		return mentors, nil
	}

	// sort=match without an embedding: fall back to keyword-only scoring
	// (§102: weight=1.0 on area_overlap_score when no bio embedding is
	// available). Fetch all mentors with the same filters, compute the
	// keyword overlap score in Go, sort by score desc, then apply
	// pagination.
	if sort == "match" {
		rows, err := s.read.ListMentors(ctx, gen.ListMentorsParams{
			TenantID:    tenantID,
			DestSpaceID: destSpaceID,
			Areas:       areas,
			Sort:        "newest",
			PageLimit:   limit,
			PageOffset:  offset,
		})
		if err != nil {
			return nil, fmt.Errorf("listing mentors for match scoring: %w", err)
		}

		mentors := make([]MentorSummary, len(rows))
		for i, r := range rows {
			mentors[i] = mentorSummaryFromRow(r.UserID, r.TenantID, r.IsMentor, r.MentorCapacity,
				r.ActiveMenteeCount, r.MentorAreas, r.MentorBio, r.MentorSince,
				r.DisplayName, r.AvatarUrl, r.Username, r.AvailableSlots)
		}

		// Compute keyword overlap score and sort by it.
		sortMentorsByKeywordMatch(mentors, areas)
		return mentors, nil
	}

	// Default: newest or capacity sort (no pgvector needed).
	rows, err := s.read.ListMentors(ctx, gen.ListMentorsParams{
		TenantID:    tenantID,
		DestSpaceID: destSpaceID,
		Areas:       areas,
		Sort:        sort,
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing mentors: %w", err)
	}

	mentors := make([]MentorSummary, len(rows))
	for i, r := range rows {
		mentors[i] = mentorSummaryFromRow(r.UserID, r.TenantID, r.IsMentor, r.MentorCapacity,
			r.ActiveMenteeCount, r.MentorAreas, r.MentorBio, r.MentorSince,
			r.DisplayName, r.AvatarUrl, r.Username, r.AvailableSlots)
	}

	return mentors, nil
}

// sortMentorsByKeywordMatch sorts mentors in-place by the §102 keyword
// overlap score: the fraction of the mentee's requested areas that appear
// in the mentor's mentor_areas. Mentors with no overlap keep their original
// relative order (stable sort).
func sortMentorsByKeywordMatch(mentors []MentorSummary, menteeAreas []string) {
	if len(menteeAreas) == 0 {
		return
	}

	// Build a lowercase set of mentee areas for case-insensitive matching.
	want := make(map[string]bool, len(menteeAreas))
	for _, a := range menteeAreas {
		want[strings.ToLower(strings.TrimSpace(a))] = true
	}

	score := func(m MentorSummary) float64 {
		if len(m.MentorAreas) == 0 {
			return 0
		}
		hits := 0
		for _, a := range m.MentorAreas {
			if want[strings.ToLower(strings.TrimSpace(a))] {
				hits++
			}
		}
		return float64(hits) / float64(len(want))
	}

	sort.SliceStable(mentors, func(i, j int) bool {
		return score(mentors[i]) > score(mentors[j])
	})
}

// GetMentorsByIDs returns up to 4 mentors for the comparison endpoint.
// GET /api/v1/compare/mentors?ids=a,b,c,d
func (s *Service) GetMentorsByIDs(ctx context.Context, tenantID pgtype.UUID, userIDs []pgtype.UUID) ([]MentorSummary, error) {
	rows, err := s.read.GetMentorsByIDs(ctx, gen.GetMentorsByIDsParams{
		TenantID: tenantID,
		UserIds:  userIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("getting mentors by IDs: %w", err)
	}

	mentors := make([]MentorSummary, len(rows))
	for i, r := range rows {
		mentors[i] = mentorSummaryFromRow(r.UserID, r.TenantID, r.IsMentor, r.MentorCapacity,
			r.ActiveMenteeCount, r.MentorAreas, r.MentorBio, r.MentorSince,
			r.DisplayName, r.AvatarUrl, r.Username, r.AvailableSlots)
	}

	return mentors, nil
}

// CreateConnection creates a new mentorship request (status = 'pending').
// POST /api/v1/mentorship/request
func (s *Service) CreateConnection(ctx context.Context, tenantID, mentorID, menteeID pgtype.UUID, goals *string, destSpaceID pgtype.UUID) (Connection, error) {
	if mentorID == menteeID {
		return Connection{}, ErrCannotSelfMentor
	}

	// §102: a new request can only be sent after the previous one has been
	// resolved. Check for an existing pending or active connection before
	// inserting. The UNIQUE(mentor_id, mentee_id, status) constraint only
	// prevents duplicate same-status rows — it does not prevent a new
	// pending request when an active connection already exists.
	if _, err := s.read.GetActiveOrPendingConnection(ctx, gen.GetActiveOrPendingConnectionParams{
		TenantID: tenantID,
		MentorID: mentorID,
		MenteeID: menteeID,
	}); err == nil {
		return Connection{}, ErrDuplicateRequest
	}

	conn, err := s.write.CreateMentorshipConnection(ctx, gen.CreateMentorshipConnectionParams{
		TenantID:           tenantID,
		MentorID:           mentorID,
		MenteeID:           menteeID,
		Goals:              goals,
		DestinationSpaceID: destSpaceID,
	})
	if err != nil {
		// Map unique constraint violation (23505) to ErrDuplicateRequest
		// so the handler can return 409 instead of 500 (Rule 5).
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Connection{}, ErrDuplicateRequest
		}
		return Connection{}, fmt.Errorf("creating mentorship connection: %w", err)
	}

	return connectionFromModel(conn), nil
}

// ListOutgoingConnections lists requests sent by the mentee.
// GET /api/v1/me/mentorship/outgoing
func (s *Service) ListOutgoingConnections(ctx context.Context, tenantID, menteeID pgtype.UUID, status string, limit, offset int32) ([]Connection, error) {
	rows, err := s.read.ListOutgoingConnections(ctx, gen.ListOutgoingConnectionsParams{
		TenantID:   tenantID,
		MenteeID:   menteeID,
		Status:     status,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing outgoing connections: %w", err)
	}

	conns := make([]Connection, len(rows))
	for i, r := range rows {
		conns[i] = Connection{
			ID:                 r.ID,
			TenantID:           r.TenantID,
			MentorID:           r.MentorID,
			MenteeID:           r.MenteeID,
			Status:             r.Status,
			Goals:              r.Goals,
			DestinationSpaceID: r.DestinationSpaceID,
			RespondedAt:        r.RespondedAt,
			CompletedAt:        r.CompletedAt,
			CancelledAt:        r.CancelledAt,
			CancelReason:       r.CancelReason,
			CreatedAt:          r.CreatedAt,
			UpdatedAt:          r.UpdatedAt,
			MentorUsername:     &r.MentorUsername,
			MentorDisplayName:  r.MentorDisplayName,
		}
	}

	return conns, nil
}

// CancelOutgoingConnection cancels a pending request by the mentee.
// DELETE /api/v1/me/mentorship/outgoing/:id
func (s *Service) CancelOutgoingConnection(ctx context.Context, tenantID, menteeID, connID pgtype.UUID) error {
	rowsAffected, err := s.write.CancelPendingConnection(ctx, gen.CancelPendingConnectionParams{
		TenantID: tenantID,
		ID:       connID,
		MenteeID: menteeID,
	})
	if err != nil {
		return fmt.Errorf("cancelling outgoing connection: %w", err)
	}

	if rowsAffected == 0 {
		return ErrConnectionNotFound
	}

	return nil
}

// ListIncomingConnections lists requests received by the mentor.
// GET /api/v1/me/mentorship/incoming
func (s *Service) ListIncomingConnections(ctx context.Context, tenantID, mentorID pgtype.UUID, status string, limit, offset int32) ([]Connection, error) {
	rows, err := s.read.ListIncomingConnections(ctx, gen.ListIncomingConnectionsParams{
		TenantID:   tenantID,
		MentorID:   mentorID,
		Status:     status,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing incoming connections: %w", err)
	}

	conns := make([]Connection, len(rows))
	for i, r := range rows {
		conns[i] = Connection{
			ID:                 r.ID,
			TenantID:           r.TenantID,
			MentorID:           r.MentorID,
			MenteeID:           r.MenteeID,
			Status:             r.Status,
			Goals:              r.Goals,
			DestinationSpaceID: r.DestinationSpaceID,
			RespondedAt:        r.RespondedAt,
			CompletedAt:        r.CompletedAt,
			CancelledAt:        r.CancelledAt,
			CancelReason:       r.CancelReason,
			CreatedAt:          r.CreatedAt,
			UpdatedAt:          r.UpdatedAt,
			MenteeUsername:     &r.MenteeUsername,
			MenteeDisplayName:  r.MenteeDisplayName,
		}
	}

	return conns, nil
}

// AcceptConnection accepts a pending request (pending → active).
// Only the mentor of the connection can accept. Also auto-opens a DM thread
// between mentor and mentee (ADR-110 §48 integration).
// PATCH /api/v1/me/mentorship/incoming/:id/accept
func (s *Service) AcceptConnection(ctx context.Context, tenantID, mentorID, connID pgtype.UUID) (Connection, error) {
	// Verify the current user is the mentor of this connection.
	existing, err := s.read.GetMentorshipConnection(ctx, gen.GetMentorshipConnectionParams{
		TenantID: tenantID,
		ID:       connID,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}
	if existing.MentorID != mentorID {
		return Connection{}, ErrNotAuthorized
	}

	conn, err := s.write.AcceptMentorshipConnection(ctx, gen.AcceptMentorshipConnectionParams{
		TenantID: tenantID,
		ID:       connID,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}

	// Auto-open DM thread (ADR-110: session communication via §48 DM).
	if s.dmCreator != nil {
		if _, dmErr := s.dmCreator.GetOrCreateConversation(ctx, tenantID, conn.MentorID, conn.MenteeID); dmErr != nil {
			slog.Warn("mentorship: auto-open DM thread on accept",
				"connection_id", connID, "error", dmErr)
			// Non-fatal — the connection is still accepted; the DM can be
			// opened manually later.
		}
	}

	// active_mentee_count changed (pending → active) → sync Meilisearch (§102).
	s.enqueueUpdateMeilisearch(ctx, tenantID, conn.MentorID)

	return connectionFromModel(conn), nil
}

// DeclineConnection declines a pending request (pending → declined).
// Only the mentor of the connection can decline.
// PATCH /api/v1/me/mentorship/incoming/:id/decline
func (s *Service) DeclineConnection(ctx context.Context, tenantID, mentorID, connID pgtype.UUID) (Connection, error) {
	// Verify the current user is the mentor of this connection.
	existing, err := s.read.GetMentorshipConnection(ctx, gen.GetMentorshipConnectionParams{
		TenantID: tenantID,
		ID:       connID,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}
	if existing.MentorID != mentorID {
		return Connection{}, ErrNotAuthorized
	}

	conn, err := s.write.DeclineMentorshipConnection(ctx, gen.DeclineMentorshipConnectionParams{
		TenantID: tenantID,
		ID:       connID,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}

	return connectionFromModel(conn), nil
}

// CompleteConnection completes an active connection (active → completed).
// Either party (mentor or mentee) can complete.
// PATCH /api/v1/mentorship/:id/complete
func (s *Service) CompleteConnection(ctx context.Context, tenantID, userID, connID pgtype.UUID) (Connection, error) {
	// Verify the current user is a participant in this connection.
	existing, err := s.read.GetMentorshipConnection(ctx, gen.GetMentorshipConnectionParams{
		TenantID: tenantID,
		ID:       connID,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}
	if existing.MentorID != userID && existing.MenteeID != userID {
		return Connection{}, ErrNotAuthorized
	}

	conn, err := s.write.CompleteMentorshipConnection(ctx, gen.CompleteMentorshipConnectionParams{
		TenantID: tenantID,
		ID:       connID,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}

	// active_mentee_count changed (active → completed) → sync Meilisearch (§102).
	s.enqueueUpdateMeilisearch(ctx, tenantID, conn.MentorID)

	return connectionFromModel(conn), nil
}

// CancelConnection cancels an active connection (active → cancelled).
// Either party (mentor or mentee) can cancel.
// PATCH /api/v1/mentorship/:id/cancel
func (s *Service) CancelConnection(ctx context.Context, tenantID, userID, connID pgtype.UUID, reason *string) (Connection, error) {
	// Verify the current user is a participant in this connection.
	existing, err := s.read.GetMentorshipConnection(ctx, gen.GetMentorshipConnectionParams{
		TenantID: tenantID,
		ID:       connID,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}
	if existing.MentorID != userID && existing.MenteeID != userID {
		return Connection{}, ErrNotAuthorized
	}

	conn, err := s.write.CancelMentorshipConnection(ctx, gen.CancelMentorshipConnectionParams{
		TenantID:     tenantID,
		ID:           connID,
		CancelReason: reason,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}

	// active_mentee_count changed (active → cancelled) → sync Meilisearch (§102).
	s.enqueueUpdateMeilisearch(ctx, tenantID, conn.MentorID)

	return connectionFromModel(conn), nil
}

// ListAllConnections lists all connections for admin (with filters).
// GET /api/v1/admin/mentorship/connections
func (s *Service) ListAllConnections(ctx context.Context, tenantID pgtype.UUID, status string, mentorID, menteeID pgtype.UUID, limit, offset int32) ([]Connection, error) {
	rows, err := s.read.ListAllConnections(ctx, gen.ListAllConnectionsParams{
		TenantID:   tenantID,
		Status:     status,
		MentorID:   mentorID,
		MenteeID:   menteeID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing all connections: %w", err)
	}

	conns := make([]Connection, len(rows))
	for i, r := range rows {
		conns[i] = Connection{
			ID:                 r.ID,
			TenantID:           r.TenantID,
			MentorID:           r.MentorID,
			MenteeID:           r.MenteeID,
			Status:             r.Status,
			Goals:              r.Goals,
			DestinationSpaceID: r.DestinationSpaceID,
			RespondedAt:        r.RespondedAt,
			CompletedAt:        r.CompletedAt,
			CancelledAt:        r.CancelledAt,
			CancelReason:       r.CancelReason,
			CreatedAt:          r.CreatedAt,
			UpdatedAt:          r.UpdatedAt,
			MentorUsername:     r.MentorUsername,
			MenteeUsername:     r.MenteeUsername,
		}
	}

	return conns, nil
}

// AdminUpdateConnection allows an admin to override a connection's status.
// PATCH /api/v1/admin/mentorship/connections/:id
func (s *Service) AdminUpdateConnection(ctx context.Context, tenantID, connID pgtype.UUID, status string) (Connection, error) {
	conn, err := s.write.AdminUpdateConnection(ctx, gen.AdminUpdateConnectionParams{
		TenantID: tenantID,
		ID:       connID,
		Status:   status,
	})
	if err != nil {
		return Connection{}, ErrConnectionNotFound
	}

	return connectionFromModel(conn), nil
}

// UpdateMentorEmbedding sets the pgvector embedding for a mentor's bio.
// Called by the mentor:generate_embedding Asynq task handler.
func (s *Service) UpdateMentorEmbedding(ctx context.Context, tenantID, userID pgtype.UUID, embedding []float32) error {
	vec := pgvector.NewVector(embedding)
	return s.write.UpdateMentorEmbedding(ctx, gen.UpdateMentorEmbeddingParams{
		TenantID: tenantID,
		UserID:   userID,
		Embedding: &vec,
	})
}

// GenerateEmbedding generates a pgvector embedding for a mentor's bio and
// stores it in user_profiles.mentor_embedding. Implements the
// worker.MentorProcessor interface — called by the mentor:generate_embedding
// Asynq task handler (§102, ADR-110).
func (s *Service) GenerateEmbedding(ctx context.Context, tenantID, userID pgtype.UUID) error {
	if s.embedder == nil {
		slog.Warn("mentorship: GenerateEmbedding called with no embedder wired")
		return nil
	}

	// Fetch the mentor's bio text.
	profile, err := s.read.GetMentorProfile(ctx, gen.GetMentorProfileParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return fmt.Errorf("fetching mentor profile for embedding: %w", err)
	}

	if profile.MentorBio == nil || *profile.MentorBio == "" {
		// No bio — nothing to embed. Clear any stale embedding.
		return s.write.UpdateMentorEmbedding(ctx, gen.UpdateMentorEmbeddingParams{
			TenantID:  tenantID,
			UserID:    userID,
			Embedding: nil,
		})
	}

	embedding, err := s.embedder.GenerateEmbedding(ctx, *profile.MentorBio, "text-embedding-3-small")
	if err != nil {
		return fmt.Errorf("generating mentor bio embedding: %w", err)
	}

	return s.UpdateMentorEmbedding(ctx, tenantID, userID, embedding)
}

// UpdateMeilisearch syncs a mentor's user document to the Meilisearch users
// index. Implements the worker.MentorProcessor interface — called by the
// mentor:update_meilisearch Asynq task handler (§102, ADR-110).
func (s *Service) UpdateMeilisearch(ctx context.Context, tenantID, userID pgtype.UUID) error {
	if s.meilisearchIndexer == nil {
		slog.Warn("mentorship: UpdateMeilisearch called with no indexer wired")
		return nil
	}

	return s.meilisearchIndexer.IndexUser(ctx, userID.Bytes, tenantID.Bytes)
}

// enqueueGenerateEmbedding fires the mentor:generate_embedding Asynq task.
func (s *Service) enqueueGenerateEmbedding(ctx context.Context, tenantID, userID pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
		"user_id":   userID.String(),
	})
	if err != nil {
		slog.Warn("mentorship: marshal embedding task payload", "error", err)
		return
	}

	if err := s.enqueuer.Enqueue("mentor:generate_embedding", payload); err != nil {
		slog.Warn("mentorship: enqueue generate_embedding task", "error", err)
	}
}

// enqueueUpdateMeilisearch fires the mentor:update_meilisearch Asynq task
// after a connection status change that affects the mentor's
// active_mentee_count (§102 spec: Meilisearch document updated whenever
// active_mentee_count changes).
func (s *Service) enqueueUpdateMeilisearch(ctx context.Context, tenantID, mentorID pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
		"user_id":   mentorID.String(),
	})
	if err != nil {
		slog.Warn("mentorship: marshal meilisearch task payload", "error", err)
		return
	}

	if err := s.enqueuer.Enqueue("mentor:update_meilisearch", payload); err != nil {
		slog.Warn("mentorship: enqueue update_meilisearch task", "error", err)
	}
}

// mentorSummaryFromRow builds a MentorSummary from common field values.
func mentorSummaryFromRow(userID, tenantID pgtype.UUID, isMentor *bool, capacity, activeCount *int16, areas []string, bio *string, since pgtype.Timestamptz, displayName, avatarURL *string, username string, availableSlots int32) MentorSummary {
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
	}
}

// connectionFromModel converts a gen.MentorshipConnection to Connection.
func connectionFromModel(c gen.MentorshipConnection) Connection {
	return Connection{
		ID:                 c.ID,
		TenantID:           c.TenantID,
		MentorID:           c.MentorID,
		MenteeID:           c.MenteeID,
		Status:             c.Status,
		Goals:              c.Goals,
		DestinationSpaceID: c.DestinationSpaceID,
		RespondedAt:        c.RespondedAt,
		CompletedAt:        c.CompletedAt,
		CancelledAt:        c.CancelledAt,
		CancelReason:       c.CancelReason,
		CreatedAt:          c.CreatedAt,
		UpdatedAt:          c.UpdatedAt,
	}
}
