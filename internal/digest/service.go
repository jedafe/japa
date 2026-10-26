// Package digest — service.go orchestrates digest edition browsing, draft
// editing, publication, and generation triggers. Feature-flagged via
// digest.enabled (feature_flags gate) and digest.email system_setting (§84).
// // Ref: japaconnect-engineering/sections/84-community-digest.md
// //	japaconnect-engineering/adr/ADR-092-community-digest.md
package digest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jedafe/japa/internal/brand"
	"html/template"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/cdn"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/email"
	"github.com/jedafe/japa/internal/literals"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// digestDraftKeyTTL is the Valkey TTL for jc:digest_draft keys (§84 spec: 2h
// review window).
const digestDraftKeyTTL = 2 * time.Hour

// ErrInvalidContent is returned when a content payload is not a JSON object
// of §84 section keys. Handlers map it to 400.
var ErrInvalidContent = errors.New("digest: content must be a JSON object")

// TaskEnqueuer is the interface for enqueuing Asynq background tasks
// (structural typing — *worker.Enqueuer satisfies this).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// noopEnqueuer silently drops tasks when no real enqueuer is wired (tests,
// development).
type noopEnqueuer struct{}

func (noopEnqueuer) Enqueue(string, []byte, ...interface{}) error { return nil }

// EmailDispatcher is the interface for sending digest emails. Satisfied by
// *email.EmailDispatcher (structural typing). When nil, SendEditionEmail
// logs a warning and skips email delivery (the edition is still marked sent
// so the task does not retry indefinitely on misconfigured environments).
type EmailDispatcher interface {
	Dispatch(ctx context.Context, req email.EmailRequest) (email.EmailResponse, error)
}

