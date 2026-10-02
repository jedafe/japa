// Package digest implements the Community Digest Widget (§84, ADR-092).
// // Periodic round-ups of community activity (new posts, trending threads,
// upcoming events, notable wiki edits) surfaced as a widget on the space
// home and optionally delivered via email. Editions move through a
// draft → scheduled → published lifecycle and are reviewed by a moderator+
// before publication.
// // Ref: japaconnect-engineering/sections/84-community-digest.md
// //	japaconnect-engineering/adr/ADR-092-community-digest.md
package digest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store is the data-access layer for digest_editions.
type Store struct {
	pool *db.DB
}

// NewStore creates a digest store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// DigestEdition represents a single digest_editions row.
// //nolint:revive // exported: stutter naming is intentional
type DigestEdition struct {
	ID pgtype.UUID

	SpaceID      pgtype.UUID
	Cadence      string
	Status       string // draft|review|published|cancelled (§84)
	ScheduledFor pgtype.Timestamptz
	PublishedAt  pgtype.Timestamptz
	Content      []byte // jsonb
	EmailSent    bool
	EmailSentAt  pgtype.Timestamptz
	GeneratedBy  string // TEXT per §84 schema ('system'|'ai'|'manual')
	ReviewedBy   pgtype.UUID
	CreatedAt    pgtype.Timestamptz
}

// EditionRow is the full row shape for digest_editions returned by update
// and read-back operations. JSON tags keep AI tool payloads snake_case.
type EditionRow struct {
	ID pgtype.UUID `json:"id"`

	SpaceID      pgtype.UUID        `json:"space_id,omitempty"`
	Cadence      string             `json:"cadence"`
	Status       string             `json:"status"`
	ScheduledFor pgtype.Timestamptz `json:"scheduled_for,omitempty"`
	PublishedAt  pgtype.Timestamptz `json:"published_at,omitempty"`
	Content      json.RawMessage    `json:"content"`
	EmailSent    bool               `json:"email_sent"`
	EmailSentAt  pgtype.Timestamptz `json:"email_sent_at,omitempty"`
	GeneratedBy  string             `json:"generated_by"`
	ReviewedBy   pgtype.UUID        `json:"reviewed_by,omitempty"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
}

// UpdateEditionInput holds the mutable fields for a partial update of
// digest_editions. Pointer fields mean "leave unchanged"; non-nil values
// replace the existing column.
type UpdateEditionInput struct {
	SpaceID      *pgtype.UUID        `json:"space_id,omitempty"`
	Cadence      *string             `json:"cadence,omitempty"`
	Status       *string             `json:"status,omitempty"`
	ScheduledFor *pgtype.Timestamptz `json:"scheduled_for,omitempty"`
	PublishedAt  *pgtype.Timestamptz `json:"published_at,omitempty"`
	Content      *json.RawMessage    `json:"content,omitempty"`
	EmailSent    *bool               `json:"email_sent,omitempty"`
	EmailSentAt  *pgtype.Timestamptz `json:"email_sent_at,omitempty"`
	GeneratedBy  *string             `json:"generated_by,omitempty"`
	ReviewedBy   *pgtype.UUID        `json:"reviewed_by,omitempty"`
}

// CreateEdition inserts a new digest edition and returns its id.
func (s *Store) CreateEdition(ctx context.Context, edition DigestEdition) (pgtype.UUID, error) {
	var id pgtype.UUID

	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO digest_editions (space_id, cadence, status, scheduled_for,
    content, generated_by) VALUES ($1, $2, COALESCE($3, 'draft'), $4, $5, $6)
RETURNING id
`, edition.SpaceID, edition.Cadence, edition.Status, edition.ScheduledFor, edition.Content, edition.GeneratedBy).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("digest: create edition: %w", err)
	}

	return id, nil
}

// GetEdition returns a single digest edition by id within a platform.
func (s *Store) GetEdition(ctx context.Context, id pgtype.UUID) (DigestEdition, error) {
	const q = `
SELECT id, space_id, cadence, status, scheduled_for, published_at,
       content, email_sent, email_sent_at, generated_by, reviewed_by, created_at
FROM digest_editions
WHERE id = $1
LIMIT 1
`

	var e DigestEdition

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&e.ID, &e.SpaceID, &e.Cadence, &e.Status,
		&e.ScheduledFor, &e.PublishedAt, &e.Content, &e.EmailSent,
		&e.EmailSentAt, &e.GeneratedBy, &e.ReviewedBy, &e.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DigestEdition{}, pgx.ErrNoRows
		}

		return DigestEdition{}, fmt.Errorf("digest: get edition: %w", err)
	}

	return e, nil
}

