package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

		return mentorToResolvedItem(mentors[0], r.mentorFieldValues(ctx, mentors[0].UserID), r.mentorProfileFieldValues(ctx, mentors[0].UserID)), true, nil
	}

	if username, ok := ref["username"].(string); ok && username != "" {
		m, err := r.svc.GetMentorByUsername(ctx, username)
		if err != nil {
			if errors.Is(err, mentorship.ErrMentorNotFound) {
				return ResolvedItem{}, false, nil
			}

			return ResolvedItem{}, false, fmt.Errorf("fetching mentor by username: %w", err)
		}

		return mentorToResolvedItem(m, r.mentorFieldValues(ctx, m.UserID), r.mentorProfileFieldValues(ctx, m.UserID)), true, nil
	}

	return ResolvedItem{}, false, errors.New("mentor item_ref missing id or username")
}

// mentorFieldValues fetches a mentor's community field values (entity_type
// 'mentor', entity_id = user_id) for the field_values overlay (§80).
func (r *mentorResolver) mentorFieldValues(ctx context.Context, userID pgtype.UUID) map[string]any {
	return customfields.EntityFieldValues(ctx, r.queries, "mentor", userID)
}

// mentorProfileFieldValues fetches the mentor's user_profile cfv bucket
// (entity_type 'user_profile', entity_id = user_profiles.id — ADR-064a).
// The bucket supplies geo fields like destination_country to the §102
// comparison fields; lookups are best-effort and never fail a resolve.
func (r *mentorResolver) mentorProfileFieldValues(ctx context.Context, userID pgtype.UUID) map[string]any {
	if r.queries == nil || !userID.Valid {
		return nil
	}

	profileID, err := r.queries.GetUserProfileID(ctx, userID)
	if err != nil || !profileID.Valid {
		return nil
	}

	return customfields.EntityFieldValues(ctx, r.queries, "user_profile", profileID)
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

	var profileFVByMentor map[pgtype.UUID]map[string]any

	if r.queries != nil && len(page) > 0 {
		ids := make([]pgtype.UUID, 0, len(page))

		for _, m := range page {
			if m.UserID.Valid {
				ids = append(ids, m.UserID)
			}
		}

		cfvByMentor = customfields.EntityFieldValuesForEntities(ctx, r.queries, "mentor", ids)

		// §102 'Destination' — destination_country lives in the user_profile
		// cfv bucket keyed by profile id (ADR-064a), so resolve profile ids
		// first, then batch-read and map back to user ids.
		if profileRows, err := r.queries.GetUserProfileIDsByUserIDs(ctx, ids); err == nil && len(profileRows) > 0 {
			profileIDs := make([]pgtype.UUID, 0, len(profileRows))
			userByProfile := make(map[pgtype.UUID]pgtype.UUID, len(profileRows))

			for _, row := range profileRows {
				profileIDs = append(profileIDs, row.ID)
				userByProfile[row.ID] = row.UserID
			}

			fvByProfile := customfields.EntityFieldValuesForEntities(ctx, r.queries, "user_profile", profileIDs)
			profileFVByMentor = make(map[pgtype.UUID]map[string]any, len(fvByProfile))

			for profileID, fv := range fvByProfile {
				profileFVByMentor[userByProfile[profileID]] = fv
			}
		}
	}

	out := make([]ResolvedItem, 0, len(page))
	for _, m := range page {
		out = append(out, mentorToResolvedItem(m, cfvByMentor[m.UserID], profileFVByMentor[m.UserID]))
	}

	return out, nil
}

func mentorToResolvedItem(m mentorship.MentorSummary, cfv, profileFV map[string]any) ResolvedItem {
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

	// §102 comparison fields: the mentor profile is columnar (user_profiles
	// columns, no JSONB field bag), so the resolver projects the spec'd keys
	// into field_values itself. The user_profile cfv bucket supplies
	// destination_country (ADR-064a); 'mentor' cfv rows overlay both —
	// canonical values win per key (§80/CF11).
	fv := mentorBaseFieldValues(m)
	if dest, ok := profileFV["destination_country"]; ok && dest != nil {
		fv["destination_country"] = dest
	}
	for k, v := range cfv {
		fv[k] = v
	}
	if len(fv) > 0 {
		meta["field_values"] = fv
	}

	return ResolvedItem{
		Type:        "mentor",
		ID:          m.UserID.String(),
		Title:       title,
		Description: Excerpt(desc, 160),
		Metadata:    meta,
	}
}

// mentorBaseFieldValues projects the columnar mentor profile into the
// field_values map the matrix hydrator reads — the §114 "Mentorship
// details" card fields (000829 columns, mentees-helped count, shadow
// listing price) plus the §102 comparison_fields set. List/date fields
// serialize to their display forms, counts to numbers. destination_country
// is not columnar — it arrives via the user_profile cfv bucket and is
// merged by the caller.
func mentorBaseFieldValues(m mentorship.MentorSummary) map[string]any {
	fv := make(map[string]any, 20)

	setText := func(key string, v *string) {
		if s := deref(v); s != "" {
			fv[key] = s
		}
	}
	setList := func(key string, v []string) {
		if len(v) > 0 {
			fv[key] = v
		}
	}

	setText("mentor_session_type", m.SessionType)
	setList("mentor_areas", m.MentorAreas)
	setList("mentor_countries", m.Countries)
	setList("mentor_credentials", m.Credentials)
	if m.YearsExperience != nil {
		fv["mentor_years_experience"] = float64(*m.YearsExperience)
	}
	if m.MenteesHelped != nil {
		fv["mentees_helped"] = float64(*m.MenteesHelped)
	}
	setText("mentor_success_label", m.SuccessLabel)
	setText("mentor_response_time", m.ResponseTime)
	setText("mentor_availability", m.Availability)
	setList("mentor_languages", m.Languages)
	setList("mentor_session_formats", m.SessionFormats)
	setText("location", m.Location)
	if m.PackagePrice != nil {
		fv["package_price"] = mentorPackagePriceLabel(*m.PackagePrice, deref(m.PackageCurrency), deref(m.PackagePer))
	}
	setText("profession", m.Profession)
	setText("mentor_title", m.MentorTitle)
	if m.MentorSince.Valid {
		fv["mentor_since"] = m.MentorSince.Time.Format("2006-01-02")
	}
	if m.MentorCapacity != nil {
		fv["available_slots"] = float64(m.AvailableSlots)
	}
	if m.ActiveMenteeCount != nil {
		fv["active_mentee_count"] = float64(*m.ActiveMenteeCount)
	}
	if bio := deref(m.MentorBio); bio != "" {
		fv["mentor_bio"] = bio
	}

	return fv
}

// mentorPackagePriceLabel renders the shadow listing's minor-unit price as
// a display string ("USD 180.00 per package"). Text, not a currency dim:
// dimension currency codes are bound to the field's options, but mentors
// price in mixed currencies, so the item's own code rides the label.
func mentorPackagePriceLabel(minor int64, currency, per string) string {
	s := fmt.Sprintf("%.2f", float64(minor)/100)
	if currency != "" {
		s = strings.ToUpper(currency) + " " + s
	}
	if per != "" {
		s += " per " + per
	}

	return s
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
