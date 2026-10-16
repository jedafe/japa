package ai

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ProjectService owns ai_projects — named user collections grouping
// unified conversations and agent sessions, each carrying per-project
// instructions (merged into the thread's AI context server-side) and
// pinned picker defaults.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// (Amendment — AI Projects)

// ProjectStore is the subset of *gen.Queries the service needs.
// *gen.Queries satisfies it; tests stub it.
type ProjectStore interface {
	CreateAIProject(ctx context.Context, arg gen.CreateAIProjectParams) (gen.AiProject, error)
	GetAIProjectForUser(ctx context.Context, arg gen.GetAIProjectForUserParams) (gen.AiProject, error)
	ListAIProjectsForUser(ctx context.Context, arg gen.ListAIProjectsForUserParams) ([]gen.ListAIProjectsForUserRow, error)
	CountAIProjectsForUser(ctx context.Context, userID pgtype.UUID) (int64, error)
	UpdateAIProjectForUser(ctx context.Context, arg gen.UpdateAIProjectForUserParams) (gen.AiProject, error)
	SoftDeleteAIProjectForUser(ctx context.Context, arg gen.SoftDeleteAIProjectForUserParams) (pgtype.UUID, error)
	ClearAIConversationProjectForProject(ctx context.Context, projectID pgtype.UUID) error
	ClearAIAgentSessionProjectForProject(ctx context.Context, projectID pgtype.UUID) error
	SetAIConversationProject(ctx context.Context, arg gen.SetAIConversationProjectParams) (pgtype.UUID, error)
	SetAIAgentSessionProject(ctx context.Context, arg gen.SetAIAgentSessionProjectParams) (pgtype.UUID, error)
	SetAIAgentSessionsProjectByConversation(ctx context.Context, arg gen.SetAIAgentSessionsProjectByConversationParams) error
	// Linked-session check — a session attached to a conversation mirrors
	// the thread's project and can't be assigned independently.
	GetAgentSessionForUser(ctx context.Context, arg gen.GetAgentSessionForUserParams) (gen.AiAgentSession, error)
	// Voice pin validation — the voices catalog lives in personas.sql.
	GetPersonaVoiceByID(ctx context.Context, id pgtype.UUID) (gen.PersonaVoice, error)
}

// ErrProjectNotFound is returned by owner-scoped lookups/updates/deletes
// when the project doesn't exist or belongs to another user.
var ErrProjectNotFound = errors.New("project not found")

// ErrSessionNotFound is returned by session-scoped mutations when the
// agent session doesn't exist or belongs to another user.
var ErrSessionNotFound = errors.New("agent session not found")

// ErrProjectNameTaken is returned when a create/update collides with the
// per-user unique index on lower(name) for live projects.
var ErrProjectNameTaken = errors.New("project name already in use")

// ErrInvalidVoicePin is returned when default_voice_id doesn't name a row
// in the persona_voices catalog.
var ErrInvalidVoicePin = errors.New("default_voice_id is not a known voice")

// ErrSessionLinked is returned when a caller tries to assign a session
// that is linked to a conversation — its project mirrors the thread's and
// must be changed through the conversation's membership route.
var ErrSessionLinked = errors.New("session is linked to a conversation")

type ProjectService struct {
	store ProjectStore
	pool  *pgxpool.Pool // nil in tests — Delete then runs non-transactionally
}

func NewProjectService(store ProjectStore, pool *pgxpool.Pool) *ProjectService {
	return &ProjectService{store: store, pool: pool}
}

// ProjectInput is the create/update payload — empty strings clear fields.
type ProjectInput struct {
	Name           string
	Instructions   string
	DefaultModel   string
	DefaultTone    string
	DefaultVoiceID string
	DefaultTools   []byte // raw jsonb; nil/empty → '{}'
}

// validateVoicePin checks that a non-empty default_voice_id names a live
// persona_voices row. Malformed/nonexistent ids fail closed — a pinned
// voice the composer can't render is worse than rejecting the write.
func (s *ProjectService) validateVoicePin(ctx context.Context, voiceID string) error {
	if voiceID == "" {
		return nil
	}

	uid, err := scanUUID(voiceID)
	if err != nil || !uid.Valid {
		return ErrInvalidVoicePin
	}

	if _, err := s.store.GetPersonaVoiceByID(ctx, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidVoicePin
		}

		return fmt.Errorf("validating voice pin: %w", err)
	}

	return nil
}

// mapWriteErr translates DB constraint failures into service sentinels.
func mapWriteErr(err error, fallback string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrProjectNameTaken
	}

	return fmt.Errorf("%s: %w", fallback, err)
}

func (s *ProjectService) Create(ctx context.Context, userID pgtype.UUID, in ProjectInput) (gen.AiProject, error) {
	if err := s.validateVoicePin(ctx, in.DefaultVoiceID); err != nil {
		return gen.AiProject{}, err
	}

	tools := in.DefaultTools
	if len(tools) == 0 {
		tools = []byte("{}")
	}

	p, err := s.store.CreateAIProject(ctx, gen.CreateAIProjectParams{
		UserID:         userID,
		Name:           in.Name,
		Instructions:   in.Instructions,
		DefaultModel:   in.DefaultModel,
		DefaultTone:    in.DefaultTone,
		DefaultVoiceID: in.DefaultVoiceID,
		DefaultTools:   tools,
	})
	if err != nil {
		return gen.AiProject{}, mapWriteErr(err, "creating project")
	}

	return p, nil
}