// GetLatestPublished returns the most recently published edition for a platform
// (and optionally a space). If spaceID is not valid, the platform-wide digest
// is returned.
func (s *Store) GetLatestPublished(ctx context.Context, spaceID pgtype.UUID) (DigestEdition, error) {
	var (
		q    strings.Builder
		args []any
	)

	q.WriteString(`
SELECT id, space_id, cadence, status, scheduled_for, published_at,
       content, email_sent, email_sent_at, generated_by, reviewed_by, created_at
FROM digest_editions
WHERE status = 'published'
`)

	argIdx := 1
	if spaceID.Valid {
		q.WriteString(fmt.Sprintf(" AND space_id = $%d", argIdx))

		args = append(args, spaceID)
		argIdx++
	}

	q.WriteString(" ORDER BY published_at DESC LIMIT 1")

	var e DigestEdition

	err := s.pool.Reader.QueryRow(ctx, q.String(), args...).Scan(
		&e.ID, &e.SpaceID, &e.Cadence, &e.Status,
		&e.ScheduledFor, &e.PublishedAt, &e.Content, &e.EmailSent,
		&e.EmailSentAt, &e.GeneratedBy, &e.ReviewedBy, &e.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DigestEdition{}, pgx.ErrNoRows
		}

		return DigestEdition{}, fmt.Errorf("digest: get latest published: %w", err)
	}

	return e, nil
}

// ListEditionsFilter holds filter parameters for the admin edition list.
type ListEditionsFilter struct {
	SpaceID pgtype.UUID
	Status  string
	Query   string // free-text search over cadence/status/generated_by
	Sort    string // cadence|status|email_sent|created_at|published_at|scheduled_for (default: created_at)
	Order   string // asc|desc (default: desc)
	Limit   int32
	Offset  int32
}

// allowedSortColumns maps sort keys to their qualified column names. Only
// these columns may appear in the ORDER BY clause (SQLi guard).
var allowedSortColumns = map[string]string{
	"cadence":       "cadence",
	"status":        "status",
	"email_sent":    "email_sent",
	"created_at":    "created_at",
	"published_at":  "published_at",
	"scheduled_for": "scheduled_for",
}

// ListEditions returns all digest editions for admin management (paginated,
// filterable, sortable).
func (s *Store) ListEditions(ctx context.Context, f ListEditionsFilter) ([]DigestEdition, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.SpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("space_id = $%d", argIdx))
		args = append(args, f.SpaceID)
		argIdx++
	}

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if q := strings.TrimSpace(f.Query); q != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(cadence ILIKE $%d OR status ILIKE $%d OR generated_by ILIKE $%d)",
			argIdx, argIdx, argIdx))
		args = append(args, "%"+q+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 200 {
		limit = 200
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// Build ORDER BY from whitelisted sort column + direction.
	sortCol := allowedSortColumns[f.Sort]
	if sortCol == "" {
		sortCol = "created_at"
	}

	orderDir := "DESC"
	if strings.ToUpper(f.Order) == "ASC" {
		orderDir = "ASC"
	}

	listQ := fmt.Sprintf(`
SELECT id, space_id, cadence, status, scheduled_for, published_at,
       content, email_sent, email_sent_at, generated_by, reviewed_by, created_at
FROM digest_editions
WHERE %s
ORDER BY %s %s
LIMIT $%d OFFSET $%d
`, whereClause, sortCol, orderDir, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("digest: list editions: %w", err)
	}
	defer rows.Close()

	var out []DigestEdition

	for rows.Next() {
		var e DigestEdition
		if err := rows.Scan(
			&e.ID, &e.SpaceID, &e.Cadence, &e.Status,
			&e.ScheduledFor, &e.PublishedAt, &e.Content, &e.EmailSent,
			&e.EmailSentAt, &e.GeneratedBy, &e.ReviewedBy, &e.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("digest: scan edition: %w", err)
		}

		out = append(out, e)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("digest: iterate editions: %w", err)
	}

	countQ := "SELECT COUNT(*) FROM digest_editions WHERE " + whereClause
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("digest: count editions: %w", err)
	}

	return out, total, nil
}

