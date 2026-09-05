// Package achievement — service.go orchestrates achievement submission,
// editing, deletion, filtered listing, and admin moderation. It delegates
// content row creation to the content.Service (§14) and manages the
// achievement_meta extension table via the local Store. Feature-flagged via
// achievement.enabled system_setting (§89).
//
// Ref: japaconnect-engineering/sections/89-community-achievement-feed.md
//
//	japaconnect-engineering/adr/ADR-097-community-achievement-feed.md
package achievement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
)

// ContentService is the subset of content.Service the achievement package
// needs. Defined as an interface so the achievement package depends on a
// narrow contract, not the full content service.
type ContentService interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, input content.CreateContentInput) (gen.CreateContentRow, error)
	GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error)
	UpdateContent(ctx context.Context, id, tenantID, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error)
	UpdateContentStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (gen.UpdateContentStatusRow, error)
	DeleteContent(ctx context.Context, id, tenantID pgtype.UUID) error
}

// AchievementDTO is the API response shape for a single achievement.
type AchievementDTO struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Slug             string         `json:"slug"`
	Body             string         `json:"body"`
	BodyHTML         string         `json:"body_html"`
	Status           string         `json:"status"`
	Visibility       string         `json:"visibility"`
	ViewCount        int32          `json:"view_count"`
	CommentCount     int32          `json:"comment_count"`
	ReactionCount    int32          `json:"reaction_count"`
	PublishedAt      string         `json:"published_at"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
	MilestoneType    string         `json:"milestone_type,omitempty"`
	OriginSpaceID    string         `json:"origin_space_id,omitempty"`
	OriginSpaceName  string         `json:"origin_space_name,omitempty"`
	OriginSpaceCode  string         `json:"origin_space_code,omitempty"`
	DestSpaceID      string         `json:"destination_space_id"`
	DestSpaceName    string         `json:"destination_space_name"`
	DestSpaceCode    string         `json:"destination_space_code,omitempty"`
	IsAnonymous      bool           `json:"is_anonymous"`
	IsOwner          bool           `json:"is_owner,omitempty"`
	AuthorID         string         `json:"author_id,omitempty"`
	AuthorName       string         `json:"author_name,omitempty"`
	AuthorAvatar     string         `json:"author_avatar,omitempty"`
	AllowedReactions []ReactionItem `json:"allowed_reactions,omitempty"`
}

// ReactionItem is a single curated reaction for achievement posts (§89).
type ReactionItem struct {
	Emoji string `json:"emoji"`
	Label string `json:"label"`
}

// SubmitInput is the request body for POST /achievements.
type SubmitInput struct {
	Title              string `json:"title"`
	Body               string `json:"body"`
	MilestoneType      string `json:"milestone_type"`
	DestinationSpaceID string `json:"destination_space_id"`
	OriginSpaceID      string `json:"origin_space_id"`
	IsAnonymous        bool   `json:"is_anonymous"`
}

// EditInput is the request body for PATCH /achievements/:id.
type EditInput struct {
	Title              *string `json:"title"`
	Body               *string `json:"body"`
	MilestoneType      *string `json:"milestone_type"`
	DestinationSpaceID *string `json:"destination_space_id"`
	OriginSpaceID      *string `json:"origin_space_id"`
	IsAnonymous        *bool   `json:"is_anonymous"`
}

// ActivityEmitter records an activity event and enqueues async processing.
// Satisfied by *wallet.Service; declared here to avoid an import cycle
// (wallet → worker → ... → achievement). Ref: §64 Gamification.
type ActivityEmitter interface {
	EmitActivityEvent(ctx context.Context, tenantID, userID pgtype.UUID, input ActivityEventInput) (ActivityEvent, error)
	EnqueueProcessActivityEvent(ctx context.Context, eventID, tenantID pgtype.UUID) error
}

// ActivityEventInput mirrors wallet.EmitActivityEventInput. Declared here to
// avoid importing wallet (import cycle).
type ActivityEventInput struct {
	ActivityKey string
	Reference   string
	Payload     map[string]interface{}
}

// ActivityEvent mirrors the minimal fields the emitter returns.
type ActivityEvent struct {
	ID pgtype.UUID
}

// Service holds the business logic for the Community Achievement Feed.
type Service struct {
	store    *Store
	content  ContentService
	settings *config.Settings
	emitter  ActivityEmitter
}

// NewService creates an achievement service.
func NewService(store *Store, contentSvc ContentService, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		content:  contentSvc,
		settings: settings,
	}
}

// WithActivityEmitter attaches the wallet activity emitter used to fire
// achievement.created events for gamification badge counting (§64/§89).
func (s *Service) WithActivityEmitter(e ActivityEmitter) *Service { s.emitter = e; return s }

// Enabled reads the achievement.enabled system_setting toggle (§89 feature
// flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, tenantID, "achievement.enabled", false)
}

// defaultReactions is the spec-defined curated reaction set (§89 Reaction
// Set), used when system_settings.achievement_reactions is empty/unparseable.
var defaultReactions = []ReactionItem{
	{Emoji: "🎉", Label: "Celebrate"},
	{Emoji: "🔥", Label: "Inspired"},
	{Emoji: "💪", Label: "Motivated"},
}

// allowedReactions reads the curated achievement reaction set from
// system_settings.achievement_reactions (JSONB array of {emoji, label}
// pairs). Falls back to the spec defaults (🎉 🔥 💪) when the setting is
// empty or unparseable. Ref: §89 Reaction Set.
func (s *Service) allowedReactions(ctx context.Context, tenantID pgtype.UUID) []ReactionItem {
	if s.settings == nil {
		return defaultReactions
	}

	raw := s.settings.GetStringOr(ctx, tenantID, "achievement_reactions", "")
	if raw == "" || raw == "[]" {
		return defaultReactions
	}

	var items []ReactionItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil || len(items) == 0 {
		return defaultReactions
	}

	return items
}

// ErrMissingMilestone is returned when a submission omits milestone_type.
var ErrMissingMilestone = errors.New("milestone_type is required")

// ErrInvalidMilestone is returned when a submission uses a milestone_type
// that is not in the spec-defined enum set (§89 Schema).
var ErrInvalidMilestone = errors.New("invalid milestone_type")

// ErrInvalidDestination is returned when the destination_space_id does not
// reference a country-level space (§89 Geo Awareness).
var ErrInvalidDestination = errors.New("destination_space_id must be a country-level space")

// validMilestoneTypes is the spec-defined enum set for achievement milestone
// types (§89 Schema). The DB column is TEXT (no CHECK constraint), so
// server-side validation is the only guard against invalid values.
// Ref: japaconnect-engineering/sections/89-achievement-feed.md.
var validMilestoneTypes = map[string]bool{
	"visa_approved":       true,
	"job_offer":           true,
	"university_accepted": true,
	"arrived":             true,
	"one_year":            true,
	"citizenship":         true,
	"other":               true,
}

// IsValidMilestoneType reports whether v is a spec-defined milestone type.
func IsValidMilestoneType(v string) bool {
	return validMilestoneTypes[v]
}

// Submit creates a new achievement — a contents row with
// content_type='achievement' plus an achievement_meta row. The achievement is
// published immediately (member+ is gated by the achievement.create
// permission at the route level). Auth required.
func (s *Service) Submit(ctx context.Context, tenantID, authorID pgtype.UUID, input SubmitInput) (AchievementDTO, error) {
	if input.Title == "" || input.DestinationSpaceID == "" {
		return AchievementDTO{}, errors.New("title and destination_space_id are required")
	}

	if input.MilestoneType == "" {
		return AchievementDTO{}, ErrMissingMilestone
	}

	if !IsValidMilestoneType(input.MilestoneType) {
		return AchievementDTO{}, ErrInvalidMilestone
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return AchievementDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	// §89 Geo Awareness: destination must be a country-level space. The spec
	// tags every achievement by destination_country_id; filing it in a city
	// or topic space would break the destination filter and Space widget.
	level, err := s.store.GetSpaceLevel(ctx, destSpaceID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AchievementDTO{}, ErrInvalidDestination
		}

		return AchievementDTO{}, fmt.Errorf("achievement: validate destination space: %w", err)
	}

	if level != "country" {
		return AchievementDTO{}, ErrInvalidDestination
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return AchievementDTO{}, fmt.Errorf("invalid origin_space_id: %w", err)
		}
	}

	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	contentRow, err := s.content.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:    authorID,
		ContentType: content.TypeAchievement,
		Title:       input.Title,
		Body:        input.Body,
		Status:      content.StatusPublished,
		Visibility:  content.VisibilityPublic,
		GeoSpaceID:  destSpaceID, // file achievement in destination country space
		PublishAt:   now,
	})
	if err != nil {
		return AchievementDTO{}, fmt.Errorf("achievement: create content: %w", err)
	}

	meta := AchievementMeta{
		TenantID:           tenantID,
		ContentID:          contentRow.ID,
		DestinationSpaceID: destSpaceID,
		OriginSpaceID:      originSpaceID,
		IsAnonymous:        input.IsAnonymous,
	}
	if input.MilestoneType != "" {
		meta.MilestoneType = &input.MilestoneType
	}

	if err := s.store.CreateMeta(ctx, meta); err != nil {
		slog.Error("achievement: create achievement_meta failed after content creation", "content_id", contentRow.ID, "error", err)
		return AchievementDTO{}, fmt.Errorf("achievement: create achievement_meta: %w", err)
	}

	// §64/§89 Gamification: fire achievement.created activity event so the
	// wallet:process_activity_event task can award points and evaluate the
	// "Community Voice" badge (activity_count: achievement.created, threshold
	// 5). Best-effort — a failure logs a warning but does not abort the
	// submission (Rule 8: fan-out is fire-and-forget).
	if s.emitter != nil {
		event, err := s.emitter.EmitActivityEvent(ctx, tenantID, authorID, ActivityEventInput{
			ActivityKey: "achievement.created",
			Reference:   contentRow.ID.String(),
			Payload:     map[string]interface{}{"milestone_type": input.MilestoneType},
		})
		if err != nil {
			slog.Warn("achievement: emit activity event failed", "content_id", contentRow.ID, "error", err)
		} else if err := s.emitter.EnqueueProcessActivityEvent(ctx, event.ID, tenantID); err != nil {
			slog.Warn("achievement: enqueue activity event failed", "content_id", contentRow.ID, "error", err)
		}
	}

	return s.getAchievementDTOByID(ctx, contentRow.ID, tenantID, authorID)
}

// GetBySlug returns a single achievement by its slug. Only published
// achievements are visible to anonymous viewers; an unpublished achievement
// (e.g. admin-moderated) is visible only to its own author. Pass a zero
// (invalid) viewerID for anonymous access.
func (s *Service) GetBySlug(ctx context.Context, slug string, tenantID, viewerID pgtype.UUID) (AchievementDTO, error) {
	row, err := s.store.GetBySlug(ctx, slug, tenantID, viewerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AchievementDTO{}, pgx.ErrNoRows
		}

		return AchievementDTO{}, fmt.Errorf("achievement: get by slug: %w", err)
	}

	return rowToDTO(row, viewerID), nil
}

// GetBySlugWithReactions returns a single achievement by slug, populated
// with the curated reaction set for the detail page UI (§89 Reaction Set).
func (s *Service) GetBySlugWithReactions(ctx context.Context, slug string, tenantID, viewerID pgtype.UUID) (AchievementDTO, error) {
	dto, err := s.GetBySlug(ctx, slug, tenantID, viewerID)
	if err != nil {
		return AchievementDTO{}, err
	}

	dto.AllowedReactions = s.allowedReactions(ctx, tenantID)

	return dto, nil
}

// List returns a paginated, filtered list of published achievements.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID pgtype.UUID, milestoneType, sort string, page, limit int32) ([]AchievementDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListAchievements(ctx, ListAchievementsFilter{
		TenantID:      tenantID,
		DestSpaceID:   destSpaceID,
		MilestoneType: milestoneType,
		Sort:          sort,
		Status:        content.StatusPublished,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("achievement: list: %w", err)
	}

	dtos := make([]AchievementDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r, pgtype.UUID{}))
	}

	return dtos, total, nil
}

// ListBySpace returns published achievements filed in the Space identified by
// its slug, for the Space widget.
func (s *Service) ListBySpace(ctx context.Context, spaceSlug string, tenantID pgtype.UUID, limit int32) ([]AchievementDTO, error) {
	rows, err := s.store.ListBySpace(ctx, spaceSlug, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("achievement: list by space: %w", err)
	}

	dtos := make([]AchievementDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r, pgtype.UUID{}))
	}

	return dtos, nil
}

// EditOwn updates the author's own achievement. The caller must verify
// ownership (achievement.edit_own permission + author_id match). Only the
// provided (non-nil) fields are updated.
func (s *Service) EditOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID, input EditInput) (AchievementDTO, error) {
	// Fetch existing content to verify ownership.
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AchievementDTO{}, pgx.ErrNoRows
		}

		return AchievementDTO{}, fmt.Errorf("achievement: edit: get content: %w", err)
	}

	if existing.AuthorID != userID {
		return AchievementDTO{}, ErrNotOwner
	}

	// Build content update input from non-nil fields. UpdateContent
	// overwrites with empty strings, so preserve existing values when
	// the input field is nil.
	updateInput := content.UpdateContentInput{
		Title: existing.Title,
		Body:  existing.Body,
	}
	if input.Title != nil {
		updateInput.Title = *input.Title
	}

	if input.Body != nil {
		updateInput.Body = *input.Body
	}

	if _, err := s.content.UpdateContent(ctx, contentID, tenantID, userID, updateInput, false); err != nil {
		return AchievementDTO{}, fmt.Errorf("achievement: edit: update content: %w", err)
	}

	// Update achievement_meta fields. Fetch the existing meta row first and
	// apply only the non-nil input fields onto it, so a partial edit (e.g.
	// title-only) does not zero out milestone_type / destination_space_id /
	// origin_space_id / is_anonymous (ACH-002).
	existingMeta, err := s.store.GetMeta(ctx, contentID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AchievementDTO{}, fmt.Errorf("achievement: edit: achievement_meta not found: %w", err)
		}

		return AchievementDTO{}, fmt.Errorf("achievement: edit: get achievement_meta: %w", err)
	}

	meta := existingMeta // start from existing values

	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		var dest pgtype.UUID
		if err := dest.Scan(*input.DestinationSpaceID); err != nil {
			return AchievementDTO{}, fmt.Errorf("achievement: edit: invalid destination_space_id: %w", err)
		}
		// §89 Geo Awareness: destination must be a country-level space.
		level, err := s.store.GetSpaceLevel(ctx, dest, tenantID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return AchievementDTO{}, ErrInvalidDestination
			}

			return AchievementDTO{}, fmt.Errorf("achievement: edit: validate destination space: %w", err)
		}

		if level != "country" {
			return AchievementDTO{}, ErrInvalidDestination
		}

		meta.DestinationSpaceID = dest
	}

	if input.OriginSpaceID != nil {
		if *input.OriginSpaceID == "" {
			// Explicitly clear origin.
			meta.OriginSpaceID = pgtype.UUID{}
		} else {
			var origin pgtype.UUID
			if err := origin.Scan(*input.OriginSpaceID); err != nil {
				return AchievementDTO{}, fmt.Errorf("achievement: edit: invalid origin_space_id: %w", err)
			}

			meta.OriginSpaceID = origin
		}
	}

	if input.MilestoneType != nil {
		if *input.MilestoneType != "" && !IsValidMilestoneType(*input.MilestoneType) {
			return AchievementDTO{}, ErrInvalidMilestone
		}

		meta.MilestoneType = input.MilestoneType
	}

	if input.IsAnonymous != nil {
		meta.IsAnonymous = *input.IsAnonymous
	}

	if err := s.store.UpdateMeta(ctx, contentID, tenantID, meta); err != nil {
		return AchievementDTO{}, fmt.Errorf("achievement: edit: update achievement_meta: %w", err)
	}

	return s.getAchievementDTOByID(ctx, contentID, tenantID, userID)
}

// DeleteOwn soft-deletes the author's own achievement. The caller must verify
// ownership (achievement.delete_own permission + author_id match).
func (s *Service) DeleteOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID) error {
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("achievement: delete: get content: %w", err)
	}

	if existing.AuthorID != userID {
		return ErrNotOwner
	}

	if err := s.content.DeleteContent(ctx, contentID, tenantID); err != nil {
		return fmt.Errorf("achievement: delete: %w", err)
	}

	return nil
}

// AdminList returns all achievements (any status) for the moderation queue.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, milestoneType, q string, page, limit int32) ([]AchievementDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{
		TenantID:      tenantID,
		Status:        status,
		MilestoneType: milestoneType,
		Q:             q,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("achievement: admin list: %w", err)
	}

	dtos := make([]AchievementDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r, pgtype.UUID{}))
	}

	return dtos, total, nil
}

// AdminSetStatus changes the publication status of an achievement (e.g.
// publish / unpublish). Used by the moderation queue. Caller must hold the
// achievement.moderate permission (enforced at route level).
func (s *Service) AdminSetStatus(ctx context.Context, contentID, tenantID pgtype.UUID, status string) (AchievementDTO, error) {
	if status != content.StatusPublished && status != content.StatusDraft && status != content.StatusInReview {
		return AchievementDTO{}, fmt.Errorf("invalid status: %s", status)
	}

	if _, err := s.content.UpdateContentStatus(ctx, contentID, tenantID, status); err != nil {
		return AchievementDTO{}, fmt.Errorf("achievement: admin set status: %w", err)
	}

	return s.getAchievementDTOByIDAdmin(ctx, contentID, tenantID)
}

// AdminDelete soft-deletes an achievement from the moderation queue. Caller
// must hold the achievement.moderate permission (enforced at route level).
func (s *Service) AdminDelete(ctx context.Context, contentID, tenantID pgtype.UUID) error {
	if err := s.content.DeleteContent(ctx, contentID, tenantID); err != nil {
		return fmt.Errorf("achievement: admin delete: %w", err)
	}

	return nil
}

// getAchievementDTOByID fetches an achievement by content_id via the full
// joined detail query (so the DTO includes reaction_count, space names, and
// author info) and converts it to a DTO. The viewerID authorises viewing
// unpublished achievements owned by the viewer; pass a zero (invalid) UUID
// when the caller is a system/admin context that should see any achievement.
func (s *Service) getAchievementDTOByID(ctx context.Context, contentID, tenantID, viewerID pgtype.UUID) (AchievementDTO, error) {
	row, err := s.store.GetByID(ctx, contentID, tenantID, viewerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AchievementDTO{}, pgx.ErrNoRows
		}

		return AchievementDTO{}, fmt.Errorf("achievement: get by id: %w", err)
	}

	return rowToDTO(row, viewerID), nil
}

// getAchievementDTOByIDAdmin fetches an achievement by content_id regardless
// of publication status, for admin/moderation response paths. The caller
// must hold the achievement.moderate permission (enforced at route level).
func (s *Service) getAchievementDTOByIDAdmin(ctx context.Context, contentID, tenantID pgtype.UUID) (AchievementDTO, error) {
	row, err := s.store.GetByIDAnyStatus(ctx, contentID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AchievementDTO{}, pgx.ErrNoRows
		}

		return AchievementDTO{}, fmt.Errorf("achievement: get by id (admin): %w", err)
	}

	return rowToDTO(row, pgtype.UUID{}), nil
}

// ErrNotOwner is returned when a user tries to edit or delete an achievement
// they did not author.
var ErrNotOwner = errors.New("not the achievement author")

// rowToDTO converts an AchievementRow (joined query result) to an
// AchievementDTO. The viewerID is used to compute is_owner and to decide
// whether author_id is exposed: per §89 Privacy, author_id is NOT exposed
// when is_anonymous is true (user_id is retained internally only). The
// is_owner boolean lets the frontend show edit/delete affordances for the
// author without needing the raw author_id.
func rowToDTO(r AchievementRow, viewerID pgtype.UUID) AchievementDTO {
	dto := AchievementDTO{
		ID:            r.ID.String(),
		Title:         r.Title,
		Slug:          r.Slug,
		Body:          r.Body,
		BodyHTML:      r.BodyHTML,
		Status:        r.Status,
		Visibility:    r.Visibility,
		ViewCount:     r.ViewCount,
		CommentCount:  r.CommentCount,
		ReactionCount: r.ReactionCount,
		PublishedAt:   formatTs(r.PublishedAt),
		CreatedAt:     formatTs(r.CreatedAt),
		UpdatedAt:     formatTs(r.UpdatedAt),
		IsAnonymous:   r.IsAnonymous,
	}
	if r.MilestoneType != nil {
		dto.MilestoneType = *r.MilestoneType
	}

	if r.OriginSpaceID.Valid {
		dto.OriginSpaceID = r.OriginSpaceID.String()
	}

	if r.OriginSpaceName != nil {
		dto.OriginSpaceName = *r.OriginSpaceName
	}

	if r.OriginSpaceCode != nil {
		dto.OriginSpaceCode = *r.OriginSpaceCode
	}

	if r.DestSpaceID.Valid {
		dto.DestSpaceID = r.DestSpaceID.String()
	}

	if r.DestSpaceName != nil {
		dto.DestSpaceName = *r.DestSpaceName
	}

	if r.DestSpaceCode != nil {
		dto.DestSpaceCode = *r.DestSpaceCode
	}
	// §89 Privacy: suppress author_name and author_avatar when anonymous.
	if !r.IsAnonymous {
		if r.AuthorName != nil {
			dto.AuthorName = *r.AuthorName
		}

		if r.AuthorAvatar != nil {
			dto.AuthorAvatar = *r.AuthorAvatar
		}
	}
	// §89 Privacy: author_id is NOT exposed when is_anonymous is true.
	// The is_owner boolean lets the frontend gate edit/delete UI without
	// the raw author_id.
	if r.AuthorID.Valid && viewerID.Valid && r.AuthorID == viewerID {
		dto.IsOwner = true
	}

	if r.AuthorID.Valid && !r.IsAnonymous {
		dto.AuthorID = r.AuthorID.String()
	}

	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}
