package forum

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// PollInput holds parameters for creating a new forum poll.
type PollInput struct {
	Question      string
	IsMultiSelect bool
	CloseAt       pgtype.Timestamptz
	Options       []PollOptionInput
}

// PollOptionInput holds a single poll option.
type PollOptionInput struct {
	Label    string
	Position int32
}

// PollResult aggregates a poll option with its vote count.
type PollResult struct {
	OptionID  pgtype.UUID `json:"option_id"`
	Label     string      `json:"label"`
	Position  int32       `json:"position"`
	VoteCount int32       `json:"vote_count"`
}

// VotePoll casts a vote on a poll option.
// Single-select polls replace any prior user vote; multi-select polls add this
// option to the user's existing selections (BUG-002).
func (s *Service) VotePoll(ctx context.Context, pollID, optionID, userID pgtype.UUID) error {
	// Check if the poll exists and is not closed.
	poll, err := s.store.getPollByID(ctx, pollID)
	if err != nil {
		return fmt.Errorf("getting poll: %w", err)
	}

	if poll.CloseAt.Valid && time.Now().After(poll.CloseAt.Time) {
		return errors.New("poll is closed")
	}

	// Single-select: clear the user's previous choice so only the new option
	// is recorded. Multi-select: keep the user's other selections.
	if !poll.IsMultiSelect {
		if err := s.store.deletePollVotesByUser(ctx, pollID, userID); err != nil {
			return fmt.Errorf("clearing prior single-select vote: %w", err)
		}
	}

	// Verify the option belongs to this poll via FK constraint (will fail if invalid).
	_, err = s.store.votePoll(ctx, gen.VoteForumPollParams{
		PollID:   pollID,
		OptionID: optionID,
		UserID:   userID,
	})
	if err != nil {
		return fmt.Errorf("voting on poll: %w", err)
	}

	// Live update: broadcast the new results to the space's WS room.
	s.broadcastPollResults(ctx, poll)

	return nil
}

// VotePollByOptionID casts a vote on the poll that owns the given option.
// Used by POST /poll-options/{id}/vote where the URL param is the option_id,
// not the poll_id. Ref: api-routes-catalog L612.
func (s *Service) VotePollByOptionID(ctx context.Context, optionID, userID pgtype.UUID) error {
	poll, err := s.store.getPollByOptionID(ctx, optionID)
	if err != nil {
		return fmt.Errorf("resolving poll from option: %w", err)
	}

	return s.VotePoll(ctx, poll.ID, optionID, userID)
}

// RetractPollVote removes the user's vote for a specific option
// (DELETE /poll-options/{id}/vote). Ref: api-routes-catalog L613.
func (s *Service) RetractPollVote(ctx context.Context, optionID, userID pgtype.UUID) error {
	poll, err := s.store.getPollByOptionID(ctx, optionID)
	if err != nil {
		return fmt.Errorf("resolving poll from option: %w", err)
	}

	if err := s.store.retractPollVote(ctx, optionID, userID); err != nil {
		return fmt.Errorf("retracting poll vote: %w", err)
	}

	s.broadcastPollResults(ctx, poll)

	return nil
}

// broadcastPollResults publishes the current poll results to the space's
// WebSocket room so connected clients see live vote counts. Ref: Section 10.
func (s *Service) broadcastPollResults(ctx context.Context, poll gen.ForumPoll) {
	spaceID, err := s.store.getThreadSpaceByID(ctx, poll.ThreadID)
	if err != nil {
		slog.Warn("forum: failed to resolve thread for poll broadcast", "thread_id", poll.ThreadID, "error", err)
		return
	}

	results, err := s.GetPollResults(ctx, poll.ID)
	if err != nil {
		return
	}

	s.broadcastSpace(pgtype.UUID{}, spaceID, "poll_updated", map[string]any{
		"poll_id": poll.ID, "results": results,
	})
}

// GetPollResults returns aggregated vote counts for a poll.
func (s *Service) GetPollResults(ctx context.Context, pollID pgtype.UUID) ([]PollResult, error) {
	rows, err := s.store.getPollResults(ctx, pollID)
	if err != nil {
		return nil, fmt.Errorf("getting poll results: %w", err)
	}

	results := make([]PollResult, len(rows))
	for i, r := range rows {
		results[i] = PollResult{
			OptionID:  r.OptionID,
			Label:     r.Label,
			Position:  r.Position,
			VoteCount: r.VoteCount,
		}
	}

	return results, nil
}