// UpdateEdition applies a platform-scoped partial update to a digest_editions
// row. Only non-nil fields in the input are written. The updated row is
// returned. It returns pgx.ErrNoRows when the row does not exist for the
// platform.
func (s *Store) UpdateEdition(ctx context.Context, id pgtype.UUID, in UpdateEditionInput) (EditionRow, error) {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.SpaceID != nil {
		sets = append(sets, fmt.Sprintf("space_id = $%d", argIdx))
		args = append(args, *in.SpaceID)
		argIdx++
	}

	if in.Cadence != nil {
		sets = append(sets, fmt.Sprintf("cadence = $%d", argIdx))
		args = append(args, *in.Cadence)
		argIdx++
	}

	if in.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *in.Status)
		argIdx++
	}

	if in.ScheduledFor != nil {
		sets = append(sets, fmt.Sprintf("scheduled_for = $%d", argIdx))
		args = append(args, *in.ScheduledFor)
		argIdx++
	}

	if in.PublishedAt != nil {
		sets = append(sets, fmt.Sprintf("published_at = $%d", argIdx))
		args = append(args, *in.PublishedAt)
		argIdx++
	}

	if in.Content != nil {
		sets = append(sets, fmt.Sprintf("content = $%d", argIdx))
		args = append(args, []byte(*in.Content))
		argIdx++
	}

	if in.EmailSent != nil {
		sets = append(sets, fmt.Sprintf("email_sent = $%d", argIdx))
		args = append(args, *in.EmailSent)
		argIdx++
	}

	if in.EmailSentAt != nil {
		sets = append(sets, fmt.Sprintf("email_sent_at = $%d", argIdx))
		args = append(args, *in.EmailSentAt)
		argIdx++
	}

	if in.GeneratedBy != nil {
		sets = append(sets, fmt.Sprintf("generated_by = $%d", argIdx))
		args = append(args, *in.GeneratedBy)
		argIdx++
	}

	if in.ReviewedBy != nil {
		sets = append(sets, fmt.Sprintf("reviewed_by = $%d", argIdx))
		args = append(args, *in.ReviewedBy)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetEditionRow(ctx, id)
	}

	args = append(args, id)
	q := fmt.Sprintf(`
UPDATE digest_editions
SET %s
WHERE id = $%d
RETURNING id, space_id, cadence, status, scheduled_for, published_at,
       content, email_sent, email_sent_at, generated_by, reviewed_by, created_at
`, strings.Join(sets, ", "), argIdx)

	var r EditionRow

	var content []byte

	err := s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&r.ID, &r.SpaceID, &r.Cadence, &r.Status,
		&r.ScheduledFor, &r.PublishedAt, &content, &r.EmailSent,
		&r.EmailSentAt, &r.GeneratedBy, &r.ReviewedBy, &r.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EditionRow{}, pgx.ErrNoRows
		}

		return EditionRow{}, fmt.Errorf("digest: update edition: %w", err)
	}

	r.Content = json.RawMessage(content)

	return r, nil
}