func (s *ProjectService) Get(ctx context.Context, userID, projectID pgtype.UUID) (gen.AiProject, error) {
	p, err := s.store.GetAIProjectForUser(ctx, gen.GetAIProjectForUserParams{
		ID:     projectID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.AiProject{}, ErrProjectNotFound
		}

		return gen.AiProject{}, fmt.Errorf("loading project: %w", err)
	}

	return p, nil
}

func (s *ProjectService) List(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.ListAIProjectsForUserRow, int64, error) {
	rows, err := s.store.ListAIProjectsForUser(ctx, gen.ListAIProjectsForUserParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing projects: %w", err)
	}

	total, err := s.store.CountAIProjectsForUser(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting projects: %w", err)
	}

	return rows, total, nil
}

func (s *ProjectService) Update(ctx context.Context, userID, projectID pgtype.UUID, in ProjectInput) (gen.AiProject, error) {
	if err := s.validateVoicePin(ctx, in.DefaultVoiceID); err != nil {
		return gen.AiProject{}, err
	}

	tools := in.DefaultTools
	if len(tools) == 0 {
		tools = []byte("{}")
	}

	p, err := s.store.UpdateAIProjectForUser(ctx, gen.UpdateAIProjectForUserParams{
		ID:             projectID,
		UserID:         userID,
		Name:           in.Name,
		Instructions:   in.Instructions,
		DefaultModel:   in.DefaultModel,
		DefaultTone:    in.DefaultTone,
		DefaultVoiceID: in.DefaultVoiceID,
		DefaultTools:   tools,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.AiProject{}, ErrProjectNotFound
		}

		return gen.AiProject{}, mapWriteErr(err, "updating project")
	}

	return p, nil
}

// Delete soft-deletes the project (Rule 11) and un-groups its member
// conversations/sessions — FK SET NULL only fires on a hard delete, which
// never happens here. When a pool is available all three writes run in one
// transaction.
func (s *ProjectService) Delete(ctx context.Context, userID, projectID pgtype.UUID) error {
	do := func(st ProjectStore) error {
		if _, err := st.SoftDeleteAIProjectForUser(ctx, gen.SoftDeleteAIProjectForUserParams{
			ID:     projectID,
			UserID: userID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProjectNotFound
			}

			return fmt.Errorf("deleting project: %w", err)
		}

		if err := st.ClearAIConversationProjectForProject(ctx, projectID); err != nil {
			return fmt.Errorf("ungrouping conversations: %w", err)
		}

		return st.ClearAIAgentSessionProjectForProject(ctx, projectID)
	}

	if s.pool != nil {
		return db.WithTx(ctx, s.pool, func(q *gen.Queries) error { return do(q) })
	}

	return do(s.store)
}

// SetConversationProject assigns/unassigns a conversation to a project.
// projectID == nil unassigns. The project (when given) must be live and
// owned; the conversation must be live and owned — a foreign id on either
// side returns ErrProjectNotFound/ErrConversationNotFound without leaking
// existence.
func (s *ProjectService) SetConversationProject(ctx context.Context, userID, convID pgtype.UUID, projectID *pgtype.UUID) error {
	var pid pgtype.UUID

	if projectID != nil && projectID.Valid {
		if _, err := s.Get(ctx, userID, *projectID); err != nil {
			return err
		}

		pid = *projectID
	}

	if _, err := s.store.SetAIConversationProject(ctx, gen.SetAIConversationProjectParams{
		ID:        convID,
		UserID:    userID,
		ProjectID: pid,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConversationNotFound
		}

		return fmt.Errorf("assigning conversation project: %w", err)
	}
	// Cascade to the thread's sessions — their grouping mirrors the
	// conversation they audited; without this /ai/sessions would keep
	// showing the old project after a move.
	if err := s.store.SetAIAgentSessionsProjectByConversation(ctx, gen.SetAIAgentSessionsProjectByConversationParams{
		ConversationID: convID,
		ProjectID:      pid,
	}); err != nil {
		return fmt.Errorf("cascading project to sessions: %w", err)
	}

	return nil
}

// SetSessionProject assigns/unassigns an agent session — same ownership
// contract as SetConversationProject. Sessions linked to a conversation
// mirror that thread's membership (SetConversationProject cascades), so
// direct assignment on them is rejected with ErrSessionLinked — move the
// conversation instead.
func (s *ProjectService) SetSessionProject(ctx context.Context, userID, sessionID pgtype.UUID, projectID *pgtype.UUID) error {
	var pid pgtype.UUID

	if projectID != nil && projectID.Valid {
		if _, err := s.Get(ctx, userID, *projectID); err != nil {
			return err
		}

		pid = *projectID
	}

	sess, err := s.store.GetAgentSessionForUser(ctx, gen.GetAgentSessionForUserParams{
		ID:     sessionID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}

		return fmt.Errorf("loading session: %w", err)
	}

	if sess.ConversationID.Valid {
		return ErrSessionLinked
	}

	if _, err := s.store.SetAIAgentSessionProject(ctx, gen.SetAIAgentSessionProjectParams{
		ID:        sessionID,
		UserID:    userID,
		ProjectID: pid,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}

		return fmt.Errorf("assigning session project: %w", err)
	}

	return nil
}

// InstructionsFor returns the project's instructions for a resolved
// conversation — the string the unified loop merges into the request
// context as `project_instructions`. Empty when the thread is ungrouped
// or the project row is gone/deleted.
func (s *ProjectService) InstructionsFor(ctx context.Context, userID pgtype.UUID, conv gen.AiConversation) string {
	if !conv.ProjectID.Valid {
		return ""
	}

	p, err := s.Get(ctx, userID, conv.ProjectID)
	if err != nil {
		return ""
	}

	return p.Instructions
}