// FeatureChecker checks whether a feature flag is enabled for a platform.
// Satisfied by *features.Checker via structural typing — defined locally to
// avoid importing the features package (keeps the dependency graph clean).
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// DigestEditionDTO is the API response shape for a digest edition (listing).
// //nolint:revive // exported: stutter naming is intentional.
type DigestEditionDTO struct {
	ID           string `json:"id"`
	SpaceID      string `json:"space_id,omitempty"`
	Cadence      string `json:"cadence"`
	Status       string `json:"status"`
	ScheduledFor string `json:"scheduled_for,omitempty"`
	PublishedAt  string `json:"published_at,omitempty"`
	EmailSent    bool   `json:"email_sent"`
	GeneratedBy  string `json:"generated_by,omitempty"`
	ReviewedBy   string `json:"reviewed_by,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// DigestEditionDetailDTO is the full digest edition with content.
// //nolint:revive // exported: stutter naming is intentional.
type DigestEditionDetailDTO struct {
	DigestEditionDTO
	Content json.RawMessage `json:"content"`
}

// literalCatalog resolves {{...}} template literals inside digest content
// JSON (§14 Global Template Literals). Implemented by *literals.Catalog;
// nil disables resolution.
type literalCatalog interface {
	ResolveJSON(ctx context.Context, raw json.RawMessage, rc literals.RenderContext) json.RawMessage
}

// Service holds the business logic for the Community Digest.
type Service struct {
	store           *Store
	settings        *config.Settings
	enqueuer        TaskEnqueuer
	vk              valkeycompat.Cmdable
	emailDispatcher EmailDispatcher
	checker         FeatureChecker
	lit             literalCatalog // nil disables literal resolution (§14)
}

// NewService creates a digest service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings, enqueuer: noopEnqueuer{}}
}

// SetEnqueuer injects the Asynq task enqueuer used to dispatch
// digest:generate and digest:send_email tasks (§84, Rule 7/8).
func (s *Service) SetEnqueuer(e TaskEnqueuer) {
	if e == nil {
		s.enqueuer = noopEnqueuer{}
		return
	}

	s.enqueuer = e
}

// SetValkey injects the Valkey client used for the jc:digest_draft lock
// (§84, valkey-keys-catalog.md, Rule 4).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// SetEmailDispatcher injects the email dispatcher used by SendEditionEmail
// to deliver the published digest to subscribers (§84 email channel, reuses
// §62 Newsletter infrastructure via the ADR-015 marketing provider).
func (s *Service) SetEmailDispatcher(d EmailDispatcher) { s.emailDispatcher = d }

// SetFeatureChecker injects the feature flag checker so Enabled can read the
// feature_flags table (the single source of truth for feature reachability)
// instead of system_settings. Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// SetLiteralCatalog injects the template-literal catalog so published digest
// content renders {{...}} placeholders (§14). Applied on public reads and
// outbound email — never on the admin edit payload, which must keep the
// authored tokens.
func (s *Service) SetLiteralCatalog(c literalCatalog) { s.lit = c }

// resolveContent resolves {{...}} placeholders inside a digest content JSON
// payload using a request-independent context — digest output is shared.
func (s *Service) resolveContent(ctx context.Context, raw json.RawMessage) json.RawMessage {
	if s.lit == nil {
		return raw
	}

	return s.lit.ResolveJSON(ctx, raw, literals.NewContext(ctx, ""))
}

// Enabled checks whether the digest feature is enabled for the given platform.
// Delegates to the features.Checker (feature_flags table) when available,
// falling back to system_settings for backward compatibility.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "digest.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "digest.enabled", false)
}

// EmailEnabled reads the digest.email system_setting toggle.
func (s *Service) EmailEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "digest.email", false)
}

// GetLatest returns the most recently published edition for a platform
// (and optionally a space).
func (s *Service) GetLatest(ctx context.Context, spaceID pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetLatestPublished(ctx, spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}

		return DigestEditionDetailDTO{}, fmt.Errorf("digest: get latest: %w", err)
	}

	dto := editionToDetailDTO(edition)
	dto.Content = s.resolveContent(ctx, dto.Content)

	return dto, nil
}

// Get returns a single published edition by id (public browse).
func (s *Service) Get(ctx context.Context, id pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetEdition(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}

		return DigestEditionDetailDTO{}, fmt.Errorf("digest: get: %w", err)
	}

	if edition.Status != "published" {
		return DigestEditionDetailDTO{}, pgx.ErrNoRows
	}

	dto := editionToDetailDTO(edition)
	dto.Content = s.resolveContent(ctx, dto.Content)

	return dto, nil
}

// AdminListFilter holds admin list filter parameters (§84 admin pattern:
// filter + sort + paginate, no infinite scroll).
type AdminListFilter struct {
	SpaceID pgtype.UUID
	Status  string // draft|review|published|cancelled
	Query   string // free-text search over cadence/status/generated_by
	Sort    string // cadence|status|email_sent|created_at|published_at|scheduled_for (default: created_at)
	Order   string // asc|desc (default: desc)
}

// AdminList returns all digest editions for admin management (paginated,
// filterable by space/status, sortable).
func (s *Service) AdminList(ctx context.Context, f AdminListFilter, page, limit int32) ([]DigestEditionDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	editions, total, err := s.store.ListEditions(ctx, ListEditionsFilter{
		SpaceID: f.SpaceID,
		Status:  f.Status,
		Query:   f.Query,
		Sort:    f.Sort,
		Order:   f.Order,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("digest: admin list: %w", err)
	}

	dtos := make([]DigestEditionDTO, 0, len(editions))
	for _, e := range editions {
		dtos = append(dtos, editionToDTO(e))
	}

	return dtos, total, nil
}

// AdminGet returns a single edition (any status) for admin management.
func (s *Service) AdminGet(ctx context.Context, id pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetEdition(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}

		return DigestEditionDetailDTO{}, fmt.Errorf("digest: admin get: %w", err)
	}

	return editionToDetailDTO(edition), nil
}

// AdminUpdate edits the content of a draft or review-status edition before
// publication. It preserves the §84 lifecycle guard by refusing to update
// editions that are already published or cancelled.
func (s *Service) AdminUpdate(ctx context.Context, id pgtype.UUID, content json.RawMessage) error {
	if len(content) == 0 {
		return errors.New("content is required")
	}

	// §84: content is a JSON object of section keys — reject scalars/arrays
	// before they hit storage and break renderDigestEmail.
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(content, &shape); err != nil || shape == nil {
		return ErrInvalidContent
	}

	e, err := s.store.GetEdition(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("digest: admin update: %w", err)
	}

	if e.Status != "draft" && e.Status != "review" {
		return pgx.ErrNoRows
	}

	_, err = s.store.UpdateEdition(ctx, id, UpdateEditionInput{Content: &content})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("digest: admin update: %w", err)
	}

	return nil
}

// AdminPublish publishes a draft or review edition, enqueues the
// digest:send_email task (fire-and-forget, Rule 8), and clears the
// jc:digest_draft Valkey key (§84, ADR-092).
func (s *Service) AdminPublish(ctx context.Context, reviewerID, id pgtype.UUID) error {
	if err := s.store.PublishEdition(ctx, id, reviewerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("digest: admin publish: %w", err)
	}

	// Clear the draft review lock (§84, valkey-keys-catalog.md).
	// ADR-134: publishing changes the public edition page.
	cdn.PurgePublicPaths(ctx, []string{"digest_edition:" + id.String()}, "/digest/"+id.String(), "/digest")

	s.clearDraftLock(ctx, id)

	// Fire-and-forget: enqueue digest:send_email so members with the digest
	// notification preference receive the published edition (§84, Rule 8).
	// Reuses §62 Newsletter infrastructure in the task handler.
	payload, _ := json.Marshal(map[string]string{
		"edition_id": id.String(),
	})
	if err := s.enqueuer.Enqueue("digest:send_email", payload); err != nil {
		// Log but do not fail the publish — email delivery is async and
		// retryable. The edition is already published.
		slog.Error("digest: enqueue send_email failed", "error", err, "edition_id", id.String())
	}

	return nil
}

// AdminGenerate triggers generation of a new digest edition by enqueuing the
// digest:generate Asynq task (§84, ADR-092). The task assembles the edition
// from platform analytics — no AI generation (purely metrics-driven).
// Returns "accepted" to indicate the task was enqueued (202 response).
func (s *Service) AdminGenerate(ctx context.Context) (string, error) {
	payload, _ := json.Marshal(map[string]string{})
	if err := s.enqueuer.Enqueue("digest:generate", payload); err != nil {
		return "", fmt.Errorf("digest: enqueue generate: %w", err)
	}

	return "accepted", nil
}

// GenerateEdition is the digest:generate task processor (§84, ADR-092). It
// assembles a new digest edition from platform analytics (top threads, new
// members, upcoming events, featured story) and stores it in draft status
// for the admin review window. When ID is the zero UUID (cron mode),
// it iterates all active platforms. When spaceID is valid, the edition is
// scoped to that Space; otherwise it is platform-wide.
// // This method satisfies the worker.DigestProcessor interface. It is
// idempotent per ( space_id, cadence, week) — a duplicate
// generate within the same week overwrites the existing draft rather than
// creating a second one (the admin review window handles the rest).
// // Ref: japaconnect-engineering/sections/84-community-digest-widget.md.
func (s *Service) GenerateEdition(ctx context.Context, spaceID pgtype.UUID) error {
	return s.generateForPlatform(ctx, spaceID)
}

// generateForPlatform assembles and stores a single digest edition for one
// platform (and optionally one Space).
func (s *Service) generateForPlatform(ctx context.Context, spaceID pgtype.UUID) error {
	if !s.Enabled(ctx) {
		slog.Info("task:digest:generate: feature disabled, skipping")
		return nil
	}

	content, err := s.store.AssembleDigestContent(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("digest: generate: assemble content: %w", err)
	}

	// §84: "A 1-hour review window opens after generation before
	// auto-publish." scheduled_for gates the digest:auto_publish cron, which
	// publishes draft/review editions once scheduled_for <= now(). Admins
	// can still publish early via AdminPublish (which short-circuits the
	// window). Ref: japaconnect-engineering/sections/84-community-digest-widget.md
	editionID, err := s.store.CreateEdition(ctx, DigestEdition{
		SpaceID:      spaceID,
		Cadence:      "weekly",
		Status:       "draft",
		ScheduledFor: pgtype.Timestamptz{Time: time.Now().Add(1 * time.Hour), Valid: true},
		Content:      content,
		GeneratedBy:  "system",
	})
	if err != nil {
		return fmt.Errorf("digest: generate: create edition: %w", err)
	}

	// Set the draft review lock so the admin review window is tracked (§84).
	s.setDraftLock(ctx, editionID)

	slog.Info("task:digest:generate: edition assembled", "edition_id", editionID, "space_id", spaceID.Valid)

	return nil
}

// AutoPublishDueEditions is the digest:auto_publish task processor (§84,
// ADR-092). It publishes every draft/review edition whose 1-hour admin review
// window has elapsed (scheduled_for <= now()), clears each edition's draft
// lock, and enqueues digest:send_email fire-and-forget per edition (Rule 8).
// // This method satisfies the worker.DigestProcessor interface. It is
// idempotent: the store UPDATE only matches rows still in draft/review, so a
// duplicate cron run publishes nothing and enqueues nothing.
// // Ref: japaconnect-engineering/sections/84-community-digest-widget.md.
func (s *Service) AutoPublishDueEditions(ctx context.Context) (int64, error) {
	published, err := s.store.AutoPublishDueEditions(ctx)
	if err != nil {
		return 0, fmt.Errorf("digest: auto-publish: %w", err)
	}

	for _, e := range published {
		// Clear the draft review lock (mirrors AdminPublish, §84).
		s.clearDraftLock(ctx, e.ID)

		// ADR-134: auto-published edition — same purge as AdminPublish.
		cdn.PurgePublicPaths(ctx, []string{"digest_edition:" + e.ID.String()}, "/digest/"+e.ID.String(), "/digest")

		// Fire-and-forget: enqueue digest:send_email so subscribed members
		// receive the now-published edition. A failed enqueue is logged but
		// does not fail the sweep — the edition is already published and an
		// admin can re-trigger email from the panel.
		payload, _ := json.Marshal(map[string]string{
			"edition_id": e.ID.String(),
		})
		if err := s.enqueuer.Enqueue("digest:send_email", payload); err != nil {
			slog.Error("digest: auto-publish enqueue send_email failed", "error", err, "edition_id", e.ID.String())
		}
	}

	slog.Info("task:digest:auto_publish: complete", "published", len(published))

	return int64(len(published)), nil
}

// SendEditionEmail is the digest:send_email task processor (§84, ADR-092).
// It loads a published edition, renders its content into HTML, and sends it
// to all subscribers with digest_enabled=true via the ADR-015 marketing
// provider. One-click unsubscribe headers are generated per recipient
// (reusing §62 Newsletter unsubscribe token infrastructure). After
// dispatch, the edition's email_sent flag is stamped.
// // This method satisfies the worker.DigestProcessor interface.
// // Ref: japaconnect-engineering/sections/84-community-digest-widget.md.
func (s *Service) SendEditionEmail(ctx context.Context, editionID pgtype.UUID) error {
	if !editionID.Valid {
		return errors.New("digest: send_email: edition_id is required")
	}

	if !s.EmailEnabled(ctx) {
		slog.Info("task:digest:send_email: email disabled, skipping")
		return nil
	}

	if s.emailDispatcher == nil {
		slog.Warn("task:digest:send_email: no email dispatcher wired, skipping")
		return nil
	}

	edition, err := s.store.GetEdition(ctx, editionID)
	if err != nil {
		return fmt.Errorf("digest: send_email: load edition: %w", err)
	}

	if edition.Status != "published" {
		return fmt.Errorf("digest: send_email: edition %s is not published (status=%s)",
			editionID, edition.Status)
	}

	subscribers, err := s.store.GetDigestSubscriberEmails(ctx)
	if err != nil {
		return fmt.Errorf("digest: send_email: list subscribers: %w", err)
	}

	if len(subscribers) == 0 {
		slog.Info("task:digest:send_email: no subscribers, skipping")
		return nil
	}

	baseURL := s.resolveBaseURL(ctx)
	subject := s.emailSubject(ctx)
	// §14: resolve {{...}} placeholders before HTML render so outbound
	// email never contains raw literals.
	htmlBody := renderDigestEmail(s.resolveContent(ctx, edition.Content), baseURL)

	sent, failed := 0, 0

	for _, recipient := range subscribers {
		unsubURL, headers := s.unsubscribeLink(ctx, recipient, baseURL)
		body := htmlBody

		if unsubURL != "" {
			body = appendUnsubscribeLinkToHTML(body, unsubURL)
		}

		if _, err := s.emailDispatcher.Dispatch(ctx, email.EmailRequest{
			To:       recipient,
			Subject:  subject,
			Body:     body,
			IsHTML:   true,
			Category: "transactional",
			Headers:  headers,
		}); err != nil {
			slog.Warn("task:digest:send_email: send failed", "recipient", recipient, "error", err)

			failed++

			continue
		}

		sent++
	}

	// Stamp email_sent so the admin panel reflects delivery status.
	if err := s.store.MarkEmailSent(ctx, editionID); err != nil {
		slog.Warn("task:digest:send_email: mark email_sent failed", "error", err)
	}

	slog.Info("task:digest:send_email: complete", "edition_id", editionID, "subscribers", len(subscribers), "sent", sent, "failed", failed)

	return nil
}

// resolveBaseURL returns the platform's site base URL from system_settings,
// falling back to the JC_SITE_URL env var (mirrors worker.resolveBaseURL).
func (s *Service) resolveBaseURL(ctx context.Context) string {
	if s.settings != nil {
		if v := s.settings.GetStringOr(ctx, "site.base_url", ""); v != "" {
			return v
		}
	}

	return os.Getenv("JC_SITE_URL")
}

// emailSubject resolves the site name for the digest email subject line.
// Reads general.site_name — the catalogued key (site.name was a stale key
// that admin edits never populated).
func (s *Service) emailSubject(ctx context.Context) string {
	name := brand.Get()

	if s.settings != nil {
		if v := s.settings.GetStringOr(ctx, "general.site_name", ""); v != "" {
			name = v
		}
	}

	return "Weekly Community Digest — " + name
}

// unsubscribeLink generates a per-recipient one-click unsubscribe URL + the
// List-Unsubscribe SMTP headers (§62 compliance, ADR-080 §4). Returns
// ("", nil) if the subscriber cannot be resolved (best-effort).
func (s *Service) unsubscribeLink(ctx context.Context, recipient, baseURL string) (string, map[string]string) {
	token, err := s.store.CreateUnsubscribeTokenForEmail(ctx, recipient)
	if err != nil {
		slog.Warn("task:digest:send_email: unsubscribe token failed", "recipient", recipient, "error", err)
		return "", nil
	}

	if token == "" {
		return "", nil
	}

	base := baseURL
	if base == "" {
		base = os.Getenv("JC_SITE_URL")
	}

	url := strings.TrimRight(base, "/") + "/unsubscribe?token=" + token
	headers := map[string]string{
		"List-Unsubscribe":      "<" + url + ">",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}

	return url, headers
}

// renderDigestEmail renders the edition content JSON into an HTML email
// body using html/template for XSS safety. The template mirrors the v1
// HandleNewsletterDigest layout (§62).
func renderDigestEmail(content []byte, baseURL string) string {
	var c DigestContent
	_ = json.Unmarshal(content, &c)

	const tmpl = `<div style="font-family: sans-serif; max-width: 600px; margin: 0 auto;">
<h2>Weekly Community Digest</h2>
{{if .TopThreads}}<h3>Top Threads</h3><ul>{{range .TopThreads}}<li><a href="{{$.BaseURL}}/forum/{{.Slug}}">{{.Title}}</a></li>{{end}}</ul>{{end}}
{{if .NewMembers}}<h3>New Members</h3><ul>{{range .NewMembers}}<li>{{if .DisplayName}}{{.DisplayName}}{{else}}{{.Username}}{{end}}</li>{{end}}</ul>{{end}}
{{if .UpcomingEvents}}<h3>Upcoming Events</h3><ul>{{range .UpcomingEvents}}<li><a href="{{$.BaseURL}}/{{.Slug}}">{{.Title}}</a></li>{{end}}</ul>{{end}}
{{if .FeaturedStory}}<h3>Featured Story</h3><p><a href="{{$.BaseURL}}/{{.FeaturedStory.Slug}}">{{.FeaturedStory.Title}}</a></p>{{end}}
</div>`

	t, err := template.New("digest").Parse(tmpl)
	if err != nil {
		return "<div style=\"font-family: sans-serif;\"><h2>Weekly Community Digest</h2></div>"
	}

	data := struct {
		BaseURL string
		DigestContent
	}{BaseURL: baseURL, DigestContent: c}

	var body strings.Builder
	_ = t.Execute(&body, data)

	return body.String()
}

// appendUnsubscribeLinkToHTML appends a visible unsubscribe link to the
// email body before </body> (or at the end). Mirrors worker.appendUnsubscribeLink.
func appendUnsubscribeLinkToHTML(htmlBody, unsubURL string) string {
	footer := fmt.Sprintf(`<p style="margin-top: 24px; font-size: 12px; color: #888;"><a href="%s">Unsubscribe</a></p>`, unsubURL)
	if strings.Contains(htmlBody, "</body>") {
		return strings.Replace(htmlBody, "</body>", footer+"</body>", 1)
	}

	return htmlBody + footer
}

// setDraftLock sets jc:digest_draft:{edition_id} with a 2h TTL
// marking the edition as pending admin review (§84, valkey-keys-catalog.md).
func (s *Service) setDraftLock(ctx context.Context, editionID pgtype.UUID) {
	if s.vk == nil || !editionID.Valid {
		return
	}

	key := "jc:digest_draft:" + editionID.String()
	if err := s.vk.Set(ctx, key, "1", digestDraftKeyTTL).Err(); err != nil {
		slog.Error("digest: set draft lock", "error", err, "key", key)
	}
}

// clearDraftLock removes the jc:digest_draft key on publish or cancel.
func (s *Service) clearDraftLock(ctx context.Context, editionID pgtype.UUID) {
	if s.vk == nil || !editionID.Valid {
		return
	}

	key := "jc:digest_draft:" + editionID.String()
	if err := s.vk.Del(ctx, key).Err(); err != nil {
		slog.Error("digest: clear draft lock", "error", err, "key", key)
	}
}

// editionToDTO converts a DigestEdition to a DigestEditionDTO.
func editionToDTO(e DigestEdition) DigestEditionDTO {
	dto := DigestEditionDTO{
		ID:        e.ID.String(),
		Cadence:   e.Cadence,
		Status:    e.Status,
		EmailSent: e.EmailSent,
		CreatedAt: formatTs(e.CreatedAt),
	}
	if e.SpaceID.Valid {
		dto.SpaceID = e.SpaceID.String()
	}

	dto.ScheduledFor = formatTs(e.ScheduledFor)
	dto.PublishedAt = formatTs(e.PublishedAt)

	if e.GeneratedBy != "" {
		dto.GeneratedBy = e.GeneratedBy
	}

	if e.ReviewedBy.Valid {
		dto.ReviewedBy = e.ReviewedBy.String()
	}

	return dto
}

// editionToDetailDTO converts a DigestEdition to a DigestEditionDetailDTO.
func editionToDetailDTO(e DigestEdition) DigestEditionDetailDTO {
	dto := DigestEditionDetailDTO{
		DigestEditionDTO: editionToDTO(e),
	}
	if len(e.Content) > 0 {
		// §84: emit the canonical section keys (events/story) even for rows
		// persisted with the legacy upcoming_events/featured_story spellings.
		dto.Content = json.RawMessage(NormalizeContent(e.Content))
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