// DeleteEdition hard-deletes a single digest edition scoped to a platform.
// It returns pgx.ErrNoRows if the row does not exist for the platform.
func (s *Store) DeleteEdition(ctx context.Context, id pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
DELETE FROM digest_editions
WHERE id = $1
`, id)
	if err != nil {
		return fmt.Errorf("digest: delete edition: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// GetEditionRow returns a single digest edition row for tool read-backs.
func (s *Store) GetEditionRow(ctx context.Context, id pgtype.UUID) (EditionRow, error) {
	const q = `
SELECT id, space_id, cadence, status, scheduled_for, published_at,
       content, email_sent, email_sent_at, generated_by, reviewed_by, created_at
FROM digest_editions
WHERE id = $1
LIMIT 1
`

	var r EditionRow

	var content []byte

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.SpaceID, &r.Cadence, &r.Status,
		&r.ScheduledFor, &r.PublishedAt, &content, &r.EmailSent,
		&r.EmailSentAt, &r.GeneratedBy, &r.ReviewedBy, &r.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EditionRow{}, pgx.ErrNoRows
		}

		return EditionRow{}, fmt.Errorf("digest: get edition row: %w", err)
	}

	r.Content = json.RawMessage(content)

	return r, nil
}

// PublishEdition marks an edition as published and stamps the reviewer.
// Accepts editions in draft or review status (§84 lifecycle).
func (s *Store) PublishEdition(ctx context.Context, id, reviewerID pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE digest_editions
SET status = 'published', published_at = now(), reviewed_by = $2
WHERE id = $1 AND status IN ('draft', 'review', 'scheduled')
`, id, reviewerID)
	if err != nil {
		return fmt.Errorf("digest: publish edition: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// PublishedEdition is the minimal row returned by AutoPublishDueEditions so
// the service can clear draft locks and enqueue digest:send_email per edition.
type PublishedEdition struct {
	ID pgtype.UUID
}

// AutoPublishDueEditions publishes every draft/review edition whose
// 1-hour admin review window has elapsed (scheduled_for <= now()). This is
// the digest:auto_publish cron body (§84: "A 1-hour review window opens after
// generation before auto-publish"). The UPDATE is atomic and returns the
// (id, ) pairs it transitioned to 'published' so the caller can fire
// digest:send_email per edition. Idempotent: once status='published' the row
// no longer matches the WHERE clause, so a duplicate cron run is a no-op.
func (s *Store) AutoPublishDueEditions(ctx context.Context) ([]PublishedEdition, error) {
	rows, err := s.pool.Writer.Query(ctx, `
UPDATE digest_editions
SET status = 'published', published_at = now()
WHERE status IN ('draft', 'review') AND scheduled_for <= now()
RETURNING id
`)
	if err != nil {
		return nil, fmt.Errorf("digest: auto-publish due editions: %w", err)
	}
	defer rows.Close()

	var out []PublishedEdition

	for rows.Next() {
		var e PublishedEdition
		if err := rows.Scan(&e.ID); err != nil {
			return nil, fmt.Errorf("digest: auto-publish due editions: scan: %w", err)
		}

		out = append(out, e)
	}

	return out, rows.Err()
}

// MarkEmailSent stamps email_sent=true and email_sent_at=now() on a
// published edition after the digest:send_email task completes.
func (s *Store) MarkEmailSent(ctx context.Context, id pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE digest_editions
SET email_sent = true, email_sent_at = now()
WHERE id = $1 AND status = 'published'
`, id)
	if err != nil {
		return fmt.Errorf("digest: mark email sent: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// DigestContent is the JSON shape stored in digest_editions.content. Each
// section is assembled by AssembleDigestContent per §84 generation logic.
// JSON keys follow the §84 schema comment: top_threads, new_members, events,
// story, poll. UnmarshalJSON additionally accepts the legacy key spellings
// (upcoming_events, featured_story) so editions stored before the contract
// fix still render.
// //nolint:revive // exported: stutter naming is intentional
type DigestContent struct {
	TopThreads     []DigestThread `json:"top_threads"`
	NewMembers     []DigestMember `json:"new_members"`
	UpcomingEvents []DigestEvent  `json:"events"`
	FeaturedStory  *DigestStory   `json:"story,omitempty"`
	Poll           *DigestPoll    `json:"poll,omitempty"`
}

// UnmarshalJSON decodes digest content, mapping the legacy
// upcoming_events/featured_story keys onto the §84 events/story contract.
func (c *DigestContent) UnmarshalJSON(data []byte) error {
	type plain DigestContent
	var aux struct {
		plain
		LegacyEvents []DigestEvent  `json:"upcoming_events"`
		LegacyStory  *DigestStory   `json:"featured_story"`
		Members      []DigestMember `json:"members"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*c = DigestContent(aux.plain)
	if len(c.UpcomingEvents) == 0 && len(aux.LegacyEvents) > 0 {
		c.UpcomingEvents = aux.LegacyEvents
	}
	if c.FeaturedStory == nil && aux.LegacyStory != nil {
		c.FeaturedStory = aux.LegacyStory
	}
	if len(c.NewMembers) == 0 && len(aux.Members) > 0 {
		c.NewMembers = aux.Members
	}
	return nil
}

// NormalizeContent rewrites a stored content blob so the API always emits the
// §84 key spellings (events/story), even for rows persisted with the legacy
// upcoming_events/featured_story keys. Unknown keys are preserved; unparseable
// blobs are returned as-is.
func NormalizeContent(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return raw
	}
	moved := false
	rename := func(from, to string) {
		v, ok := m[from]
		if !ok {
			return
		}
		if _, exists := m[to]; !exists {
			m[to] = v
		}
		delete(m, from)
		moved = true
	}
	rename("upcoming_events", "events")
	rename("featured_story", "story")
	rename("members", "new_members")
	if !moved {
		return raw
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// DigestThread is a top forum thread summary.
// //nolint:revive // exported: stutter naming is intentional
type DigestThread struct {
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	VoteScore  int    `json:"vote_score"`
	ReplyCount int    `json:"reply_count"`
}

// DigestMember is a new-member summary (display_name only — no PII leak).
// //nolint:revive // exported: stutter naming is intentional
type DigestMember struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
}

// DigestEvent is an upcoming event summary.
// //nolint:revive // exported: stutter naming is intentional
type DigestEvent struct {
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	StartsAt string `json:"starts_at,omitempty"`
}

// DigestStory is the featured story summary.
// //nolint:revive // exported: stutter naming is intentional
type DigestStory struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// DigestPoll is the community poll section (§84 item 5 — admin-seeded or
// auto-suggested poll question; reserved for the poll generation phase).
// //nolint:revive // exported: stutter naming is intentional
type DigestPoll struct {
	Question string `json:"question"`
	URL      string `json:"url,omitempty"`
}

// AssembleDigestContent queries platform analytics and assembles the five
// digest sections per §84 generation logic: top threads, new members,
// upcoming events, and a featured story. The poll section is omitted (it
// requires admin-seeded poll questions and is deferred to a future phase).
// When spaceID is valid, results are scoped to that Space; otherwise the
// platform-wide digest is assembled.
// // Ref: japaconnect-engineering/sections/84-community-digest-widget.md.
func (s *Store) AssembleDigestContent(ctx context.Context, spaceID pgtype.UUID) (json.RawMessage, error) {
	content := DigestContent{}

	// 1. Top threads: top 5 forum threads by (vote_score × 3 + reply_count × 5)
	// in the past 7 days (§84).
	var (
		threadsQ strings.Builder
		tArgs    []any
	)

	threadsQ.WriteString(`
SELECT ft.title, ft.slug, ft.vote_score, ft.reply_count
FROM forum_threads ft
WHERE ft.deleted_at IS NULL
  AND ft.created_at >= now() - interval '7 days'
`)

	argIdx := 1
	if spaceID.Valid {
		threadsQ.WriteString(fmt.Sprintf(" AND ft.space_id = $%d", argIdx))

		tArgs = append(tArgs, spaceID)
		argIdx++
	}

	threadsQ.WriteString(" ORDER BY (ft.vote_score * 3 + ft.reply_count * 5) DESC LIMIT 5")

	trows, err := s.pool.Reader.Query(ctx, threadsQ.String(), tArgs...)
	if err != nil {
		return nil, fmt.Errorf("digest: assemble: top threads: %w", err)
	}

	for trows.Next() {
		var t DigestThread
		if err := trows.Scan(&t.Title, &t.Slug, &t.VoteScore, &t.ReplyCount); err != nil {
			trows.Close()
			return nil, fmt.Errorf("digest: assemble: scan thread: %w", err)
		}

		content.TopThreads = append(content.TopThreads, t)
	}

	trows.Close()

	if err := trows.Err(); err != nil {
		return nil, fmt.Errorf("digest: assemble: iterate threads: %w", err)
	}

	// 2. New members: users created in the past 7 days who completed onboarding
	// (§84). Opt-out is handled by user privacy settings — this query surfaces
	// only username and display_name.
	mrows, err := s.pool.Reader.Query(ctx, `
SELECT u.username, COALESCE(up.display_name, '')
FROM users u
JOIN onboarding_state os ON os.user_id = u.id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE u.created_at >= now() - interval '7 days'
  AND os.completed = true
ORDER BY u.created_at DESC
LIMIT 10
`)
	if err != nil {
		return nil, fmt.Errorf("digest: assemble: new members: %w", err)
	}

	for mrows.Next() {
		var m DigestMember
		if err := mrows.Scan(&m.Username, &m.DisplayName); err != nil {
			mrows.Close()
			return nil, fmt.Errorf("digest: assemble: scan member: %w", err)
		}

		content.NewMembers = append(content.NewMembers, m)
	}

	mrows.Close()

	if err := mrows.Err(); err != nil {
		return nil, fmt.Errorf("digest: assemble: iterate members: %w", err)
	}

	// 3. Upcoming events: next 5 published events (content_type='event'),
	// geo-filtered to the digest's Space where applicable (§84).
	var (
		eventsQ strings.Builder
		eArgs   []any
	)

	eventsQ.WriteString(`
SELECT c.title, c.slug, c.publish_at
FROM contents c
WHERE c.content_type = 'event'
  AND c.status = 'published'
  AND c.publish_at IS NOT NULL
  AND c.publish_at >= now()
`)

	argIdx = 1
	if spaceID.Valid {
		eventsQ.WriteString(fmt.Sprintf(" AND c.space_id = $%d", argIdx))

		eArgs = append(eArgs, spaceID)
		argIdx++
	}

	eventsQ.WriteString(" ORDER BY c.publish_at ASC LIMIT 5")

	erows, err := s.pool.Reader.Query(ctx, eventsQ.String(), eArgs...)
	if err != nil {
		return nil, fmt.Errorf("digest: assemble: upcoming events: %w", err)
	}

	for erows.Next() {
		var e DigestEvent
		var startsAt pgtype.Timestamptz
		if err := erows.Scan(&e.Title, &e.Slug, &startsAt); err != nil {
			erows.Close()
			return nil, fmt.Errorf("digest: assemble: scan event: %w", err)
		}
		if startsAt.Valid {
			e.StartsAt = startsAt.Time.UTC().Format(time.RFC3339)
		}

		content.UpcomingEvents = append(content.UpcomingEvents, e)
	}

	erows.Close()

	if err := erows.Err(); err != nil {
		return nil, fmt.Errorf("digest: assemble: iterate events: %w", err)
	}

	// 4. Featured story: highest engagement story (content_type='story') in the
	// past 30 days (§84). Engagement proxy: comment_count + share_count +
	// reshare_count (content_scores not reliably maintained for all content
	// types).
	var (
		storyQ strings.Builder
		sArgs  []any
	)

	storyQ.WriteString(`
SELECT c.title, c.slug
FROM contents c
WHERE c.content_type = 'story'
  AND c.status = 'published'
  AND c.created_at >= now() - interval '30 days'
`)

	argIdx = 1
	if spaceID.Valid {
		storyQ.WriteString(fmt.Sprintf(" AND c.space_id = $%d", argIdx))

		sArgs = append(sArgs, spaceID)
		argIdx++
	}

	storyQ.WriteString(" ORDER BY (c.comment_count + c.share_count + c.reshare_count) DESC LIMIT 1")

	var story DigestStory

	err = s.pool.Reader.QueryRow(ctx, storyQ.String(), sArgs...).Scan(&story.Title, &story.Slug)
	if err == nil {
		content.FeaturedStory = &story
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("digest: assemble: featured story: %w", err)
	}

	raw, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("digest: assemble: marshal content: %w", err)
	}

	return raw, nil
}

// GetDigestSubscriberEmails returns the emails of all subscribers with
// digest_enabled=true for the given platform. Used by SendEditionEmail to
// dispatch the published digest (§84 email channel, reuses §62 Newsletter
// subscriber infrastructure).
func (s *Store) GetDigestSubscriberEmails(ctx context.Context) ([]string, error) {
	q := gen.New(s.pool.Reader)

	var emails []string

	page := int32(1)

	const pageSize = int32(500)

	for {
		rows, err := q.GetDigestSubscribers(ctx, gen.GetDigestSubscribersParams{

			PageOffset: (page - 1) * pageSize,
			PageLimit:  pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("digest: get subscribers: %w", err)
		}

		if len(rows) == 0 {
			break
		}

		for _, r := range rows {
			emails = append(emails, r.Email)
		}

		if len(rows) < int(pageSize) {
			break
		}

		page++
	}

	return emails, nil
}

// CreateUnsubscribeTokenForEmail resolves the newsletter subscriber by email
// and creates a one-click unsubscribe token, returning the token string.
// Returns ("", nil) if the subscriber is not found (best-effort — the email
// is still sent without an unsubscribe header).
func (s *Store) CreateUnsubscribeTokenForEmail(ctx context.Context, emailAddr string) (string, error) {
	q := gen.New(s.pool.Reader)

	sub, err := q.GetSubscriberByEmail(ctx,
		emailAddr,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}

		return "", fmt.Errorf("digest: resolve subscriber: %w", err)
	}

	token, err := generateUnsubscribeToken()
	if err != nil {
		return "", fmt.Errorf("digest: generate token: %w", err)
	}

	if _, err := q.CreateUnsubscribeToken(ctx, gen.CreateUnsubscribeTokenParams{
		Token:        token,
		SubscriberID: sub.ID,
	}); err != nil {
		return "", fmt.Errorf("digest: persist token: %w", err)
	}

	return token, nil
}

// generateUnsubscribeToken generates a 32-byte hex token for one-click
// unsubscribe, mirroring the worker package's newsletterGenerateUnsubscribeToken.
func generateUnsubscribeToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("digest: generate token: %w", err)
	}

	return hex.EncodeToString(b), nil
}
