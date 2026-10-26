// Package mutualaid — service.go orchestrates post creation, editing,
// deletion, fulfillment, renewal, filtered listing, and admin moderation
// for the Mutual Aid Board (§85, ADR-093). Feature-flagged via the
// mutual_aid.enabled system_setting.
// // Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
// //	japaconnect-engineering/adr/ADR-093-mutual-aid-board.md
package mutualaid

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/cdn"
	"github.com/jedafe/japa/internal/config"
)

// PostExpiry is the default lifetime of a mutual aid post before it must
// be renewed (§85: 30-day expiry).
const PostExpiry = 30 * 24 * time.Hour

// PostDTO is the API response shape for a single mutual aid post.
type PostDTO struct {
	ID                 string `json:"id"`
	PostType           string `json:"post_type"`
	Category           string `json:"category,omitempty"`
	Title              string `json:"title"`
	Description        string `json:"description,omitempty"`
	DestinationSpaceID string `json:"destination_space_id"`
	DestSpaceName      string `json:"destination_space_name,omitempty"`
	DestSpaceCode      string `json:"destination_space_code,omitempty"`
	CitySpaceID        string `json:"city_space_id,omitempty"`
	CitySpaceName      string `json:"city_space_name,omitempty"`
	CitySpaceCode      string `json:"city_space_code,omitempty"`
	IsFulfilled        bool   `json:"is_fulfilled"`
	Status             string `json:"status"`
	ExpiresAt          string `json:"expires_at"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
	AuthorID           string `json:"author_id,omitempty"`
	AuthorName         string `json:"author_name,omitempty"`
	AuthorAvatar       string `json:"author_avatar,omitempty"`
}

// CreatePostRequest is the request body for POST /mutual-aid.
type CreatePostRequest struct {
	PostType           string `json:"post_type"`
	Category           string `json:"category"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	DestinationSpaceID string `json:"destination_space_id"`
	CitySpaceID        string `json:"city_space_id"`
}

