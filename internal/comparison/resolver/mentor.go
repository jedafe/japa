package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/mentorship"
)

// mentorResolver resolves mentorship users.
type mentorResolver struct {
	svc *mentorship.Service
}

// NewMentorResolver creates a resolver for mentors.
func NewMentorResolver(svc *mentorship.Service) Resolver {
	return &mentorResolver{svc: svc}
}

func (r *mentorResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		mentors, err := r.svc.GetMentorsByIDs(ctx, tenantID, []pgtype.UUID{id})
		if err != nil {
			return ResolvedItem{}, false, fmt.Errorf("fetching mentor by id: %w", err)
		}
		if len(mentors) == 0 {
			return ResolvedItem{}, false, nil
		}
		return mentorToResolvedItem(mentors[0]), true, nil
	}

	if username, ok := ref["username"].(string); ok && username != "" {
		m, err := r.svc.GetMentorByUsername(ctx, tenantID, username)
		if err != nil {
			if errors.Is(err, mentorship.ErrMentorNotFound) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching mentor by username: %w", err)
		}
		return mentorToResolvedItem(m), true, nil
	}

	return ResolvedItem{}, false, errors.New("mentor item_ref missing id or username")
}

func (r *mentorResolver) Search(ctx context.Context, query string, _ map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	if offset < 0 {
		offset = 0
	}

	mentors, err := r.svc.SearchMentors(ctx, tenantID, query, limit+offset)
	if err != nil {
		return nil, fmt.Errorf("searching mentors: %w", err)
	}

	if int(offset) >= len(mentors) {
		return []ResolvedItem{}, nil
	}
	end := int(offset) + int(limit)
	if end > len(mentors) {
		end = len(mentors)
	}

	out := make([]ResolvedItem, 0, end-int(offset))
	for _, m := range mentors[offset:end] {
		out = append(out, mentorToResolvedItem(m))
	}
	return out, nil
}

func mentorToResolvedItem(m mentorship.MentorSummary) ResolvedItem {
	title := m.Username
	if m.DisplayName != nil && *m.DisplayName != "" {
		title = *m.DisplayName
	}
	desc := deref(m.MentorBio)
	areas := m.MentorAreas
	if areas == nil {
		areas = []string{}
	}
	meta := map[string]any{
		"username":     m.Username,
		"areas":        areas,
		"score_inputs": mentorScoreInputs(m),
	}
	if len(m.MentorAreas) > 0 {
		meta["tags"] = m.MentorAreas
	}
	return ResolvedItem{
		Type:        "mentor",
		ID:          m.UserID.String(),
		Title:       title,
		Description: Excerpt(desc, 160),
		Metadata:    meta,
	}
}

func mentorScoreInputs(m mentorship.MentorSummary) map[string]any {
	areas := m.MentorAreas
	if areas == nil {
		areas = []string{}
	}

	var sessionsCount any
	if m.ActiveMenteeCount != nil {
		sessionsCount = int(*m.ActiveMenteeCount)
	}

	return map[string]any{
		"cost":              nil,
		"rating":            nil,
		"sessions_count":    sessionsCount,
		"areas":             areas,
		"language":          []string{},
		"availability_days": nil,
	}
}
