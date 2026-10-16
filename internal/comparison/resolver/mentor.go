package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/mentorship"
)

// mentorResolver resolves mentorship users.
type mentorResolver struct {
	svc     *mentorship.Service
	queries *gen.Queries // optional: enables custom_field_values merge
}

// NewMentorResolver creates a resolver for mentors. Pass WithQueries to
// merge custom_field_values into field_values (§80 Tool Surface Hydration).
func NewMentorResolver(svc *mentorship.Service, opts ...Option) Resolver {
	o := newResolverOptions(opts...)

	return &mentorResolver{svc: svc, queries: o.queries}
}

func (r *mentorResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}

		mentors, err := r.svc.GetMentorsByIDs(ctx, []pgtype.UUID{id})
		if err != nil {
			return ResolvedItem{}, false, fmt.Errorf("fetching mentor by id: %w", err)
		}

		if len(mentors) == 0 {
			return ResolvedItem{}, false, nil
		}

		return mentorToResolvedItem(mentors[0], r.mentorFieldValues(ctx, mentors[0].UserID)), true, nil
	}

	if username, ok := ref["username"].(string); ok && username != "" {
		m, err := r.svc.GetMentorByUsername(ctx, username)
		if err != nil {
			if errors.Is(err, mentorship.ErrMentorNotFound) {
				return ResolvedItem{}, false, nil
			}

			return ResolvedItem{}, false, fmt.Errorf("fetching mentor by username: %w", err)
		}

		return mentorToResolvedItem(m, r.mentorFieldValues(ctx, m.UserID)), true, nil
	}

	return ResolvedItem{}, false, errors.New("mentor item_ref missing id or username")
}

// mentorFieldValues fetches a mentor's community field values (entity_type
// 'mentor', entity_id = user_id) for the field_values overlay (§80).
func (r *mentorResolver) mentorFieldValues(ctx context.Context, userID pgtype.UUID) map[string]any {
	return customfields.EntityFieldValues(ctx, r.queries, "mentor", userID)
}

func (r *mentorResolver) Search(ctx context.Context, query string, _ map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	mentors, err := r.svc.SearchMentors(ctx, query, limit+offset)
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

	page := mentors[offset:end]

	// One batched cfv read for the page — no per-mentor queries (§80).
	var cfvByMentor map[pgtype.UUID]map[string]any

	if r.queries != nil && len(page) > 0 {
		ids := make([]pgtype.UUID, 0, len(page))

		for _, m := range page {
			if m.UserID.Valid {
				ids = append(ids, m.UserID)
			}
		}

		cfvByMentor = customfields.EntityFieldValuesForEntities(ctx, r.queries, "mentor", ids)
	}

	out := make([]ResolvedItem, 0, len(page))
	for _, m := range page {
		out = append(out, mentorToResolvedItem(m, cfvByMentor[m.UserID]))
	}

	return out, nil
}

func mentorToResolvedItem(m mentorship.MentorSummary, cfv map[string]any) ResolvedItem {
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

	// §80 Tool Surface Hydration: mentors have no JSONB field bag, so
	// community field values project as field_values.
	if len(cfv) > 0 {
		meta["field_values"] = cfv
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