// UpdatePostRequest is the request body for PATCH /mutual-aid/:id.
// Only non-nil fields are applied.
type UpdatePostRequest struct {
	Category    *string `json:"category"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

// RespondRequest is the request body for POST /mutual-aid/:id/respond.
type RespondRequest struct {
	Message string `json:"message"`
}

// Notifier is the interface used by the mutual aid service to dispatch
// expiry-approaching notifications. It is satisfied by
// *notifications.Service. Defined here (rather than importing notifications
// directly) to avoid an import cycle: notifications → worker → mutualaid.
// Ref: japaconnect-engineering/sections/35-notification-system.md.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// Service holds the business logic for the Mutual Aid Board.
type Service struct {
	store    *Store
	settings *config.Settings
	notifier Notifier
}

// NewService creates a mutual aid service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetNotifier injects the notification service used to dispatch 3-day-prior
// expiry notifications (§85). Optional — when nil, ExpirePosts skips the
// notification step but still expires posts.
func (s *Service) SetNotifier(n Notifier) {
	s.notifier = n
}

// Option is a single selectable value exposed by the option endpoints.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// human labels for option endpoints (Rule 16: backend owns canonical values).
var postTypeLabels = map[string]string{
	PostTypeNeed:      "Need",
	PostTypeOffer:     "Offer",
	PostTypeSkillSwap: "Skill swap",
}

var categoryLabels = map[string]string{
	CategoryHousing:   "Housing",
	CategoryTransport: "Transport",
	CategoryDocuments: "Documents",
	CategoryJobSearch: "Job search",
	CategoryLanguage:  "Language",
	CategorySocial:    "Social",
	CategoryOther:     "Other",
}

func sortOptions(opts []Option) {
	sort.Slice(opts, func(i, j int) bool { return opts[i].Value < opts[j].Value })
}

// PostTypes returns the canonical mutual aid post type options.
func (s *Service) PostTypes() []Option {
	out := make([]Option, 0, len(postTypeLabels))
	for v, label := range postTypeLabels {
		out = append(out, Option{Value: v, Label: label})
	}

	sortOptions(out)

	return out
}

// Categories returns the canonical mutual aid category options.
func (s *Service) Categories() []Option {
	out := make([]Option, 0, len(categoryLabels))
	for v, label := range categoryLabels {
		out = append(out, Option{Value: v, Label: label})
	}

	sortOptions(out)

	return out
}

// Create inserts a new mutual aid post. Auth required (permission checked at
// route level based on post_type: create_offer, create_need, create_skill_swap).
func (s *Service) Create(ctx context.Context, userID pgtype.UUID, input CreatePostRequest) (PostDTO, error) {
	if input.Title == "" {
		return PostDTO{}, errors.New("title is required")
	}

	if input.DestinationSpaceID == "" {
		return PostDTO{}, errors.New("destination_space_id is required")
	}

	if input.PostType != PostTypeNeed && input.PostType != PostTypeOffer && input.PostType != PostTypeSkillSwap {
		return PostDTO{}, fmt.Errorf("invalid post_type: %s", input.PostType)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return PostDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var citySpaceID pgtype.UUID
	if input.CitySpaceID != "" {
		if err := citySpaceID.Scan(input.CitySpaceID); err != nil {
			return PostDTO{}, fmt.Errorf("invalid city_space_id: %w", err)
		}
	}

	storeInput := CreatePostInput{
		UserID:             userID,
		PostType:           input.PostType,
		Title:              input.Title,
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		ExpiresAt:          pgtype.Timestamptz{Time: time.Now().UTC().Add(PostExpiry), Valid: true},
	}
	if input.Category != "" {
		storeInput.Category = &input.Category
	}

	if input.Description != "" {
		storeInput.Description = &input.Description
	}

	post, err := s.store.CreatePost(ctx, storeInput)
	if err != nil {
		return PostDTO{}, fmt.Errorf("mutualaid: create: %w", err)
	}

	// Re-fetch via GetPost to populate joined space/author fields.
	row, err := s.store.GetPost(ctx, post.ID)
	if err != nil {
		slog.Error("mutualaid: create re-fetch failed", "post_id", post.ID, "error", err)
		return PostDTO{}, fmt.Errorf("mutualaid: create re-fetch: %w", err)
	}

	return rowToDTO(row), nil
}

// Get returns a single post by id. Anonymous-accessible for public posts;
// the author may also fetch their own moderation-removed post so the §22
// appeal surface on the detail page stays reachable.
func (s *Service) Get(ctx context.Context, id, viewerID pgtype.UUID) (PostDTO, error) {
	row, err := s.store.GetPostForViewer(ctx, id, viewerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PostDTO{}, pgx.ErrNoRows
		}

		return PostDTO{}, fmt.Errorf("mutualaid: get: %w", err)
	}

	return rowToDTO(row), nil
}

// List returns a paginated, filtered list of active posts.
func (s *Service) List(ctx context.Context, postType, category string, destSpaceID, citySpaceID pgtype.UUID, page, limit int32) ([]PostDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListPosts(ctx, ListPostsFilter{
		PostType:           postType,
		Category:           category,
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("mutualaid: list: %w", err)
	}

	dtos := make([]PostDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// UpdateOwn updates the author's own post. The caller must verify ownership
// (mutual_aid.* permission + user_id match). Only non-nil fields are applied.
func (s *Service) UpdateOwn(ctx context.Context, id, userID pgtype.UUID, input UpdatePostRequest) (PostDTO, error) {
	existing, err := s.store.GetPost(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PostDTO{}, pgx.ErrNoRows
		}

		return PostDTO{}, fmt.Errorf("mutualaid: update: get: %w", err)
	}

	if existing.UserID != userID {
		return PostDTO{}, ErrNotOwner
	}

	storeInput := UpdatePostInput{
		Category:    existing.Category,
		Title:       &existing.Title,
		Description: existing.Description,
	}
	if input.Category != nil {
		storeInput.Category = input.Category
	}

	if input.Title != nil {
		storeInput.Title = input.Title
	}

	if input.Description != nil {
		storeInput.Description = input.Description
	}

	if err := s.store.UpdatePost(ctx, id, storeInput); err != nil {
		return PostDTO{}, fmt.Errorf("mutualaid: update: %w", err)
	}

	row, err := s.store.GetPost(ctx, id)
	if err != nil {
		return PostDTO{}, fmt.Errorf("mutualaid: update re-fetch: %w", err)
	}

	cdn.PurgePublicPaths(ctx, []string{"mutual_aid:" + id.String()}, "/mutual-aid/"+id.String(), "/mutual-aid")

	return rowToDTO(row), nil
}

// DeleteOwn soft-deletes the author's own post. The caller must verify
// ownership (mutual_aid.* permission + user_id match).
func (s *Service) DeleteOwn(ctx context.Context, id, userID pgtype.UUID) error {
	existing, err := s.store.GetPost(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("mutualaid: delete: get: %w", err)
	}

	if existing.UserID != userID {
		return ErrNotOwner
	}

	if err := s.store.DeletePost(ctx, id); err != nil {
		return fmt.Errorf("mutualaid: delete: %w", err)
	}

	cdn.PurgePublicPaths(ctx, []string{"mutual_aid:" + id.String()}, "/mutual-aid/"+id.String(), "/mutual-aid")

	return nil
}

// MarkFulfilled marks a post as fulfilled by its author.
func (s *Service) MarkFulfilled(ctx context.Context, id, userID pgtype.UUID) error {
	existing, err := s.store.GetPost(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("mutualaid: fulfill: get: %w", err)
	}

	if existing.UserID != userID {
		return ErrNotOwner
	}

	if err := s.store.MarkFulfilled(ctx, id); err != nil {
		return fmt.Errorf("mutualaid: fulfill: %w", err)
	}

	cdn.PurgePublicPaths(ctx, []string{"mutual_aid:" + id.String()}, "/mutual-aid/"+id.String(), "/mutual-aid")

	return nil
}

// Renew extends a post's expiry by 30 days from now. Author-only.
// Q1: only active|expired posts are renewable — a moderator-removed
// post cannot be resurrected via renew, and a fulfilled post stays
// fulfilled (renewing it would contradict the §85 lifecycle).
func (s *Service) Renew(ctx context.Context, id, userID pgtype.UUID) (PostDTO, error) {
	existing, err := s.store.GetPost(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PostDTO{}, pgx.ErrNoRows
		}

		return PostDTO{}, fmt.Errorf("mutualaid: renew: get: %w", err)
	}

	if existing.UserID != userID {
		return PostDTO{}, ErrNotOwner
	}

	switch existing.Status {
	case StatusRemoved:
		return PostDTO{}, ErrPostRemoved
	case StatusFulfilled:
		return PostDTO{}, ErrPostFulfilled
	case StatusActive, StatusExpired:
		// renewable
	default:
		return PostDTO{}, fmt.Errorf("%w: post status %q is not renewable", ErrInvalid, existing.Status)
	}

	newExpiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(PostExpiry), Valid: true}
	if err := s.store.RenewPost(ctx, id, newExpiry); err != nil {
		return PostDTO{}, fmt.Errorf("mutualaid: renew: %w", err)
	}

	row, err := s.store.GetPost(ctx, id)
	if err != nil {
		return PostDTO{}, fmt.Errorf("mutualaid: renew re-fetch: %w", err)
	}

	cdn.PurgePublicPaths(ctx, []string{"mutual_aid:" + id.String()}, "/mutual-aid/"+id.String(), "/mutual-aid")

	return rowToDTO(row), nil
}

// AdminList returns all posts (any status) for the moderation queue.
func (s *Service) AdminList(ctx context.Context, postType, status, q string, destSpaceID, citySpaceID pgtype.UUID, page, limit int32) ([]PostDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.AdminListPosts(ctx, AdminListPostsFilter{
		PostType:           postType,
		Status:             status,
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		Q:                  q,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("mutualaid: admin list: %w", err)
	}

	dtos := make([]PostDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// AdminModerate sets the status of a post (moderation action).
func (s *Service) AdminModerate(ctx context.Context, id pgtype.UUID, status string) error {
	if status != StatusActive && status != StatusExpired && status != StatusRemoved && status != StatusFulfilled {
		return fmt.Errorf("invalid status: %s", status)
	}

	if err := s.store.AdminUpdateStatus(ctx, id, status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("mutualaid: admin moderate: %w", err)
	}

	cdn.PurgePublicPaths(ctx, []string{"mutual_aid:" + id.String()}, "/mutual-aid/"+id.String(), "/mutual-aid")

	return nil
}

// NeedReputationThreshold returns the minimum reputation (lifetime points)
// required to create a need post. Reads the mutual_aid.need_reputation_threshold
// system_setting (§85: create_need is reputation-gated).
func (s *Service) NeedReputationThreshold(ctx context.Context) int64 {
	if s.settings == nil {
		return 0
	}

	return int64(s.settings.GetIntOr(ctx, "mutual_aid.need_reputation_threshold", 0))
}

// ExpirePosts marks all active posts past their expires_at as 'expired'
// and sends a 3-day-prior notification to owners whose posts are about
// to expire. Called nightly by the mutual_aid:expire Asynq task (§85).
// Implements worker.MutualAidProcessor.
func (s *Service) ExpirePosts(ctx context.Context) (int64, error) {
	ids, err := s.store.ExpirePosts(ctx)
	if err != nil {
		return 0, fmt.Errorf("mutualaid: expire: %w", err)
	}
	n := int64(len(ids))
	if n > 0 {
		// ADR-134: bulk UPDATE bypasses any tag emission — purge each expired
		// post's detail page plus the listing.
		paths := make([]string, 0, len(ids)+1)
		paths = append(paths, "/mutual-aid")
		for _, pid := range ids {
			paths = append(paths, "/mutual-aid/"+pid.String())
		}
		cdn.PurgePublicPaths(ctx, nil, paths...)
	}
	// BUG-004: send 3-day-prior expiry notifications to owners of active
	// posts that will expire within the next 72 hours. §85 requires the
	// nightly task to notify owners three days before expiry. Failures
	// here are logged but do not fail the task — expiry already succeeded.
	if s.notifier != nil {
		const withinHours = 72

		expiring, err := s.store.PostsExpiringSoon(ctx, withinHours)
		if err != nil {
			slog.Warn("mutualaid: expiring-soon lookup failed", "error", err)
		} else {
			zeroUUID := pgtype.UUID{}

			for _, p := range expiring {
				if !p.UserID.Valid || !(pgtype.UUID{}).
					Valid {
					continue
				}

				title := "Your mutual aid post expires soon"
				body := "Your mutual aid post \"" + p.Title + "\" will expire in less than 3 days. Renew it to keep it visible."
				actionURL := "/mutual-aid/" + p.ID.String()

				if err := s.notifier.CreateNotification(
					ctx,

					p.UserID,
					"mutual_aid_expiring",
					"system",
					zeroUUID,
					"mutual_aid_post",
					p.ID,
					title,
					body,
					actionURL,
				); err != nil {
					slog.Warn("mutualaid: expiry notification failed",
						"post_id", p.ID.String(),
						"user_id", p.UserID.String(),
						"error", err)
				}
			}
		}
	}

	return n, nil
}

// ErrNotOwner is returned when a user tries to edit, delete, fulfill, or
// renew a post they did not author.
var ErrNotOwner = errors.New("not the post author")

// ErrPostRemoved is returned when renewing a moderator-removed post —
// removal is terminal for the author (§22 appeals are the only path back).
var ErrPostRemoved = errors.New("post was removed by moderation")

// ErrPostFulfilled is returned when renewing a fulfilled post —
// fulfillment is a terminal lifecycle state (§85).
var ErrPostFulfilled = errors.New("post is already fulfilled")

// ErrInvalid is the generic validation sentinel for lifecycle rejections.
var ErrInvalid = errors.New("invalid post state")

// rowToDTO converts a PostRow (joined query result) to a PostDTO.
func rowToDTO(r PostRow) PostDTO {
	dto := PostDTO{
		ID:          r.ID.String(),
		PostType:    r.PostType,
		Title:       r.Title,
		IsFulfilled: r.IsFulfilled,
		Status:      r.Status,
		ExpiresAt:   formatTs(r.ExpiresAt),
		CreatedAt:   formatTs(r.CreatedAt),
		UpdatedAt:   formatTs(r.UpdatedAt),
	}
	if r.Category != nil {
		dto.Category = *r.Category
	}

	if r.Description != nil {
		dto.Description = *r.Description
	}

	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}

	if r.DestSpaceName != nil {
		dto.DestSpaceName = *r.DestSpaceName
	}

	if r.DestSpaceCode != nil {
		dto.DestSpaceCode = *r.DestSpaceCode
	}

	if r.CitySpaceID.Valid {
		dto.CitySpaceID = r.CitySpaceID.String()
	}

	if r.CitySpaceName != nil {
		dto.CitySpaceName = *r.CitySpaceName
	}

	if r.CitySpaceCode != nil {
		dto.CitySpaceCode = *r.CitySpaceCode
	}

	if r.UserID.Valid {
		dto.AuthorID = r.UserID.String()
	}

	if r.AuthorName != nil {
		dto.AuthorName = *r.AuthorName
	}

	if r.AuthorAvatar != nil {
		dto.AuthorAvatar = *r.AuthorAvatar
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
