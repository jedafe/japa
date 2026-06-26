package forum

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CreatePollInput holds parameters for creating a new forum poll.
type CreatePollInput struct {
	ThreadID      pgtype.UUID
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

// CreatePoll creates a new poll for a thread with its options.
func (s *Service) CreatePoll(ctx context.Context, input CreatePollInput) (gen.ForumPoll, error) {
	if input.Question == "" {
		return gen.ForumPoll{}, fmt.Errorf("poll question is required")
	}
	if len(input.Options) < 2 {
		return gen.ForumPoll{}, fmt.Errorf("poll requires at least 2 options")
	}

	poll, err := s.store.createPoll(ctx, gen.CreateForumPollParams{
		ThreadID:      input.ThreadID,
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
			pos = int32(i)
		}
		if _, err := s.store.addPollOption(ctx, gen.AddForumPollOptionParams{
			PollID:   poll.ID,
			Label:    opt.Label,
			Position: pos,
		}); err != nil {
			return gen.ForumPoll{}, fmt.Errorf("adding poll option: %w", err)
		}
	}

	return poll, nil
}

// VotePoll casts a vote on a poll option. One vote per user per poll.
func (s *Service) VotePoll(ctx context.Context, pollID, optionID, userID pgtype.UUID) error {
	// Check if the poll exists and is not closed.
	poll, err := s.store.getPollByID(ctx, pollID)
	if err != nil {
		return fmt.Errorf("getting poll: %w", err)
	}
	if poll.CloseAt.Valid && time.Now().After(poll.CloseAt.Time) {
		return fmt.Errorf("poll is closed")
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
	return nil
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

// GetPollByThreadID returns the poll for a given thread.
func (s *Service) GetPollByThreadID(ctx context.Context, threadID pgtype.UUID) (gen.ForumPoll, error) {
	return s.store.getPollByThreadID(ctx, threadID)
}

// GetUserPollVote returns the user's vote on a poll.
func (s *Service) GetUserPollVote(ctx context.Context, pollID, userID pgtype.UUID) (gen.ForumPollVote, error) {
	return s.store.getPollUserVote(ctx, pollID, userID)
}