// GetPollByID returns a poll by its primary key.
func (s *Service) GetPollByID(ctx context.Context, pollID pgtype.UUID) (gen.ForumPoll, error) {
	return s.store.getPollByID(ctx, pollID)
}

// GetPollByThreadID returns the poll for a given thread.
func (s *Service) GetPollByThreadID(ctx context.Context, threadID pgtype.UUID) (gen.ForumPoll, error) {
	return s.store.getPollByThreadID(ctx, threadID)
}

// GetPollByOptionID resolves a poll from one of its option IDs.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (BUG-006).
func (s *Service) GetPollByOptionID(ctx context.Context, optionID pgtype.UUID) (gen.ForumPoll, error) {
	return s.store.getPollByOptionID(ctx, optionID)
}

// GetUserPollVotes returns the user's selected option IDs for a poll.
// Multi-select polls can return more than one row.
func (s *Service) GetUserPollVotes(ctx context.Context, pollID, userID pgtype.UUID) ([]gen.ForumPollVote, error) {
	return s.store.listPollUserVotes(ctx, pollID, userID)
}

// CreatePollForThread adds a poll to an existing forum thread.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Polls).
func (s *Service) CreatePollForThread(ctx context.Context, threadID, tenantID, userID pgtype.UUID, input PollInput) (gen.ForumPoll, error) {
	if input.Question == "" {
		return gen.ForumPoll{}, errors.New("poll question is required")
	}

	if len(input.Options) < 2 {
		return gen.ForumPoll{}, errors.New("poll requires at least 2 options")
	}

	for i, opt := range input.Options {
		if opt.Label == "" {
			return gen.ForumPoll{}, fmt.Errorf("poll option %d label is required", i+1)
		}
	}

	// Verify the thread exists and belongs to the tenant.
	thread, err := s.store.getThreadByID(ctx, threadID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.ForumPoll{}, errpkg.ErrNotFound
		}
		return gen.ForumPoll{}, fmt.Errorf("getting thread for poll: %w", err)
	}

	// A thread can only have one poll.
	_, err = s.store.getPollByThreadID(ctx, thread.ID)
	if err == nil {
		return gen.ForumPoll{}, fmt.Errorf("%w: thread already has a poll", errpkg.ErrAlreadyExists)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return gen.ForumPoll{}, fmt.Errorf("checking existing poll: %w", err)
	}

	// Only the thread author or a moderator may attach a poll.
	if thread.AuthorUserID != userID && !s.canModerateThread(ctx, thread, userID, tenantID, "forum.thread.moderate") {
		return gen.ForumPoll{}, fmt.Errorf("%w: only the author or a moderator may add a poll", errpkg.ErrForbidden)
	}

	poll, err := db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (gen.ForumPoll, error) {
		p, err := q.CreateForumPoll(ctx, gen.CreateForumPollParams{
			ThreadID:      thread.ID,
			Question:      input.Question,
			IsMultiSelect: input.IsMultiSelect,
			CloseAt:       input.CloseAt,
		})
		if err != nil {
			return gen.ForumPoll{}, fmt.Errorf("creating forum poll: %w", err)
		}

		for i, opt := range input.Options {
			pos := opt.Position
			if pos == 0 {
				pos = int32(i) //nolint:gosec // options count is bounded
			}
			if _, err := q.AddForumPollOption(ctx, gen.AddForumPollOptionParams{
				PollID:   p.ID,
				Label:    opt.Label,
				Position: pos,
			}); err != nil {
				return gen.ForumPoll{}, fmt.Errorf("adding poll option: %w", err)
			}
		}

		return p, nil
	})
	if err != nil {
		return gen.ForumPoll{}, err
	}

	s.broadcastSpace(tenantID, thread.SpaceID, "poll_created", map[string]any{
		"poll_id":    poll.ID,
		"thread_id":  thread.ID,
		"created_by": userID,
	})

	return poll, nil
}
