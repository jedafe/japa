// Package personas implements the AI Persona System (Section 68, ADR-068).
//
// It provides transactional mass provisioning of AI personas (backing user +
// professional profile byline + ai_personas config row in one tx), CRUD over
// the config, and an activity-log reader backed by platform_events.
//
// The package follows the layered architecture: Store wraps the sqlc-generated
// queries with read/write pool separation; Service holds business logic and
// transaction boundaries; Handlers parse/validate input and render responses.
//
// Ref: japaconnect-engineering/sections/68-ai-personas.md
//
//	japaconnect-engineering/adr/ADR-068-ai-persona-system.md
package personas

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated queries for ai_personas operations.
// All writes go through the write pool; all reads through the read pool.
// Tenant scoping is enforced at the query level (every query filters on
// tenant_id).
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates a personas store bound to the provided database pools.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}

// DB exposes the underlying pool for transactional service methods.
func (s *Store) DB() *db.DB { return s.db }

// ---------------------------------------------------------------------------
// Persona provisioning (transactional — called from Service.ProvisionPersonas)
// ---------------------------------------------------------------------------

// CreatePersonaUser creates a persona backing account (is_bot = true). Used
// inside the transactional provisioning flow — the caller wraps this with
// profile and config creation in a single tx.
func (s *Store) CreatePersonaUser(ctx context.Context, q *gen.Queries, tenantID pgtype.UUID, email, username string) (gen.User, error) {
	row, err := q.CreatePersonaUser(ctx, gen.CreatePersonaUserParams{
		TenantID: tenantID,
		Email:    email,
		Username: username,
	})
	if err != nil {
		return gen.User{}, fmt.Errorf("personas: create persona user: %w", err)
	}
	return row, nil
}

// CreateProfessionalProfile creates the byline profile for a persona. Used
// inside the transactional provisioning flow.
func (s *Store) CreateProfessionalProfile(ctx context.Context, q *gen.Queries, params gen.CreateProfessionalProfileParams) (gen.ProfessionalProfile, error) {
	row, err := q.CreateProfessionalProfile(ctx, params)
	if err != nil {
		return gen.ProfessionalProfile{}, fmt.Errorf("personas: create professional profile: %w", err)
	}
	return row, nil
}

// CreateAIPersona creates the ai_personas config row linking the backing user
// to its professional profile byline. Used inside the transactional
// provisioning flow.
func (s *Store) CreateAIPersona(ctx context.Context, q *gen.Queries, params gen.CreateAIPersonaParams) (gen.AiPersona, error) {
	row, err := q.CreateAIPersona(ctx, params)
	if err != nil {
		return gen.AiPersona{}, fmt.Errorf("personas: create ai persona: %w", err)
	}
	return row, nil
}

// GetProfileTypeBySlug looks up a professional_profile_type by slug. Used to
// resolve the default "contributor"/"columnist" type when no type_id is
// supplied in the provisioning input.
func (s *Store) GetProfileTypeBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.ProfessionalProfileType, error) {
	return s.read.GetProfessionalProfileTypeBySlug(ctx, gen.GetProfessionalProfileTypeBySlugParams{
		Slug:     slug,
		TenantID: tenantID,
	})
}

// ---------------------------------------------------------------------------
// Persona CRUD
// ---------------------------------------------------------------------------

// GetAIPersonaByID retrieves a single persona by ID (with joined user +
// profile names).
func (s *Store) GetAIPersonaByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.GetAIPersonaByIDRow, error) {
	return s.read.GetAIPersonaByID(ctx, gen.GetAIPersonaByIDParams{
		TenantID: tenantID, ID: id,
	})
}

// ListAIPersonas lists personas for a tenant with pagination, optional status
// filter, search query, and sort. An empty status returns all statuses; an
// empty q matches all rows; sort/dir default to created_at DESC.
func (s *Store) ListAIPersonas(ctx context.Context, tenantID pgtype.UUID, status, q, sort, dir string, limit, offset int32) ([]gen.ListAIPersonasRow, error) {
	return s.read.ListAIPersonas(ctx, gen.ListAIPersonasParams{
		TenantID:   tenantID,
		Status:     status,
		Q:          q,
		Sort:       sort,
		Dir:        dir,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

// CountAIPersonas counts personas for a tenant with the same status and search
// filters applied to ListAIPersonas.
func (s *Store) CountAIPersonas(ctx context.Context, tenantID pgtype.UUID, status, q string) (int64, error) {
	return s.read.CountAIPersonas(ctx, gen.CountAIPersonasParams{
		TenantID: tenantID, Status: status, Q: q,
	})
}

// UpdateAIPersona updates a persona's config (activity_policy, modes, status,
// voice_prompt). Nil pointer fields are left unchanged (COALESCE).
func (s *Store) UpdateAIPersona(ctx context.Context, params gen.UpdateAIPersonaParams) (gen.AiPersona, error) {
	row, err := s.write.UpdateAIPersona(ctx, params)
	if err != nil {
		return gen.AiPersona{}, fmt.Errorf("personas: update ai persona: %w", err)
	}
	return row, nil
}

// GetAIPersonaByUserID retrieves a persona by its backing user_id. Used by
// the orchestration engine to resolve the persona config from the actor
// identity.
func (s *Store) GetAIPersonaByUserID(ctx context.Context, tenantID, userID pgtype.UUID) (gen.AiPersona, error) {
	return s.read.GetAIPersonaByUserID(ctx, gen.GetAIPersonaByUserIDParams{
		TenantID: tenantID, UserID: userID,
	})
}

// ---------------------------------------------------------------------------
// Persona activity log
// ---------------------------------------------------------------------------

// ListPersonaActivity returns paginated platform_events for a persona's
// backing user (is_synthetic = true only).
func (s *Store) ListPersonaActivity(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.ListPersonaActivityRow, error) {
	return s.read.ListPersonaActivity(ctx, gen.ListPersonaActivityParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

// CountPersonaActivity counts platform_events for a persona's backing user
// (is_synthetic = true only).
func (s *Store) CountPersonaActivity(ctx context.Context, tenantID, userID pgtype.UUID) (int64, error) {
	return s.read.CountPersonaActivity(ctx, gen.CountPersonaActivityParams{
		TenantID: tenantID, UserID: userID,
	})
}
