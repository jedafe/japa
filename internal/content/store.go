package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/seo"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/jedafe/japa/internal/wallet"
	"github.com/jedafe/japa/internal/worker"
	"github.com/pgvector/pgvector-go"
)

// ---------------------------------------------------------------------------
// Contents — CRUD
// ---------------------------------------------------------------------------

// CreateContentInput holds the parameters for creating a new content row.
// The Service layer derives slug uniqueness, read time, and content hash
// from these inputs.
type CreateContentInput struct {
	AuthorID        pgtype.UUID
	OrganizationID  pgtype.UUID
	SpaceID         pgtype.UUID
	ContentType     string
	Title           string
	Slug            string
	Body            string
	BodyHtml        string //nolint:revive // matches sqlc-generated field name
	FieldValues     []byte
	ContentHash     *string
	GeoSpaceID      pgtype.UUID
	HeaderImageID   pgtype.UUID
	FeaturedImageID pgtype.UUID
	VideoSourceID   pgtype.UUID
	CanonicalUrl    *string //nolint:revive // matches sqlc-generated field name
	MetaTitle       *string
	MetaDescription *string
	OgImageUrl      *string //nolint:revive // matches sqlc-generated field name
	SeoIndexable    bool
	Status          string
	Visibility      string
	PublishAt       pgtype.Timestamptz
	GuideCategory   *string
	// Journey-aware recommendation metadata (§14 / ADR-064a).
	JourneyStage  *string
	VisaRoute     *string
	SourceCountry *string
	TargetCountry *string
	// SkipStoryModeration lets moderator callers (story.verify,
	// content.edit_any, super-admin) set published/scheduled directly on
	// content_type='story' — author-facing writes always clamp to in_review.
	SkipStoryModeration bool
	// AI provenance fields (ADR-088). When AIGenerated is true, the service
	// stamps ai_generated_at with the current time if the caller leaves it
	// empty. AIModel records the model/category used for the draft.
	AIGenerated   bool
	AIGeneratedAt pgtype.Timestamptz
	AIModel       *string
	// Source provenance for promoted/ingested content (v1 sources directory).
	IntelSourceID pgtype.UUID
	IntelRecordID pgtype.UUID
}

// resolveSpaceCountryCode walks a space's parent_id chain until it reaches
// a country and returns its 2-letter geo_code. Returns "" for non-geo spaces
// or when the chain has no country.
func resolveSpaceCountryCode(ctx context.Context, q *gen.Queries, spaceID pgtype.UUID) (string, error) {
	const maxDepth = 5

	currentID := spaceID
	for range maxDepth {
		space, err := q.GetSpaceByID(ctx, currentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", nil
			}

			return "", err
		}

		if space.Level == "country" {
			if space.GeoCode != nil {
				return strings.ToUpper(strings.TrimSpace(*space.GeoCode)), nil
			}

			return "", nil
		}

		if !space.ParentID.Valid || space.ParentID == space.ID {
			return "", nil
		}

		currentID = space.ParentID
	}

	return "", nil
}

// normalizeJourneyFields canonicalizes the journey metadata supplied by the
// client so stored values match the VisitorContext normalization rules.
// Unknown but non-empty values are discarded for country codes and the journey
// stage; visa routes fall back to "other" so the boost logic still works.
func normalizeJourneyFields(journeyStage, visaRoute, sourceCountry, targetCountry **string) {
	if journeyStage != nil && *journeyStage != nil && **journeyStage != "" {
		n := visitor.NormalizeJourneyStage(**journeyStage)
		if n == "" {
			*journeyStage = nil
		} else {
			**journeyStage = n
		}
	}

	if visaRoute != nil && *visaRoute != nil && **visaRoute != "" {
		**visaRoute = visitor.NormalizeVisaRoute(**visaRoute)
	}

	if sourceCountry != nil && *sourceCountry != nil && **sourceCountry != "" {
		n := visitor.NormalizeCountry(**sourceCountry)
		if n == "" {
			*sourceCountry = nil
		} else {
			**sourceCountry = n
		}
	}

	if targetCountry != nil && *targetCountry != nil && **targetCountry != "" {
		n := visitor.NormalizeCountry(**targetCountry)
		if n == "" {
			*targetCountry = nil
		} else {
			**targetCountry = n
		}
	}
}

// contentGeoFromFieldValues extracts source/target country from the content's
// own field_values geo keys (e.g. a visa guide's origin_country /
// destination_country fields, or a banking guide's country field). Values
// may be ISO codes or country names. These are author-provided metadata
// about the post's subject — the author's profile is deliberately NOT a
// source: an author can write about journeys unrelated to their own
// (§14 Global Template Literals / §71).
func contentGeoFromFieldValues(fieldValues []byte) (source, target string) {
	if len(fieldValues) == 0 {
		return "", ""
	}

	var fv map[string]any
	if err := json.Unmarshal(fieldValues, &fv); err != nil {
		return "", ""
	}

	source = literals.ToCountryCode(literals.GeoFieldValue(fv, "source_country", "origin_country"))
	target = literals.ToCountryCode(literals.GeoFieldValue(fv, "target_country", "destination_country", "country"))

	return source, target
}

// syncStoryMeta upserts the story_meta companion row for a story content row
// written through the generic contents pipeline (/create composer, AI tools)
// so the Story Wall and its origin/destination/visa/profession filters keep
// working regardless of the submission surface (§79). Source/target country
// codes or names resolve to the platform's country-level geo spaces; the
// structured fields come from field_values (the story_fields group). The
// wall requires a destination — when none resolves, the meta row is skipped
// until a destination is provided.
func (s *Service) syncStoryMeta(ctx context.Context, q *gen.Queries, contentID pgtype.UUID, source, target *string, fieldValues []byte) {
	if target == nil {
		return
	}

	destCode := literals.ToCountryCode(*target)
	if destCode == "" {
		return
	}

	dest, err := q.GetCountrySpaceIDByGeoCode(ctx,

		&destCode,
	)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("content: story destination space lookup failed", "error", err)
		}

		return
	}

	var origin pgtype.UUID

	originResolved := false

	if source != nil {
		if srcCode := literals.ToCountryCode(*source); srcCode != "" {
			if sp, err := q.GetCountrySpaceIDByGeoCode(ctx,

				&srcCode,
			); err == nil {
				origin = sp
				originResolved = true
			}
		}
	}

	var fv map[string]any
	_ = json.Unmarshal(fieldValues, &fv)

	// str reads a field_values scalar; repeater values join into a single
	// text column (e.g. most_useful_resource). Items may be plain strings or
	// item_fields objects — {name, link} pairs render as "Name — Link" (§79).
	str := func(key string) *string {
		switch v := fv[key].(type) {
		case string:
			v = strings.TrimSpace(v)
			if v == "" {
				return nil
			}

			return &v
		case []any:
			parts := make([]string, 0, len(v))

			for _, item := range v {
				switch it := item.(type) {
				case string:
					if s := strings.TrimSpace(it); s != "" {
						parts = append(parts, s)
					}
				case map[string]any:
					name, _ := it["name"].(string)
					link, _ := it["link"].(string)
					name = strings.TrimSpace(name)
					link = strings.TrimSpace(link)

					switch {
					case name != "" && link != "":
						parts = append(parts, name+" — "+link)
					case name != "":
						parts = append(parts, name)
					case link != "":
						parts = append(parts, link)
					}
				}
			}

			if len(parts) == 0 {
				return nil
			}

			joined := strings.Join(parts, "; ")

			return &joined
		default:
			return nil
		}
	}

	var year *int16

	switch v := fv["relocation_year"].(type) {
	case float64:
		if v > 0 && v <= math.MaxInt16 {
			y := int16(v)
			year = &y
		}
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 16); err == nil && n > 0 {
			y := int16(n)
			year = &y
		}
	}

	// Merge semantics: a field_values key that is present overwrites the
	// column (empty values clear it); a key that is absent keeps the existing
	// value. Without this, any contents update that omits field_values —
	// e.g. PATCH /stories/:id, which delegates to UpdateContent — would wipe
	// every structured story field to NULL.
	mergeStr := func(key string, existing *string) *string {
		if _, present := fv[key]; present {
			return str(key)
		}

		return existing
	}
	mergeYear := func(existing *int16) *int16 {
		if _, present := fv["relocation_year"]; !present {
			return existing
		}

		return year
	}

	existingMeta, metaErr := q.GetStoryMetaByContent(ctx,
		contentID,
	)
	// Origin is optional: when no source country is provided at all, keep the
	// stored origin space instead of wiping it (contents.source_country is
	// only populated for content that carries journey metadata).
	if metaErr == nil && !originResolved {
		origin = existingMeta.OriginSpaceID
	}

	if metaErr == nil {
		err = q.UpdateStoryMetaByContent(ctx, gen.UpdateStoryMetaByContentParams{
			ContentID: contentID,

			OriginSpaceID:      origin,
			DestinationSpaceID: dest,
			RelocationYear:     mergeYear(existingMeta.RelocationYear),
			VisaCategory:       mergeStr("visa_category", existingMeta.VisaCategory),
			Profession:         mergeStr("profession", existingMeta.Profession),
			BiggestChallenge:   mergeStr("biggest_challenge", existingMeta.BiggestChallenge),
			MostUsefulResource: mergeStr("most_useful_resource", existingMeta.MostUsefulResource),
			BestAdvice:         mergeStr("best_advice", existingMeta.BestAdvice),
		})
	} else if errors.Is(metaErr, pgx.ErrNoRows) {
		err = q.InsertStoryMeta(ctx, gen.InsertStoryMetaParams{
			ContentID:          contentID,
			OriginSpaceID:      origin,
			DestinationSpaceID: dest,
			RelocationYear:     year,
			VisaCategory:       str("visa_category"),
			Profession:         str("profession"),
			BiggestChallenge:   str("biggest_challenge"),
			MostUsefulResource: str("most_useful_resource"),
			BestAdvice:         str("best_advice"),
		})
	}

	if err != nil {
		slog.Warn("content: story meta sync failed", "content_id", contentID, "error", err)
		return
	}

	// §79: published stories federate into the destination country Space
	// feed via content_spaces. A story created through the composer keeps
	// the author's picked primary space, but the destination must still be
	// present as an additional assignment or the destination feed misses it.
	// When the destination changed, the old federation row is dropped so the
	// story leaves the stale feed. A user's explicit primary assignment is
	// never touched.
	spaces, spErr := q.GetContentSpaces(ctx, contentID)
	if spErr != nil {
		slog.Warn("content: story destination federation lookup failed", "content_id", contentID, "error", spErr)
		return
	}

	destPresent := false

	var stale pgtype.UUID

	for _, sp := range spaces {
		if isSameUUID(sp.SpaceID, dest) {
			destPresent = true
			break
		}
	}

	if metaErr == nil && existingMeta.DestinationSpaceID.Valid && !isSameUUID(existingMeta.DestinationSpaceID, dest) {
		// Only remove the old destination when it exists as a non-primary
		// federation row — if the author explicitly filed the story there,
		// that is their choice, not our stale assignment.
		for _, sp := range spaces {
			if isSameUUID(sp.SpaceID, existingMeta.DestinationSpaceID) && !sp.IsPrimary {
				stale = existingMeta.DestinationSpaceID
			}
		}
	}

	// Remove the stale federation row first (delete-all + rebuild minus the
	// stale space, since content_spaces has no single-row delete query),
	// then add the new destination — rebuilding from the snapshot taken
	// before the add keeps the ordering correct.
	if stale.Valid {
		if err := q.DeleteContentSpaces(ctx, contentID); err == nil {
			for _, sp := range spaces {
				if isSameUUID(sp.SpaceID, stale) {
					continue
				}

				if err := q.AddContentSpace(ctx, gen.AddContentSpaceParams{
					ContentID: contentID,
					SpaceID:   sp.SpaceID,
					IsPrimary: sp.IsPrimary,
				}); err != nil {
					slog.Warn("content: story federation rebuild failed", "content_id", contentID, "error", err)
					break
				}
			}
		} else {
			slog.Warn("content: stale story federation removal failed", "content_id", contentID, "error", err)
		}
	}

	if !destPresent {
		if err := q.AddContentSpace(ctx, gen.AddContentSpaceParams{
			ContentID: contentID,
			SpaceID:   dest,
			IsPrimary: false,
		}); err != nil {
			slog.Warn("content: story destination federation failed", "content_id", contentID, "error", err)
		}
	}
}

// inferJourneyMetadataForCreate fills in source/target country and journey
// stage for newly created content when the caller left them unset. It uses
// only the content's own geo signals: explicit values, then field_values
// country keys, then the primary space's geo_code (which itself may have
// been auto-assigned from the title's country/state/city keywords via
// FindSpacesByKeywords). The author's profile is never used.
func (s *Service) inferJourneyMetadataForCreate(ctx context.Context, q *gen.Queries, input *CreateContentInput) {
	normalizeJourneyFields(&input.JourneyStage, &input.VisaRoute, &input.SourceCountry, &input.TargetCountry)

	src, dst := contentGeoFromFieldValues(input.FieldValues)
	if (input.SourceCountry == nil || *input.SourceCountry == "") && src != "" {
		input.SourceCountry = &src
	}

	if (input.TargetCountry == nil || *input.TargetCountry == "") && dst != "" {
		input.TargetCountry = &dst
	}

	// §79: a story's origin/destination are explicit structured fields — the
	// destination drives story_meta (and therefore wall visibility), so it
	// must never be inferred from the posting space. A story filed into an
	// unrelated space would otherwise surface on the wall under the wrong
	// country. Other content types keep the space fallback since their
	// journey fields only feed recommendations.
	if input.ContentType == TypeStory {
		return
	}

	primarySpaceID := input.GeoSpaceID
	if !primarySpaceID.Valid {
		primarySpaceID = input.SpaceID
	}

	if (input.TargetCountry == nil || *input.TargetCountry == "") && primarySpaceID.Valid {
		code, err := resolveSpaceCountryCode(ctx, q, primarySpaceID)
		if err == nil && code != "" {
			input.TargetCountry = &code
		}
	}
}

// CreateContent creates a new content row, ensuring slug uniqueness within
// the platform. If the provided slug is empty, one is derived from the title.
// The content hash and read time are computed automatically from the body.
func (s *Service) CreateContent(ctx context.Context, input CreateContentInput) (gen.CreateContentRow, error) {
	content, err := s.createContent(ctx, s.read, s.write, input)
	if err != nil {
		return content, err
	}

	// §64: record the content.created activity so badge/challenge triggers
	// (e.g. first_post) can fire. Fire-and-forget. The WithQueries variant
	// does not emit — it runs inside a caller's transaction, so the row is
	// not committed yet.
	s.emitActivity(ctx, input.AuthorID, wallet.ActivityKeyContentCreated, "content:"+content.ID.String(), map[string]interface{}{
		"content_type": string(input.ContentType),
	})

	return content, nil
}

// CreateContentWithQueries creates a content row using the provided *gen.Queries
// instance for both reads and writes. This is used by callers that need to
// include content creation inside a larger transaction (e.g. the comparison
// service's CreateComparison method, which must atomically create the content
// row plus its items and dimensions). Ref: §6.2 transaction boundary, BUG-001.
func (s *Service) CreateContentWithQueries(ctx context.Context, q *gen.Queries, input CreateContentInput) (gen.CreateContentRow, error) {
	return s.createContent(ctx, q, q, input)
}

// createContent is the shared implementation behind CreateContent and
// CreateContentWithQueries. It accepts separate read/write queries so the
// non-transactional path can use the reader/writer split while the
// transactional path uses a single queries instance for both.
func (s *Service) createContent(ctx context.Context, readQ, writeQ *gen.Queries, input CreateContentInput) (gen.CreateContentRow, error) {
	slug := input.Slug
	if slug == "" {
		slug = Slugify(input.Title)
	}

	uniqueSlug, err := s.ensureUniqueSlugWithQueries(ctx, readQ, slug)
	if err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("ensuring unique slug: %w", err)
	}

	fieldValues := input.FieldValues
	if fieldValues == nil {
		fieldValues = []byte("{}")
	}

	// Default status and visibility to safe public-draft values. The HTTP
	// handler already does this, but non-HTTP callers (AI agent, imports,
	// bulk creation) can omit the fields and still satisfy the DB CHECK.
	if input.Status == "" {
		input.Status = StatusDraft
	}

	// §79: story submissions always enter the moderation queue. Publish and
	// schedule requests through the generic contents pipeline clamp to
	// in_review — moderator approval paths (SkipStoryModeration) publish
	// directly.
	if input.ContentType == TypeStory && !input.SkipStoryModeration &&
		(input.Status == StatusPublished || input.Status == StatusScheduled) {
		input.Status = StatusInReview
	}

	// content.require_content_review gates every content type behind the
	// in_review moderation queue when enabled (Rule 3). Moderator/admin
	// callers bypass via input.SkipStoryModeration — the HTTP handler sets
	// it on every create, not just stories.
	if !input.SkipStoryModeration &&
		(input.Status == StatusPublished || input.Status == StatusScheduled) &&
		s.requireContentReview(ctx) {
		input.Status = StatusInReview
	}

	if input.Visibility == "" {
		input.Visibility = VisibilityPublic
	}

	// Stamp AI generation time when the caller marks a draft as AI-generated
	// but does not provide the generation timestamp (ADR-088 / §106).
	aiGeneratedAt := input.AIGeneratedAt
	if input.AIGenerated && !aiGeneratedAt.Valid {
		aiGeneratedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}

	// Validate status/visibility for non-HTTP callers (agent tools, imports,
	// domain services like comparison) that bypass handler-level validation.
	// Without this, a bad value surfaces as a raw contents_*_check violation
	// instead of a clean sentinel error.
	if !IsValidStatus(input.Status) {
		return gen.CreateContentRow{}, fmt.Errorf("%w: %q", ErrInvalidStatus, input.Status)
	}

	if !IsValidVisibility(input.Visibility) {
		return gen.CreateContentRow{}, fmt.Errorf("%w: %q", ErrInvalidVisibility, input.Visibility)
	}

	// Render markdown body to HTML when no HTML form is supplied.
	// The frontend detail pages render body_html, so a missing value
	// would show unformatted plain text (Section 14 / §15).
	bodyHTML := SanitizeHTML(input.BodyHtml)
	if bodyHTML == "" && strings.TrimSpace(input.Body) != "" {
		bodyHTML = SanitizeHTML(RenderBodyHTML(input.Body))
	}

	// Compute content hash from body if not provided.
	// Skip hashing for empty bodies — the unique index has
	// `WHERE content_hash IS NULL`, so NULL hashes don't collide.
	// Without this, every empty-body draft gets the same hash and
	// the autosave's first fire (before the user types body content)
	// hits a 409 duplicate_content (Rule 12 / §41).
	//
	// For video posts, append the video_source_id so the same body text
	// with different sources does not collide (§106).
	contentHash := input.ContentHash
	if contentHash == nil && strings.TrimSpace(input.Body) != "" {
		var h string
		if input.ContentType == TypeVideo && input.VideoSourceID.Valid {
			h = ComputeContentHashWithDistinguisher(input.Body, input.VideoSourceID.String())
		} else {
			h = ComputeContentHash(input.Body)
		}

		contentHash = &h
	}

	// Published or scheduled content must have a primary space. A provided
	// SpaceID / GeoSpaceID will be written as the primary content_space
	// immediately after the contents row is inserted (BUG-007).
	if (input.Status == StatusPublished || input.Status == StatusScheduled) &&
		!input.SpaceID.Valid && !input.GeoSpaceID.Valid {
		return gen.CreateContentRow{}, ErrMissingPrimarySpace
	}

	// Post-as-org (ADR-062): attributing to an org requires the author be an
	// owner/admin/editor of it. author_id remains the human, always.
	if input.OrganizationID.Valid {
		role, err := readQ.GetOrganizationMemberRole(ctx, gen.GetOrganizationMemberRoleParams{
			OrganizationID: input.OrganizationID,
			UserID:         input.AuthorID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.CreateContentRow{}, ErrOrgPostForbidden
			}

			return gen.CreateContentRow{}, fmt.Errorf("checking org membership: %w", err)
		}

		if !isOrgPostRole(role) {
			return gen.CreateContentRow{}, ErrOrgPostForbidden
		}
	}

	// Journey-aware recommendations: infer source/target countries from the
	// content's own field_values and the primary space when not explicitly
	// set. The author's profile is never a source — authors can write about
	// destinations unrelated to their own journey.
	s.inferJourneyMetadataForCreate(ctx, readQ, &input)

	// §79: when the caller did not pick a space, file the story into its
	// destination country space (same behaviour as POST /stories).
	if input.ContentType == TypeStory && !input.GeoSpaceID.Valid && !input.SpaceID.Valid {
		if input.TargetCountry != nil {
			if code := literals.ToCountryCode(*input.TargetCountry); code != "" {
				if sp, err := readQ.GetCountrySpaceIDByGeoCode(ctx, &code); err == nil {
					input.GeoSpaceID = sp
				}
			}
		}
	}

	content, err := writeQ.CreateContent(ctx, gen.CreateContentParams{
		AuthorID:        input.AuthorID,
		OrganizationID:  input.OrganizationID,
		SpaceID:         input.SpaceID,
		ContentType:     input.ContentType,
		Title:           input.Title,
		Slug:            uniqueSlug,
		Body:            input.Body,
		BodyHtml:        bodyHTML,
		FieldValues:     fieldValues,
		ContentHash:     contentHash,
		GeoSpaceID:      input.GeoSpaceID,
		HeaderImageID:   input.HeaderImageID,
		FeaturedImageID: input.FeaturedImageID,
		VideoSourceID:   input.VideoSourceID,
		CanonicalUrl:    input.CanonicalUrl,
		MetaTitle:       input.MetaTitle,
		MetaDescription: input.MetaDescription,
		OgImageUrl:      input.OgImageUrl,
		SeoIndexable:    input.SeoIndexable,
		Status:          input.Status,
		Visibility:      input.Visibility,
		ReadTimeMinutes: ComputeReadTime(input.Body),
		PublishAt:       input.PublishAt,
		GuideCategory:   input.GuideCategory,
		JourneyStage:    input.JourneyStage,
		VisaRoute:       input.VisaRoute,
		SourceCountry:   input.SourceCountry,
		TargetCountry:   input.TargetCountry,
		AiGenerated:     input.AIGenerated,
		AiModel:         input.AIModel,
		AiGeneratedAt:   aiGeneratedAt,
		IntelSourceID:   input.IntelSourceID,
		IntelRecordID:   input.IntelRecordID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.CreateContentRow{}, ErrDuplicateContent
		}

		return gen.CreateContentRow{}, fmt.Errorf("creating content: %w", err)
	}

	// Create the initial content_space row for the primary space (Section 14).
	// This keeps content_spaces authoritative and ensures list-by-space and
	// fan-out queries work regardless of the legacy contents.space_id column.
	if input.SpaceID.Valid || input.GeoSpaceID.Valid {
		primarySpaceID := input.SpaceID
		if !primarySpaceID.Valid {
			primarySpaceID = input.GeoSpaceID
		}

		if err := writeQ.AddContentSpace(ctx, gen.AddContentSpaceParams{
			ContentID: content.ID,
			SpaceID:   primarySpaceID,
			IsPrimary: true,
		}); err != nil {
			return gen.CreateContentRow{}, fmt.Errorf("adding primary content_space: %w", err)
		}

		if err := writeQ.SetContentSpaceID(ctx, gen.SetContentSpaceIDParams{
			ContentID: content.ID,
			SpaceID:   primarySpaceID,
		}); err != nil {
			return gen.CreateContentRow{}, fmt.Errorf("setting primary content space: %w", err)
		}
	}

	// Create the initial revision.
	if _, err := writeQ.CreateContentRevision(ctx, gen.CreateContentRevisionParams{
		ContentID:   content.ID,
		FieldValues: fieldValues,
		Title:       input.Title,
		Body:        input.Body,
		BodyHtml:    input.BodyHtml,
		EditedBy:    input.AuthorID,
	}); err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("creating initial revision: %w", err)
	}

	// §79: keep the story_meta companion row in sync for stories written
	// through the generic contents pipeline.
	if input.ContentType == TypeStory {
		s.syncStoryMeta(ctx, writeQ, content.ID, input.SourceCountry, input.TargetCountry, fieldValues)
	}

	s.emitWebhook(ctx, "content.created", map[string]any{
		"content_id": content.ID,
		"slug":       content.Slug,
		"title":      content.Title,
		"author_id":  content.AuthorID,
	})

	// When content is created directly as published (bypassing the dedicated
	// POST /content/{id}/publish flow), also emit the content.published
	// webhook — subscribers to content.published (e.g. RSS, notifications)
	// would otherwise miss this content. The content_scores row and affiliate
	// scan are handled by the CreateContent handler (§32, §55).
	if content.Status == StatusPublished {
		s.emitWebhook(ctx, "content.published", map[string]any{
			"content_id": content.ID,
			"slug":       content.Slug,
			"title":      content.Title,
			"actor_id":   content.AuthorID,
		})
		s.purgeEdgeContent(ctx, content.ID, content.ContentType, content.Slug)
	}

	return content, nil
}

// GetContent returns the content with the given ID, scoped to the platform.
func (s *Service) GetContent(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error) {
	content, err := s.read.GetContentByID(ctx,
		id,
	)
	if err != nil {
		return gen.GetContentByIDRow{}, fmt.Errorf("getting content: %w", err)
	}

	return content, nil
}

// GetContentBySlug returns the content matching the given slug, scoped to
// the platform.
func (s *Service) GetContentBySlug(ctx context.Context, slug string) (gen.GetContentBySlugRow, error) {
	content, err := s.read.GetContentBySlug(ctx,
		slug,
	)
	if err != nil {
		return gen.GetContentBySlugRow{}, fmt.Errorf("getting content by slug: %w", err)
	}

	return content, nil
}

// GetContentDetailBySlug returns the enriched single-content row used by the
// public /content/{slug} reading page: author name/username/avatar, space
// name/slug, tag slugs, featured + header image storage refs, and blog_series
// navigation. Mirrors the blog package's GetBlogPostDetail for non-blog
// content types (guide, page, comparison, discussion, …).
func (s *Service) GetContentDetailBySlug(ctx context.Context, slug string) (gen.GetContentDetailBySlugRow, error) {
	content, err := s.read.GetContentDetailBySlug(ctx,
		slug,
	)
	if err != nil {
		return gen.GetContentDetailBySlugRow{}, fmt.Errorf("getting content detail by slug: %w", err)
	}

	return content, nil
}

// GetContentDetailBySlugAndType returns the enriched single-content row for the
// public /post/{content_type}/{slug} social page. Disambiguates by content_type.
func (s *Service) GetContentDetailBySlugAndType(ctx context.Context, contentType, slug string) (gen.GetContentDetailBySlugAndTypeRow, error) {
	content, err := s.read.GetContentDetailBySlugAndType(ctx, gen.GetContentDetailBySlugAndTypeParams{
		ContentType: contentType,
		Slug:        slug,
	})
	if err != nil {
		return gen.GetContentDetailBySlugAndTypeRow{}, fmt.Errorf("getting content detail by slug and type: %w", err)
	}

	return content, nil
}

// ListContents returns a page of content for the given platform, newest first.
// Pass a zero-value CursorCreatedAt for the first page. When q is non-empty,
// results are filtered by case-insensitive match on title, slug, or body.
// Visibility is gated by the supplied filter.
func (s *Service) ListContents(ctx context.Context, q string, cursor pgtype.Timestamptz, limit int32, filter visibility.VisibilityFilter) ([]gen.ListContentsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	contents, err := s.read.ListContents(ctx, gen.ListContentsParams{
		Q:                 q,
		CursorCreatedAt:   cursor,
		PageLimit:         limit,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("listing contents: %w", err)
	}

	return contents, nil
}

// SearchContents uses Meilisearch to find content matching the query, then
// fetches the full rows (with JOINed author/space names) from PostgreSQL by ID.
// Returns the same column set as ListContents but with Meilisearch-grade
// full-text search (typo tolerance, relevance ranking) instead of ILIKE.
// Call this when a ContentSearcher is wired and q is non-empty.
func (s *Service) SearchContents(ctx context.Context, q string, limit int32, filter visibility.VisibilityFilter) ([]gen.ListContentsByIDsRow, error) {
	if s.searcher == nil {
		return nil, errors.New("no content searcher configured")
	}

	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	ids, err := s.searcher.SearchContentIDs(ctx, q, int(limit))
	if err != nil {
		return nil, fmt.Errorf("searching content via Meilisearch: %w", err)
	}

	if len(ids) == 0 {
		return []gen.ListContentsByIDsRow{}, nil
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	return s.read.ListContentsByIDs(ctx, gen.ListContentsByIDsParams{
		Ids:               ids,
		PageLimit:         limit,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
}

// ListFeaturedContentsByType returns rows from the generic content_featured
// junction (Section 14 — generic featured list powering the Featured tabs
// on /guides, /stories, /forum). contentType ” means all types.
func (s *Service) ListFeaturedContentsByType(ctx context.Context, contentType string, offset, limit int32, filter visibility.VisibilityFilter) ([]gen.ListFeaturedContentsByTypeRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	rows, err := s.read.ListFeaturedContentsByType(ctx, gen.ListFeaturedContentsByTypeParams{
		ContentType:       contentType,
		PageOffset:        offset,
		PageLimit:         limit,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("listing featured contents: %w", err)
	}

	return rows, nil
}

// FeatureContent adds a content row to the shared content_featured junction
// (Section 14 — generic featuring; gated by the content.feature node).
func (s *Service) FeatureContent(ctx context.Context, contentID pgtype.UUID, order int32, until pgtype.Timestamptz) (gen.ContentFeatured, error) {
	return s.write.FeatureContent(ctx, gen.FeatureContentParams{
		ContentID:     contentID,
		FeaturedOrder: order,
		FeaturedUntil: until,
	})
}

// UnfeatureContent removes a content row from content_featured.
func (s *Service) UnfeatureContent(ctx context.Context, contentID pgtype.UUID) error {
	return s.write.UnfeatureContent(ctx,

		contentID,
	)
}

// ListContentsByType returns a page of content filtered by content_type and
// optional guide_category. Composite cursor (created_at, id) prevents pages
// with identical timestamps from being skipped. sort='popular' orders by
// view_count and paginates via offset (cursor pagination only applies to the
// default created_at order); sort='roadmap' ranks by the visitor's journey
// signals (stage > destination > visa > origin) then canonical §87 stage
// order — empty signals degrade to plain stage order.
func (s *Service) ListContentsByType(ctx context.Context, contentType, guideCategory string, cursor pgtype.Timestamptz, cursorID pgtype.UUID, offset, limit int32, sort string, vctx visitor.VisitorContext, filter visibility.VisibilityFilter) ([]gen.ListContentsByTypeRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	contents, err := s.read.ListContentsByType(ctx, gen.ListContentsByTypeParams{
		ContentType:       contentType,
		GuideCategory:     guideCategory,
		CursorCreatedAt:   cursor,
		CursorID:          cursorID,
		Sort:              sort,
		VStage:            vctx.JourneyStage,
		VDest:             vctx.DestinationCountry,
		VVisa:             vctx.VisaRoute,
		VOrigin:           vctx.OriginCountry,
		PageOffset:        offset,
		PageLimit:         limit,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("listing contents by type: %w", err)
	}

	return contents, nil
}

// ListContentsBySpace returns a page of content filtered by space_id.
// Visibility is gated by the supplied filter.
func (s *Service) ListContentsBySpace(ctx context.Context, spaceID pgtype.UUID, cursor pgtype.Timestamptz, limit int32, filter visibility.VisibilityFilter) ([]gen.ListContentsBySpaceRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	contents, err := s.read.ListContentsBySpace(ctx, gen.ListContentsBySpaceParams{
		SpaceID:           spaceID,
		CursorCreatedAt:   cursor,
		PageLimit:         limit,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("listing contents by space: %w", err)
	}

	return contents, nil
}

// SearchContentsBySpace returns content within a space whose title matches the
// query (case-insensitive substring). Used by the space search tab
// (GET /spaces/{slug}/content?q=...). Offset-paginated.
// Visibility is gated by the supplied filter.
func (s *Service) SearchContentsBySpace(ctx context.Context, spaceID pgtype.UUID, query string, limit, offset int32, filter visibility.VisibilityFilter) ([]gen.SearchContentsBySpaceRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	contents, err := s.read.SearchContentsBySpace(ctx, gen.SearchContentsBySpaceParams{
		SpaceID:           spaceID,
		Query:             &query,
		PageLimit:         limit,
		PageOffset:        offset,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("searching contents by space: %w", err)
	}

	return contents, nil
}

// SearchContentsByTypeAndSpace returns content of a specific type within a
// space whose title matches the query. Used by the JCE picker when a content
// leaf is scoped to a geo or entity space. Offset-paginated.
func (s *Service) SearchContentsByTypeAndSpace(ctx context.Context, spaceID pgtype.UUID, contentType, query string, limit, offset int32, filter visibility.VisibilityFilter) ([]gen.SearchContentsByTypeAndSpaceRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	contents, err := s.read.SearchContentsByTypeAndSpace(ctx, gen.SearchContentsByTypeAndSpaceParams{
		SpaceID:           spaceID,
		ContentType:       contentType,
		Query:             &query,
		PageLimit:         limit,
		PageOffset:        offset,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("searching contents by type and space: %w", err)
	}

	return contents, nil
}

// UpdateContentInput holds the parameters for updating an existing content row.
type UpdateContentInput struct {
	Title           string
	Slug            string
	Body            string
	BodyHtml        string //nolint:revive // matches sqlc-generated field name
	FieldValues     []byte
	ContentHash     *string
	SpaceID         pgtype.UUID
	GeoSpaceID      pgtype.UUID
	HeaderImageID   pgtype.UUID
	FeaturedImageID pgtype.UUID
	VideoSourceID   pgtype.UUID
	CanonicalUrl    *string //nolint:revive // matches sqlc-generated field name
	MetaTitle       *string
	MetaDescription *string
	OgImageUrl      *string //nolint:revive // matches sqlc-generated field name
	SeoIndexable    *bool
	Status          string
	Visibility      string
	PublishAt       pgtype.Timestamptz
	// ClearPublishAt NULLs publish_at. An invalid PublishAt means "keep the
	// stored schedule" — publish_at is otherwise managed by ScheduleContent
	// and the PATCH publish_at field.
	ClearPublishAt bool
	// IsLocked/IsPinned are *bool so a PATCH that omits them preserves the
	// stored flags; non-nil writes the value (including explicit false).
	IsLocked       *bool
	IsPinned       *bool
	OrganizationID pgtype.UUID
	// ClearOrganizationID NULLs organization_id ("post as yourself"). An
	// invalid UUID alone now means "keep the stored attribution".
	ClearOrganizationID bool
	// ClearFeaturedImageID/ClearHeaderImageID NULL the image columns when
	// set. An invalid UUID alone now means "keep the stored image".
	ClearFeaturedImageID bool
	ClearHeaderImageID   bool
	// ExpectedUpdatedAt is the optimistic-lock precondition (U1): when valid,
	// the update only proceeds if the stored updated_at still equals it. A
	// mismatch returns ErrStaleWrite so the composer can reload-merge instead
	// of silently overwriting a concurrent edit.
	ExpectedUpdatedAt pgtype.Timestamptz
	// ClearCanonicalUrl/ClearMetaTitle/ClearMetaDescription/ClearOgImageUrl
	// write NULL to the nullable SEO columns. A nil *string alone now means
	// "keep the stored value" — these flags restore the explicit JSON null
	// clear semantics.
	ClearCanonicalUrl    bool
	ClearMetaTitle       bool
	ClearMetaDescription bool
	ClearOgImageUrl      bool
	GuideCategory        *string
	AIGeneratedAt        pgtype.Timestamptz
	AIModel              *string
	// BypassSpaceCheck lets admins update the primary space without the
	// per-space membership/permission gate.
	BypassSpaceCheck bool
	// SkipStoryModeration lets moderator callers publish/schedule stories
	// directly instead of re-entering the §79 in_review queue.
	SkipStoryModeration bool
	// Journey-aware recommendation metadata.
	JourneyStage  *string
	VisaRoute     *string
	SourceCountry *string
	TargetCountry *string
	// Source provenance for promoted/ingested content (v1 sources directory).
	IntelSourceID pgtype.UUID
	IntelRecordID pgtype.UUID
}

// inferJourneyMetadataForUpdate fills in source/target country for an existing
// content row when the update leaves them unset. It uses only the content's
// own geo signals: field_values country keys (the caller's payload when sent,
// else the stored values), then the primary space's geo_code. The author's
// profile is never used — an author can write about other destinations.
func (s *Service) inferJourneyMetadataForUpdate(ctx context.Context, q *gen.Queries, existing gen.GetContentByIDRow, input *UpdateContentInput) {
	normalizeJourneyFields(&input.JourneyStage, &input.VisaRoute, &input.SourceCountry, &input.TargetCountry)

	fieldValues := input.FieldValues
	if len(fieldValues) == 0 {
		fieldValues = existing.FieldValues
	}

	src, dst := contentGeoFromFieldValues(fieldValues)
	if (input.SourceCountry == nil || *input.SourceCountry == "") && src != "" {
		input.SourceCountry = &src
	}

	if (input.TargetCountry == nil || *input.TargetCountry == "") && dst != "" {
		input.TargetCountry = &dst
	}

	// §79: a story's origin/destination are explicit structured fields —
	// never infer the destination from the posting space, or a story filed
	// into an unrelated space surfaces on the wall under the wrong country.
	if existing.ContentType == TypeStory {
		return
	}

	primarySpaceID := input.GeoSpaceID
	if !primarySpaceID.Valid {
		primarySpaceID = input.SpaceID
	}

	if !primarySpaceID.Valid {
		primarySpaceID = existing.GeoSpaceID
	}

	if !primarySpaceID.Valid {
		primarySpaceID = existing.SpaceID
	}

	if (input.TargetCountry == nil || *input.TargetCountry == "") && primarySpaceID.Valid {
		code, err := resolveSpaceCountryCode(ctx, q, primarySpaceID)
		if err == nil && code != "" {
			input.TargetCountry = &code
		}
	}
}

// UpdateContent updates an existing content row, recording a revision and
// tracking slug changes in slug_history. The old slug is preserved for
// 301 redirects.
func (s *Service) UpdateContent(ctx context.Context, id, editedBy pgtype.UUID, input UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error) {
	existing, err := s.read.GetContentByID(ctx,
		id,
	)
	if err != nil {
		return gen.UpdateContentRow{}, fmt.Errorf("getting existing content for update: %w", err)
	}

	// U1 optimistic lock: when the caller supplies expected_updated_at,
	// reject the write if the row moved since the editor loaded it.
	if input.ExpectedUpdatedAt.Valid && existing.UpdatedAt.Valid &&
		!input.ExpectedUpdatedAt.Time.Equal(existing.UpdatedAt.Time) {
		return gen.UpdateContentRow{}, errpkg.ErrStaleWrite
	}

	// Edit-window enforcement (BL-008 / Section 49): the original author may
	// only edit within `content.edit_window_minutes` of creation. Moderators
	// and admins editing someone else's post bypass this check because they
	// are not the author. Super admins bypass even on their own content.
	// A zero/missing created_at is treated as "no limit".
	if !bypassEditWindow && existing.AuthorID.Valid && existing.CreatedAt.Valid &&
		existing.AuthorID.Bytes == editedBy.Bytes {
		windowMinutes := defaultEditWindowMinutes
		if s.settings != nil {
			windowMinutes = s.settings.GetIntOr(ctx, "content.edit_window_minutes", defaultEditWindowMinutes)
		}

		if windowMinutes > 0 {
			deadline := existing.CreatedAt.Time.Add(time.Duration(windowMinutes) * time.Minute)
			if time.Now().After(deadline) {
				return gen.UpdateContentRow{}, ErrEditWindowExpired
			}
		}
	}

	// Preserve omitted top-level scalar fields; an empty value means "no
	// change" for partial updates from both the REST API and agent tools.
	if input.Title == "" {
		input.Title = existing.Title
	}

	if input.Status == "" {
		input.Status = existing.Status
	}

	// §79: stories always re-enter the moderation queue on publish/schedule —
	// author-facing status writes clamp to in_review (same as create);
	// moderator callers set SkipStoryModeration to publish directly.
	if existing.ContentType == TypeStory && !input.SkipStoryModeration &&
		(input.Status == StatusPublished || input.Status == StatusScheduled) {
		input.Status = StatusInReview
	}

	// content.require_content_review gates every content type behind the
	// in_review moderation queue when enabled (Rule 3), same as create.
	if !input.SkipStoryModeration &&
		(input.Status == StatusPublished || input.Status == StatusScheduled) &&
		s.requireContentReview(ctx) {
		input.Status = StatusInReview
	}

	if input.Visibility == "" {
		input.Visibility = existing.Visibility
	}

	// Same guard as createContent: reject invalid status/visibility from
	// non-HTTP callers before the UPDATE hits the DB CHECK constraint.
	if !IsValidStatus(input.Status) {
		return gen.UpdateContentRow{}, fmt.Errorf("%w: %q", ErrInvalidStatus, input.Status)
	}

	if !IsValidVisibility(input.Visibility) {
		return gen.UpdateContentRow{}, fmt.Errorf("%w: %q", ErrInvalidVisibility, input.Visibility)
	}

	newSlug := input.Slug
	if newSlug == "" {
		newSlug = existing.Slug
	} else if newSlug != existing.Slug {
		uniqueSlug, err := s.EnsureUniqueSlug(ctx, newSlug)
		if err != nil {
			return gen.UpdateContentRow{}, fmt.Errorf("ensuring unique slug on update: %w", err)
		}

		newSlug = uniqueSlug
	}

	// Preserve body, body_html, and content_hash on metadata-only updates
	// where the client omits those fields (BUG-004). The SQL UPDATE uses
	// COALESCE for content_hash; this service-level fallback keeps body/body_html
	// from being overwritten when both are empty.
	if input.Body == "" && input.BodyHtml == "" {
		input.Body = existing.Body
		input.BodyHtml = existing.BodyHtml

		if input.ContentHash == nil && existing.ContentHash != nil && *existing.ContentHash != "" {
			existingHash := *existing.ContentHash
			input.ContentHash = &existingHash
		}
	}

	// Render markdown body to HTML when no HTML form is supplied. body_html is
	// the canonical rendered form used by detail pages (Section 14 / §15).
	if input.BodyHtml == "" && strings.TrimSpace(input.Body) != "" {
		input.BodyHtml = SanitizeHTML(RenderBodyHTML(input.Body))
	}

	// Post-as-org (ADR-062): attributing an org on this post requires the
	// editor be an owner/admin/editor of that org. Symmetric with CreateContent.
	// The guard only runs when the input sets an org — OrganizationID persists
	// when omitted, and removal is an explicit org-admin action (or an
	// explicit organization_id null clear), not a side effect of edits.
	if input.OrganizationID.Valid {
		role, err := s.read.GetOrganizationMemberRole(ctx, gen.GetOrganizationMemberRoleParams{
			OrganizationID: input.OrganizationID,
			UserID:         editedBy,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.UpdateContentRow{}, ErrOrgPostForbidden
			}

			return gen.UpdateContentRow{}, fmt.Errorf("checking org membership on update: %w", err)
		}

		if !isOrgPostRole(role) {
			return gen.UpdateContentRow{}, ErrOrgPostForbidden
		}
	}

	// Duplicate-content pre-check (Rule 12 / BL-006): if a content_hash is
	// supplied, verify no other row in this platform already has it. The unique
	// index idx_contents_platform_content_hash is the ultimate guard, but this
	// yields a clean 409 duplicate_content instead of a raw 23505 violation.
	if input.ContentHash != nil && *input.ContentHash != "" {
		if _, err := s.read.GetContentByHash(ctx, gen.GetContentByHashParams{
			ContentHash: input.ContentHash,
			ExcludeID:   id,
		}); err == nil {
			return gen.UpdateContentRow{}, ErrDuplicateContent
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return gen.UpdateContentRow{}, fmt.Errorf("checking content hash uniqueness: %w", err)
		}
	}

	// Determine the primary space for this update. Prefer an explicit new
	// space_id, then an explicit geo_space_id, then the existing values.
	primarySpaceID := input.SpaceID
	if !primarySpaceID.Valid {
		primarySpaceID = input.GeoSpaceID
	}

	if !primarySpaceID.Valid {
		primarySpaceID = existing.GeoSpaceID
	}

	if !primarySpaceID.Valid {
		primarySpaceID = existing.SpaceID
	}

	// Published/scheduled content must have a primary space. If the caller
	// is setting the primary space in this same update, the sync below will
	// create it; otherwise we must ensure one already exists (BUG-007).
	if input.Status == StatusPublished || input.Status == StatusScheduled {
		if !primarySpaceID.Valid {
			if err := hasPrimarySpace(ctx, s.read, id); err != nil {
				return gen.UpdateContentRow{}, err
			}
		}
	}

	// Journey-aware recommendations: infer source/target country for this
	// content when the update leaves them unset.
	s.inferJourneyMetadataForUpdate(ctx, s.read, existing, &input)

	params := buildUpdateContentParams(id, input, existing, newSlug, primarySpaceID)

	content, err := s.write.UpdateContent(ctx, params)
	if err != nil {
		return gen.UpdateContentRow{}, fmt.Errorf("updating content: %w", err)
	}

	// Synchronize content_spaces when the primary space changes, preserving
	// any additional space assignments. This keeps content_spaces
	// authoritative and the legacy contents.space_id / geo_space_id in sync.
	if input.SpaceID.Valid || input.GeoSpaceID.Valid {
		existingSpaces, err := s.read.GetContentSpaces(ctx, content.ID)
		if err != nil {
			return gen.UpdateContentRow{}, fmt.Errorf("getting content spaces for update: %w", err)
		}

		additional := make([]pgtype.UUID, 0, len(existingSpaces))

		for _, sp := range existingSpaces {
			if !isSameUUID(sp.SpaceID, primarySpaceID) {
				additional = append(additional, sp.SpaceID)
			}
		}

		if err := s.UpdateContentSpaces(ctx, content.ID, editedBy, primarySpaceID, additional, input.BypassSpaceCheck); err != nil {
			return gen.UpdateContentRow{}, fmt.Errorf("updating content spaces: %w", err)
		}
	}

	// §79: keep story_meta in sync after story updates through the generic
	// contents pipeline. Journey pointers fall back to the stored values —
	// the UPDATE uses COALESCE, so nil means "unchanged".
	if existing.ContentType == TypeStory {
		src := input.SourceCountry
		if src == nil {
			src = existing.SourceCountry
		}

		dst := input.TargetCountry
		if dst == nil {
			dst = existing.TargetCountry
		}

		s.syncStoryMeta(ctx, s.write, content.ID, src, dst, params.FieldValues)
	}

	// Record slug change for 301 redirect.
	s.RecordSlugChange(ctx, EntityContent, id, existing.Slug, newSlug)

	// Create a revision.
	if _, err := s.write.CreateContentRevision(ctx, gen.CreateContentRevisionParams{
		ContentID:   id,
		FieldValues: params.FieldValues,
		Title:       input.Title,
		Body:        input.Body,
		BodyHtml:    input.BodyHtml,
		EditedBy:    editedBy,
	}); err != nil {
		return gen.UpdateContentRow{}, fmt.Errorf("creating revision: %w", err)
	}

	s.emitWebhook(ctx, "content.updated", map[string]any{
		"content_id": content.ID,
		"slug":       content.Slug,
		"title":      content.Title,
		"actor_id":   editedBy,
	})

	// Purge both slugs — a rename leaves the old URL cached at the edge.
	// Only publicly served revisions can sit in the edge cache; edits to
	// drafts/members-only content would burn CDN purge quota on pages the
	// edge never cached (ADR-134).
	wasPublic := existing.Status == StatusPublished && existing.Visibility == VisibilityPublic
	isPublic := content.Status == StatusPublished && content.Visibility == VisibilityPublic
	if wasPublic || isPublic {
		s.purgeEdgeContent(ctx, content.ID, content.ContentType, existing.Slug, content.Slug)
	}

	return content, nil
}

// buildUpdateContentParams maps an UpdateContentInput onto the generated
// UPDATE params. The UpdateContent query writes most columns unconditionally
// via sqlc.narg (not COALESCE), so every field the caller did not explicitly
// provide is seeded from the existing row — an omitted field must preserve
// the stored value, not NULL it.
//
// Rules:
//   - published_at is never writable here; it is stamped by
//     UpdateContentStatus, so the stored value always passes through.
//   - organization_id: an invalid input keeps the stored attribution unless
//     ClearOrganizationID requests an explicit NULL ("post as yourself").
//     The org role-guard only runs when setting an org.
//   - *string fields (canonical_url, meta_title, meta_description,
//     og_image_url): nil keeps the stored value, non-nil writes as-is, and
//     the matching Clear* flag writes an explicit NULL (JSON null clear).
//   - publish_at, header_image_id, featured_image_id: invalid keeps the
//     stored value unless the matching Clear* flag requests an explicit NULL.
//   - is_locked / is_pinned: nil keeps the stored flag; non-nil writes the
//     pointed-at value so callers can both set and unset.
func buildUpdateContentParams(id pgtype.UUID, input UpdateContentInput, existing gen.GetContentByIDRow, newSlug string, primarySpaceID pgtype.UUID) gen.UpdateContentParams {
	params := gen.UpdateContentParams{
		ID:              id,
		Title:           input.Title,
		Slug:            newSlug,
		Body:            input.Body,
		BodyHtml:        input.BodyHtml,
		ContentHash:     input.ContentHash,
		GeoSpaceID:      primarySpaceID,
		VideoSourceID:   input.VideoSourceID,
		SeoIndexable:    input.SeoIndexable,
		Status:          input.Status,
		Visibility:      input.Visibility,
		ReadTimeMinutes: ComputeReadTime(input.Body),
		GuideCategory:   input.GuideCategory,
		AiGeneratedAt:   input.AIGeneratedAt,
		AiModel:         input.AIModel,
		JourneyStage:    input.JourneyStage,
		VisaRoute:       input.VisaRoute,
		SourceCountry:   input.SourceCountry,
		TargetCountry:   input.TargetCountry,
		IntelSourceID:   input.IntelSourceID,
		IntelRecordID:   input.IntelRecordID,

		// Preserve-on-omit seeds.
		FieldValues:     input.FieldValues,
		OrganizationID:  input.OrganizationID,
		CanonicalUrl:    input.CanonicalUrl,
		MetaTitle:       input.MetaTitle,
		MetaDescription: input.MetaDescription,
		OgImageUrl:      input.OgImageUrl,
		PublishAt:       input.PublishAt,
		PublishedAt:     existing.PublishedAt,
		HeaderImageID:   input.HeaderImageID,
		FeaturedImageID: input.FeaturedImageID,
		IsLocked:        existing.IsLocked,
		IsPinned:        existing.IsPinned,
	}

	if params.FieldValues == nil {
		params.FieldValues = existing.FieldValues
	}

	if !input.OrganizationID.Valid {
		params.OrganizationID = existing.OrganizationID
	}

	if input.ClearOrganizationID && !input.OrganizationID.Valid {
		params.OrganizationID = pgtype.UUID{}
	}

	if input.CanonicalUrl == nil {
		params.CanonicalUrl = existing.CanonicalUrl
	}

	if input.ClearCanonicalUrl && input.CanonicalUrl == nil {
		params.CanonicalUrl = nil
	}

	if input.MetaTitle == nil {
		params.MetaTitle = existing.MetaTitle
	}

	if input.ClearMetaTitle && input.MetaTitle == nil {
		params.MetaTitle = nil
	}

	if input.MetaDescription == nil {
		params.MetaDescription = existing.MetaDescription
	}

	if input.ClearMetaDescription && input.MetaDescription == nil {
		params.MetaDescription = nil
	}

	if input.OgImageUrl == nil {
		params.OgImageUrl = existing.OgImageUrl
	}

	if input.ClearOgImageUrl && input.OgImageUrl == nil {
		params.OgImageUrl = nil
	}

	if !input.PublishAt.Valid {
		params.PublishAt = existing.PublishAt
	}

	if input.ClearPublishAt && !input.PublishAt.Valid {
		params.PublishAt = pgtype.Timestamptz{}
	}

	if !input.HeaderImageID.Valid {
		params.HeaderImageID = existing.HeaderImageID
	}

	if input.ClearHeaderImageID && !input.HeaderImageID.Valid {
		params.HeaderImageID = pgtype.UUID{}
	}

	if !input.FeaturedImageID.Valid {
		params.FeaturedImageID = existing.FeaturedImageID
	}

	if input.ClearFeaturedImageID && !input.FeaturedImageID.Valid {
		params.FeaturedImageID = pgtype.UUID{}
	}

	if input.IsLocked != nil {
		params.IsLocked = *input.IsLocked
	}

	if input.IsPinned != nil {
		params.IsPinned = *input.IsPinned
	}

	return params
}

// ApplyExplicitNullClears marks the Clear* flags on input for each nullable
// PATCH key the raw request body set to a literal JSON null. encoding/json
// decodes null into a nil pointer exactly like an absent key, so handlers
// re-scan the raw object to distinguish "clear this column" from "not sent"
// (preserve-on-omit). Empty-string clears are handled separately at decode
// time and are unaffected.
func ApplyExplicitNullClears(raw map[string]json.RawMessage, input *UpdateContentInput) {
	if isExplicitNull(raw, "featured_image_id") {
		input.ClearFeaturedImageID = true
	}

	if isExplicitNull(raw, "header_image_id") {
		input.ClearHeaderImageID = true
	}

	if isExplicitNull(raw, "organization_id") {
		input.ClearOrganizationID = true
	}

	if isExplicitNull(raw, "publish_at") {
		input.ClearPublishAt = true
	}

	if isExplicitNull(raw, "canonical_url") {
		input.ClearCanonicalUrl = true
	}

	if isExplicitNull(raw, "meta_title") {
		input.ClearMetaTitle = true
	}

	if isExplicitNull(raw, "meta_description") {
		input.ClearMetaDescription = true
	}

	if isExplicitNull(raw, "og_image_url") {
		input.ClearOgImageUrl = true
	}
}

// isExplicitNull reports whether key is present in the raw JSON object with
// a literal null value.
func isExplicitNull(raw map[string]json.RawMessage, key string) bool {
	v, ok := raw[key]
	return ok && strings.TrimSpace(string(v)) == "null"
}

// UpdateContentStatus changes the status of a content row (e.g. draft to
// published). When transitioning to 'published', published_at is stamped
// if it was previously null. Publishing or scheduling requires a primary
// space (Section 14 / BUG-007).
func (s *Service) UpdateContentStatus(ctx context.Context, id pgtype.UUID, status string) (gen.UpdateContentStatusRow, error) {
	if !IsValidStatus(status) {
		return gen.UpdateContentStatusRow{}, fmt.Errorf("%w: %q", ErrInvalidStatus, status)
	}

	_, err := s.read.GetContentByID(ctx, id)
	if err != nil {
		return gen.UpdateContentStatusRow{}, fmt.Errorf("getting content for status change: %w", err)
	}

	if status == StatusPublished || status == StatusScheduled {
		if err := hasPrimarySpace(ctx, s.read, id); err != nil {
			return gen.UpdateContentStatusRow{}, err
		}
	}

	content, err := s.write.UpdateContentStatus(ctx, gen.UpdateContentStatusParams{
		Status: status,
		ID:     id,
	})
	if err != nil {
		return gen.UpdateContentStatusRow{}, fmt.Errorf("updating content status: %w", err)
	}

	// Ensure a content_scores row exists when content is published. The feed
	// queries (explore/top/rising/space/profile) INNER JOIN content_scores, so
	// a missing row excludes the content from those feeds entirely. The row is
	// created with zero defaults; the engagement worker increments it as
	// views/votes/comments/bookmarks arrive. Ref: Section 32, Guide §21.
	if status == StatusPublished {
		if err := s.write.EnsureContentScore(ctx, content.ID); err != nil {
			// Non-fatal: a missing score row degrades ranking but does not
			// break the publish. Log and continue.
			slog.Warn("content: failed to ensure content_scores row on publish",
				"content_id", content.ID.String(), "error", err)
		}

		s.emitWebhook(ctx, "content.published", map[string]any{
			"content_id": content.ID,
			"slug":       content.Slug,
			"title":      content.Title,
			"actor_id":   content.AuthorID,
		})
	}

	// Scoped retrieval (§69 amendment): reconcile RAG state on every status
	// transition, not just the HTTP update route — blog review, agent draft
	// approval, and studio publishing all funnel through here. Leaving
	// 'published' purges chunks; entering it re-indexes (the embed task also
	// refreshes visibility/space/author scope metadata). Fire-and-forget:
	// the retrieval-time EXISTS guard already fails closed on status.
	if status == StatusPublished {
		worker.EnqueueContentEmbedding(ctx, id)
	} else {
		worker.PurgeRAGEntity(ctx, "content", id)
	}

	// Every status transition purges the edge cache — unpublishing must drop
	// the cached copy just as publishing refreshes it (ADR-134).
	s.purgeEdgeContent(ctx, content.ID, content.ContentType, content.Slug)

	return content, nil
}

// ScheduleContent sets status='scheduled' and publish_at for a future publish
// time, without touching title/body/other fields (catalog: POST
// /content/:id/schedule). Callers must also enqueue the scheduled_publish
// Asynq task with ProcessAt=publishAt for the actual publish to happen.
// Scheduled content must have a primary space (Section 14 / BUG-007).
func (s *Service) ScheduleContent(ctx context.Context, id pgtype.UUID, publishAt time.Time) (gen.ScheduleContentRow, error) {
	_, err := s.read.GetContentByID(ctx, id)
	if err != nil {
		return gen.ScheduleContentRow{}, fmt.Errorf("getting content for scheduling: %w", err)
	}

	if err := hasPrimarySpace(ctx, s.read, id); err != nil {
		return gen.ScheduleContentRow{}, err
	}

	content, err := s.write.ScheduleContent(ctx, gen.ScheduleContentParams{
		PublishAt: pgtype.Timestamptz{Time: publishAt, Valid: true},
		ID:        id,
	})
	if err != nil {
		return gen.ScheduleContentRow{}, fmt.Errorf("scheduling content: %w", err)
	}

	return content, nil
}

// IncrementContentView increments the view count of a content row.
func (s *Service) IncrementContentView(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.IncrementContentView(ctx,
		id,
	); err != nil {
		return fmt.Errorf("incrementing content view: %w", err)
	}

	return nil
}

// RecordContentViewWithDetails records a deduplicated row in
// user_content_views with country_code and session_id (Section 14 — view
// tracking), then increments the view count only when the detailed row was
// actually inserted. The user_content_views PK deduplicates per user/session
// per day; ON CONFLICT DO NOTHING prevents duplicates, and the returned
// command tag tells us whether the insert succeeded.
//
// For anonymous users, user_id is invalid and session_id is used for
// deduplication. When a new view is recorded, a corresponding platform event
// (content.viewed) is also written so Section 40 analytics (traffic sources,
// engagement) can source real-time data. The optional referrer is captured
// from the request header/body and stored in the event meta.
func (s *Service) RecordContentViewWithDetails(ctx context.Context, id, userID pgtype.UUID, sessionID, countryCode, referrer *string) error {
	tag, err := s.write.RecordContentView(ctx, gen.RecordContentViewParams{
		ContentID:   id,
		UserID:      userID,
		SessionID:   sessionID,
		CountryCode: countryCode,
	})
	if err != nil {
		return fmt.Errorf("recording content view: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// Duplicate view for this user/session/day; do not increment the count.
		return nil
	}

	if err := s.write.IncrementContentView(ctx,
		id,
	); err != nil {
		return fmt.Errorf("incrementing content view: %w", err)
	}

	meta := map[string]any{}
	if referrer != nil && *referrer != "" {
		meta["referrer"] = *referrer
	}

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling view event meta: %w", err)
	}

	eventType := "content.viewed"
	entityType := "content"

	if _, err := s.write.CreatePlatformEvent(ctx, gen.CreatePlatformEventParams{
		EventType:   eventType,
		UserID:      userID,
		EntityType:  &entityType,
		EntityID:    id,
		Meta:        metaBytes,
		CountryCode: countryCode,
		SessionID:   sessionID,
		IsSynthetic: false,
	}); err != nil {
		return fmt.Errorf("recording content viewed platform event: %w", err)
	}

	return nil
}

// IncrementContentCommentCount increments the denormalized comment_count on a
// content row. Called when a comment is created so feed cards (which read
// contents.comment_count) show accurate counts immediately. Ref: Section 32.
func (s *Service) IncrementContentCommentCount(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.IncrementContentCommentCount(ctx,
		id,
	); err != nil {
		return fmt.Errorf("incrementing content comment count: %w", err)
	}

	return nil
}

// DecrementContentCommentCount decrements the denormalized comment_count on a
// content row (clamped at 0). Called when a comment is removed.
func (s *Service) DecrementContentCommentCount(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DecrementContentCommentCount(ctx,
		id,
	); err != nil {
		return fmt.Errorf("decrementing content comment count: %w", err)
	}

	return nil
}

// EnsureContentScore creates a zero-default content_scores row if one does not
// yet exist. Idempotent (ON CONFLICT DO NOTHING). Required so INNER JOIN feed
// queries do not exclude content and so the engagement worker can increment
// counts in place. Ref: Section 32, Guide §21.
func (s *Service) EnsureContentScore(ctx context.Context, contentID pgtype.UUID) error {
	if err := s.write.EnsureContentScore(ctx, contentID); err != nil {
		return fmt.Errorf("ensuring content score: %w", err)
	}

	return nil
}

// DeleteContent soft-deletes a content row, scoped to the platform.
func (s *Service) DeleteContent(ctx context.Context, id pgtype.UUID) error {
	// Fetch the row before soft-deleting so the webhook payload can carry
	// the slug/title/author (the deleted_at filter would hide it after).
	existing, err := s.read.GetContentByID(ctx,
		id,
	)
	if err != nil {
		// Non-fatal: proceed with the delete even if the pre-read fails
		// (the row may already be gone); the webhook is best-effort.
		slog.Warn("content: pre-delete fetch for webhook payload", "content_id", id.String(), "error", err)
	}

	if err := s.write.DeleteContent(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting content: %w", err)
	}

	s.emitWebhook(ctx, "content.deleted", map[string]any{
		"content_id": id,
		"slug":       existing.Slug,
		"title":      existing.Title,
		"actor_id":   existing.AuthorID,
	})

	s.purgeEdgeContent(ctx, id, existing.ContentType, existing.Slug)

	return nil
}

// HardDeleteContent permanently removes a content row that is already
// soft-deleted (admin ?force=true — gated by admin.hard_delete). Children
// cascade via ON DELETE CASCADE; NO ACTION dependents surface as
// errpkg.ErrInUse. Returns pgx.ErrNoRows when missing and
// errpkg.ErrNotDeleted while live.
func (s *Service) HardDeleteContent(ctx context.Context, id pgtype.UUID) error {
	// Read slug+type before the row disappears so the edge purge can still
	// target the canonical URL — the soft-delete purge ran earlier, but a
	// failed purge gets retried here (ADR-134). GetContentSlugTypeAny sees
	// soft-deleted rows; GetContentByID does not.
	var slug, contentType string
	if st, err := s.read.GetContentSlugTypeAny(ctx, id); err == nil {
		slug = st.Slug
		contentType = st.ContentType
	}

	affected, err := s.write.HardDeleteContent(ctx, id)
	if err != nil {
		if errpkg.IsFKViolation(err) {
			return errpkg.ErrInUse
		}

		return fmt.Errorf("hard deleting content: %w", err)
	}

	if affected == 0 {
		exists, err := s.read.ContentExistsAny(ctx, id)
		if err != nil {
			return fmt.Errorf("check content exists: %w", err)
		}

		if !exists {
			return pgx.ErrNoRows
		}

		return errpkg.ErrNotDeleted
	}

	s.purgeEdgeContent(ctx, id, contentType, slug)

	return nil
}

// ---------------------------------------------------------------------------
// Content Revisions
// ---------------------------------------------------------------------------

// ListContentRevisions returns the full revision history for a content row,
// newest first. Platform-scoped for defense-in-depth (BUG-012).
func (s *Service) ListContentRevisions(ctx context.Context, contentID pgtype.UUID) ([]gen.ContentRevision, error) {
	revisions, err := s.read.ListContentRevisions(ctx,

		contentID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing content revisions: %w", err)
	}

	return revisions, nil
}

// GetContentRevision returns a specific revision by ID. Platform-scoped for
// defense-in-depth (BUG-012).
func (s *Service) GetContentRevision(ctx context.Context, id, contentID pgtype.UUID) (gen.ContentRevision, error) {
	revision, err := s.read.GetContentRevision(ctx, gen.GetContentRevisionParams{
		ID:        id,
		ContentID: contentID,
	})
	if err != nil {
		return gen.ContentRevision{}, fmt.Errorf("getting content revision: %w", err)
	}

	return revision, nil
}

// requireContentReview reports whether the platform forces publish/schedule
// writes through the in_review moderation queue
// (content.require_content_review, Rule 3). Falls back to false when the
// settings reader is unwired.
func (s *Service) requireContentReview(ctx context.Context) bool {
	return s.settings != nil && s.settings.GetBoolOr(ctx, "content.require_content_review", false)
}

// ---------------------------------------------------------------------------
// Space Pinned Posts
// ---------------------------------------------------------------------------

// PinContent pins a content row to a space. The pinnedBy user and position
// are recorded. If the content is already pinned, the unique constraint
// prevents a duplicate.
func (s *Service) PinContent(ctx context.Context, spaceID, contentID, pinnedBy pgtype.UUID, position int32) (gen.SpacePinnedPost, error) {
	// Only content assigned to the space (authoritative content_spaces
	// junction, BUG-010) may be pinned — otherwise a moderator could pin
	// foreign content into the space's pinned tab.
	inSpace, err := s.read.IsContentInSpace(ctx, gen.IsContentInSpaceParams{
		SpaceID:   spaceID,
		ContentID: contentID,
	})
	if err != nil {
		return gen.SpacePinnedPost{}, fmt.Errorf("checking content space membership: %w", err)
	}

	if !inSpace {
		return gen.SpacePinnedPost{}, fmt.Errorf("content_not_in_space: content %s is not assigned to space %s", contentID.String(), spaceID.String())
	}

	maxPinned := 5
	if s.settings != nil {
		maxPinned = s.settings.GetIntOr(ctx, "spaces.max_pinned_posts", 5)
	}

	if maxPinned > 0 {
		existing, err := s.read.ListPinnedContent(ctx, spaceID)
		if err != nil {
			return gen.SpacePinnedPost{}, fmt.Errorf("checking pinned count: %w", err)
		}

		if len(existing) >= maxPinned {
			return gen.SpacePinnedPost{}, fmt.Errorf("max_pinned_posts_reached: space already has %d pinned posts (limit %d)", len(existing), maxPinned)
		}
	}

	pin, err := s.write.PinContentToSpace(ctx, gen.PinContentToSpaceParams{
		SpaceID:   spaceID,
		ContentID: contentID,
		PinnedBy:  pinnedBy,
		Position:  position,
	})
	if err != nil {
		return gen.SpacePinnedPost{}, fmt.Errorf("pinning content to space: %w", err)
	}

	return pin, nil
}

// UnpinContent removes a pin from a space.
func (s *Service) UnpinContent(ctx context.Context, spaceID, contentID pgtype.UUID) error {
	if err := s.write.UnpinContentFromSpace(ctx, gen.UnpinContentFromSpaceParams{
		SpaceID:   spaceID,
		ContentID: contentID,
	}); err != nil {
		return fmt.Errorf("unpinning content from space: %w", err)
	}

	return nil
}

// ListPinnedContent returns all pinned posts for a space, ordered by position.
func (s *Service) ListPinnedContent(ctx context.Context, spaceID pgtype.UUID) ([]gen.SpacePinnedPost, error) {
	pins, err := s.read.ListPinnedContent(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("listing pinned content: %w", err)
	}

	return pins, nil
}

// ListPinnedContentBySpace returns the pinned content rows for a space in pin
// order — the full content payload the space hub's Pinned tab renders, not
// just pin metadata. Visibility is gated by the supplied filter so guests
// never see members/private pins.
func (s *Service) ListPinnedContentBySpace(ctx context.Context, spaceID pgtype.UUID, filter visibility.VisibilityFilter) ([]gen.ListPinnedContentBySpaceRow, error) {
	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	rows, err := s.read.ListPinnedContentBySpace(ctx, gen.ListPinnedContentBySpaceParams{
		SpaceID:           spaceID,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("listing pinned content by space: %w", err)
	}

	return rows, nil
}

// IsContentPinned returns true if the given content is pinned to the space.
func (s *Service) IsContentPinned(ctx context.Context, spaceID, contentID pgtype.UUID) (bool, error) {
	pinned, err := s.read.IsContentPinned(ctx, gen.IsContentPinnedParams{
		SpaceID:   spaceID,
		ContentID: contentID,
	})
	if err != nil {
		return false, fmt.Errorf("checking if content is pinned: %w", err)
	}

	return pinned, nil
}

// ---------------------------------------------------------------------------
// Reactions
// ---------------------------------------------------------------------------

// ListReactionTypes returns all reaction types for a platform, ordered by position.
func (s *Service) ListReactionTypes(ctx context.Context) ([]gen.ReactionType, error) {
	types, err := s.read.ListReactionTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing reaction types: %w", err)
	}

	return types, nil
}

// GetReactionType returns a single reaction type by platform and key.
func (s *Service) GetReactionType(ctx context.Context, key string) (gen.ReactionType, error) {
	rt, err := s.read.GetReactionType(ctx,

		key,
	)
	if err != nil {
		return gen.ReactionType{}, fmt.Errorf("getting reaction type: %w", err)
	}

	return rt, nil
}

// CreateReactionType creates a new reaction type with the given key, emoji,
// label, and position for a specific platform.
func (s *Service) CreateReactionType(ctx context.Context, key, emoji, label string, enabled bool, position int32) (gen.ReactionType, error) {
	rt, err := s.write.CreateReactionType(ctx, gen.CreateReactionTypeParams{
		Key:      key,
		Emoji:    emoji,
		Label:    label,
		Enabled:  enabled,
		Position: position,
	})
	if err != nil {
		return gen.ReactionType{}, fmt.Errorf("creating reaction type: %w", err)
	}

	return rt, nil
}

// UpdateReactionType updates an existing reaction type by platform and key.
func (s *Service) UpdateReactionType(ctx context.Context, key, emoji, label string, enabled bool, position int32) (gen.ReactionType, error) {
	rt, err := s.write.UpdateReactionType(ctx, gen.UpdateReactionTypeParams{
		Emoji:    emoji,
		Label:    label,
		Enabled:  enabled,
		Position: position,

		Key: key,
	})
	if err != nil {
		return gen.ReactionType{}, fmt.Errorf("updating reaction type: %w", err)
	}

	return rt, nil
}

// DeleteReactionType removes a reaction type by key, then deletes the
// reaction rows that referenced it. The reactions_reaction_key_fkey
// constraint was dropped (migration 000954) so achievement reactions can
// use emoji keys — orphan cleanup is now explicit, committed atomically
// with the type delete.
func (s *Service) DeleteReactionType(ctx context.Context, key string) error {
	return db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if err := q.DeleteReactionType(ctx, key); err != nil {
			return fmt.Errorf("deleting reaction type: %w", err)
		}

		if err := q.DeleteReactionsByKey(ctx, key); err != nil {
			return fmt.Errorf("deleting reactions for key: %w", err)
		}

		return nil
	})
}

// AddReaction adds a reaction from a user to an entity. Idempotent — if the
// user already reacted with the same key, nothing changes (ON CONFLICT DO NOTHING).
func (s *Service) AddReaction(ctx context.Context, entityType string, entityID, userID pgtype.UUID, reactionKey string) (gen.Reaction, error) {
	reaction, err := s.write.AddReaction(ctx, gen.AddReactionParams{
		EntityType:  entityType,
		EntityID:    entityID,
		UserID:      userID,
		ReactionKey: reactionKey,
	})
	if err != nil {
		return gen.Reaction{}, fmt.Errorf("adding reaction: %w", err)
	}

	// ADR-136: the content author earns reaction.added points (self-reacts
	// excluded). Monotonic seq references let a remove→re-add cycle award
	// again while replays dedupe (same pattern as comment/forum votes).
	if entityType == "content" {
		if authorID, aerr := s.read.GetContentAuthorID(ctx, entityID); aerr == nil && authorID.Valid && authorID != userID {
			s.emitReactionAward(ctx, entityID, userID, authorID, reactionKey, reaction.CreatedAt.Time.UnixMicro())
		}
	}

	return reaction, nil
}

// reactionRefPrefix is the dedupe namespace for one (entity, reactor, key)
// tuple of reaction point events.
func reactionRefPrefix(entityID, userID pgtype.UUID, reactionKey string) string {
	return "reaction:" + entityID.String() + ":" + userID.String() + ":" + reactionKey + ":"
}

// emitReactionAward emits reaction.added with the reaction row's
// created_at as the transition marker — unique per physical insert and
// replay-safe (re-adds after removal get a fresh row + timestamp).
func (s *Service) emitReactionAward(ctx context.Context, entityID, userID, authorID pgtype.UUID, reactionKey string, createdMicro int64) {
	prefix := reactionRefPrefix(entityID, userID, reactionKey)
	s.emitActivity(ctx, authorID, wallet.ActivityKeyReactionAdded, fmt.Sprintf("%s%d", prefix, createdMicro), nil)
}

// RemoveReaction removes a user's reaction from an entity. Returns whether
// a row was actually deleted so callers only decrement the engagement
// counter on a real removal (E-003).
func (s *Service) RemoveReaction(ctx context.Context, entityType string, entityID, userID pgtype.UUID, reactionKey string) (bool, error) {
	rows, err := s.write.RemoveReaction(ctx, gen.RemoveReactionParams{
		EntityType:  entityType,
		EntityID:    entityID,
		UserID:      userID,
		ReactionKey: reactionKey,
	})
	if err != nil {
		return false, fmt.Errorf("removing reaction: %w", err)
	}

	// ADR-136: removing a reaction reverts its award. undoes targets the
	// most recent reaction.added event for this (entity, reactor, key);
	// processing skips cleanly if it never credited.
	if rows > 0 && entityType == "content" {
		if authorID, aerr := s.read.GetContentAuthorID(ctx, entityID); aerr == nil && authorID.Valid && authorID != userID {
			prefix := reactionRefPrefix(entityID, userID, reactionKey)
			if priorRef, lerr := s.write.GetLatestActivityEventReferenceByPrefixAndKey(ctx,
				gen.GetLatestActivityEventReferenceByPrefixAndKeyParams{ActivityKey: wallet.ActivityKeyReactionAdded, Prefix: prefix + "%"}); lerr == nil {
				s.emitActivity(ctx, authorID, wallet.ActivityKeyReactionReverted, priorRef+":undo",
					map[string]interface{}{"undoes_key": wallet.ActivityKeyReactionAdded, "undoes": priorRef})
			} else if !errors.Is(lerr, pgx.ErrNoRows) {
				slog.Warn("content: prior reaction award lookup failed", "prefix", prefix, "error", lerr)
			}
		}
	}

	return rows > 0, nil
}

// CanReactToContent reports whether the (authenticated) viewer may see —
// and therefore react to — the content. Mirrors the comments visibility
// cascade (C-001): published, not deleted/auto-hidden, live public or
// member space, and the contents.visibility axis (private = author only).
// Reactions on invisible or nonexistent content must not exist (E-002).
func (s *Service) CanReactToContent(ctx context.Context, contentID, userID pgtype.UUID) (bool, error) {
	visible, err := s.read.IsContentVisibleToUser(ctx, gen.IsContentVisibleToUserParams{
		ContentID: contentID,
		UserID:    userID,
	})
	if err != nil {
		return false, fmt.Errorf("checking content visibility for reaction: %w", err)
	}

	return visible, nil
}

// CanViewContentReactions is the guest-aware variant used by the public
// reaction-counts read — 'members' content stays hidden from anonymous
// viewers (same predicate as CanViewComments, §49).
func (s *Service) CanViewContentReactions(ctx context.Context, contentID, viewerID pgtype.UUID, authed bool) (bool, error) {
	visible, err := s.read.CanViewComments(ctx, gen.CanViewCommentsParams{
		ContentID: contentID,
		UserID:    viewerID,
		Authed:    authed,
	})
	if err != nil {
		return false, fmt.Errorf("checking content visibility for reactions: %w", err)
	}

	return visible, nil
}

// ListReactorsByEntity returns the "who reacted" rows (user display info
// per reaction) for the ReactionRail popover.
func (s *Service) ListReactorsByEntity(ctx context.Context, entityType string, entityID pgtype.UUID, reactionKey *string, limit, offset int32) ([]gen.ListReactorsByEntityRow, error) {
	key := ""
	if reactionKey != nil {
		key = *reactionKey
	}
	rows, err := s.read.ListReactorsByEntity(ctx, gen.ListReactorsByEntityParams{
		EntityType:  entityType,
		EntityID:    entityID,
		ReactionKey: key,
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing reactors: %w", err)
	}
	return rows, nil
}

// CountReactionsByEntity returns reaction counts grouped by reaction key.
func (s *Service) CountReactionsByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) ([]gen.CountReactionsByEntityRow, error) {
	counts, err := s.read.CountReactionsByEntity(ctx, gen.CountReactionsByEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("counting reactions by entity: %w", err)
	}

	return counts, nil
}

// GetUserReactions returns all reactions a specific user has on an entity.
func (s *Service) GetUserReactions(ctx context.Context, entityType string, entityID, userID pgtype.UUID) ([]gen.Reaction, error) {
	reactions, err := s.read.GetUserReactions(ctx, gen.GetUserReactionsParams{
		EntityType: entityType,
		EntityID:   entityID,
		UserID:     userID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting user reactions: %w", err)
	}

	return reactions, nil
}

// ViewerContentState is the caller's interaction state for one content item.
type ViewerContentState struct {
	ReactionKeys []string `json:"reaction_keys"`
	Bookmarked   bool     `json:"bookmarked"`
}

// GetViewerContentStates returns the caller's reaction keys + bookmark state
// for a batch of content ids in two queries. Backs GET /me/content-states so
// card lists resolve all per-item viewer state in one round-trip instead of
// firing /reactions/me + /bookmark per card.
func (s *Service) GetViewerContentStates(ctx context.Context, userID pgtype.UUID, ids []pgtype.UUID) (map[string]*ViewerContentState, error) {
	states := make(map[string]*ViewerContentState, len(ids))

	reactionRows, err := s.read.GetUserReactionsForContents(ctx, gen.GetUserReactionsForContentsParams{
		UserID: userID,
		Ids:    ids,
	})
	if err != nil {
		return nil, fmt.Errorf("getting batch user reactions: %w", err)
	}

	for _, row := range reactionRows {
		st := states[row.EntityID.String()]
		if st == nil {
			st = &ViewerContentState{}
			states[row.EntityID.String()] = st
		}

		st.ReactionKeys = append(st.ReactionKeys, row.ReactionKey)
	}

	bookmarked, err := s.read.ListBookmarkedContentIDs(ctx, gen.ListBookmarkedContentIDsParams{
		UserID: userID,
		Ids:    ids,
	})
	if err != nil {
		return nil, fmt.Errorf("getting batch bookmarks: %w", err)
	}

	for _, entityID := range bookmarked {
		st := states[entityID.String()]
		if st == nil {
			st = &ViewerContentState{}
			states[entityID.String()] = st
		}

		st.Bookmarked = true
	}

	return states, nil
}

// ---------------------------------------------------------------------------
// JSON-LD Templates
// ---------------------------------------------------------------------------

// GetJSONLDTemplate returns the active JSON-LD template for a content type
// within a platform.
func (s *Service) GetJSONLDTemplate(ctx context.Context, contentType string) (gen.ContentTypeJsonldTemplate, error) {
	tmpl, err := s.read.GetJSONLDTemplate(ctx,

		contentType,
	)
	if err != nil {
		return gen.ContentTypeJsonldTemplate{}, fmt.Errorf("getting JSON-LD template: %w", err)
	}

	return tmpl, nil
}

// ResolveJSONLD fetches the active admin-managed JSON-LD template for the
// given content type, interpolates the supplied field values into the
// {{field_key}} placeholders, and returns the ready-to-emit JSON-LD. When no
// active template exists for the content type, it returns (nil, nil) so the
// caller can fall back to a hardcoded generator (Section 14, Section 26).
func (s *Service) ResolveJSONLD(ctx context.Context, contentType string, fields map[string]string) (json.RawMessage, error) {
	tmpl, err := s.GetJSONLDTemplate(ctx, contentType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	body, err := seo.InterpolateJSONLD(tmpl.Template, fields)
	if err != nil {
		return nil, fmt.Errorf("interpolating JSON-LD template for %q: %w", contentType, err)
	}

	return body, nil
}

// ListJSONLDTemplates returns all JSON-LD templates for a platform, optionally
// filtered by q (matches content_type or template body).
func (s *Service) ListJSONLDTemplates(ctx context.Context, q pgtype.Text) ([]gen.ContentTypeJsonldTemplate, error) {
	templates, err := s.read.ListJSONLDTemplates(ctx, q.String)
	if err != nil {
		return nil, fmt.Errorf("listing JSON-LD templates: %w", err)
	}

	return templates, nil
}

// UpsertJSONLDTemplate creates or updates a JSON-LD template for a content
// type within a platform.
func (s *Service) UpsertJSONLDTemplate(ctx context.Context, contentType string, template []byte, isActive bool) (gen.ContentTypeJsonldTemplate, error) {
	tmpl, err := s.write.UpsertJSONLDTemplate(ctx, gen.UpsertJSONLDTemplateParams{
		ContentType: contentType,
		Template:    template,
		IsActive:    isActive,
	})
	if err != nil {
		return gen.ContentTypeJsonldTemplate{}, fmt.Errorf("upserting JSON-LD template: %w", err)
	}

	return tmpl, nil
}

// DeleteJSONLDTemplate removes a JSON-LD template by ID, scoped to platform.
func (s *Service) DeleteJSONLDTemplate(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteJSONLDTemplate(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting JSON-LD template: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Content Scores
// ---------------------------------------------------------------------------

// UpsertContentScore creates or updates the score row for a content item.
func (s *Service) UpsertContentScore(ctx context.Context, contentID pgtype.UUID, voteScore, commentCount, viewCount, bookmarkCount int32, velocityScore pgtype.Numeric) (gen.ContentScore, error) {
	score, err := s.write.UpsertContentScore(ctx, gen.UpsertContentScoreParams{
		ContentID:     contentID,
		VoteScore:     voteScore,
		CommentCount:  commentCount,
		ViewCount:     viewCount,
		BookmarkCount: bookmarkCount,
		VelocityScore: velocityScore,
	})
	if err != nil {
		return gen.ContentScore{}, fmt.Errorf("upserting content score: %w", err)
	}

	return gen.ContentScore{
		ContentID:          score.ContentID,
		VoteScore:          score.VoteScore,
		CommentCount:       score.CommentCount,
		ViewCount:          score.ViewCount,
		BookmarkCount:      score.BookmarkCount,
		VelocityScore:      score.VelocityScore,
		ReactionCount:      score.ReactionCount,
		RelevanceEmbedding: score.RelevanceEmbedding,
		LastScoredAt:       score.LastScoredAt,
	}, nil
}

// GetContentScore returns the score row for a content item.
func (s *Service) GetContentScore(ctx context.Context, contentID pgtype.UUID) (gen.ContentScore, error) {
	score, err := s.read.GetContentScore(ctx, contentID)
	if err != nil {
		return gen.ContentScore{}, fmt.Errorf("getting content score: %w", err)
	}

	return gen.ContentScore{
		ContentID:          score.ContentID,
		VoteScore:          score.VoteScore,
		CommentCount:       score.CommentCount,
		ViewCount:          score.ViewCount,
		BookmarkCount:      score.BookmarkCount,
		VelocityScore:      score.VelocityScore,
		ReactionCount:      score.ReactionCount,
		RelevanceEmbedding: score.RelevanceEmbedding,
		LastScoredAt:       score.LastScoredAt,
	}, nil
}

// ---------------------------------------------------------------------------
// Content Tags
// ---------------------------------------------------------------------------

// AddContentTag associates a tag with content. Idempotent (ON CONFLICT DO NOTHING).
func (s *Service) AddContentTag(ctx context.Context, contentID, tagID pgtype.UUID) error {
	if err := s.write.AddContentTag(ctx, gen.AddContentTagParams{
		ContentID: contentID,
		TagID:     tagID,
	}); err != nil {
		return fmt.Errorf("adding content tag: %w", err)
	}

	return nil
}

// RemoveContentTag disassociates a tag from content.
func (s *Service) RemoveContentTag(ctx context.Context, contentID, tagID pgtype.UUID) error {
	if err := s.write.RemoveContentTag(ctx, gen.RemoveContentTagParams{
		ContentID: contentID,
		TagID:     tagID,
	}); err != nil {
		return fmt.Errorf("removing content tag: %w", err)
	}

	return nil
}

// ListContentTags returns all tags associated with the given content.
func (s *Service) ListContentTags(ctx context.Context, contentID pgtype.UUID) ([]gen.ListContentTagsRow, error) {
	tags, err := s.read.ListContentTags(ctx, contentID)
	if err != nil {
		return nil, fmt.Errorf("listing content tags: %w", err)
	}

	return tags, nil
}

// ListContentByTag returns published content items tagged with the given tag.
// The cursor is a composite (created_at, id) pair to avoid skipping items
// that share the same created_at timestamp. Ref: §4.1.
// Visibility is gated by the supplied filter.
func (s *Service) ListContentByTag(ctx context.Context, tagID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, limit int32, filter visibility.VisibilityFilter) ([]gen.ListContentByTagRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := filter.Params()

	contents, err := s.read.ListContentByTag(ctx, gen.ListContentByTagParams{
		TagID: tagID,

		CursorCreatedAt:   cursorCreatedAt,
		CursorID:          cursorID,
		PageLimit:         limit,
		RequesterID:       requesterID,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeeMembers:     canSeeMembers,
		CanSeePrivate:     canSeePrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("listing content by tag: %w", err)
	}

	return contents, nil
}

// ---------------------------------------------------------------------------
// Content Shares
// ---------------------------------------------------------------------------

// ShareContent records a share event and returns the created share.
func (s *Service) ShareContent(ctx context.Context, contentID, userID pgtype.UUID, platform string) (gen.ContentShare, error) {
	if platform == "" {
		platform = "link"
	}

	share, err := s.write.CreateContentShare(ctx, gen.CreateContentShareParams{
		ContentID: contentID,
		UserID:    userID,
		Platform:  platform,
	})
	if err != nil {
		return gen.ContentShare{}, fmt.Errorf("sharing content: %w", err)
	}

	return share, nil
}

// CountContentShares returns the total share count for content.
func (s *Service) CountContentShares(ctx context.Context, contentID pgtype.UUID) (int32, error) {
	count, err := s.read.CountContentShares(ctx,

		contentID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting content shares: %w", err)
	}

	return count, nil
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

// CreateBookmark saves a bookmark for the given entity. Idempotent — upserts
// folder. folderID is the optional FK to bookmark_folders (BL-011); pass a
// zero pgtype.UUID for "unfiled". When folder is empty, the default folder
// is resolved from the entity/content type and created lazily.
func (s *Service) CreateBookmark(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, folder string, folderID pgtype.UUID) (gen.CreateBookmarkRow, error) {
	if folder == "" {
		contentType := ""

		if entityType == EntityContent || entityType == TypeBlogPost {
			c, err := s.read.GetContentByID(ctx,
				entityID,
			)

			switch {
			case err == nil:
				contentType = c.ContentType
			case errors.Is(err, pgx.ErrNoRows):
				// nothing to do; contentType stays empty
			default:
				slog.Warn("content: failed to look up content type for bookmark folder", "error", err)
			}
		}

		if s.folderResolver != nil {
			f, err := s.folderResolver.Resolve(ctx, userID, entityType, contentType)
			if err == nil && f.ID != "" {
				folder = f.Name

				var fid pgtype.UUID
				if scanErr := fid.Scan(f.ID); scanErr == nil {
					folderID = fid
				}
			} else if err != nil {
				slog.Warn("content: failed to resolve default bookmark folder", "error", err)
			}
		}

		if folder == "" {
			folder = "default"
		}
	}

	bm, err := s.write.CreateBookmark(ctx, gen.CreateBookmarkParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Folder:     folder,
		FolderID:   folderID,
	})
	if err != nil {
		return gen.CreateBookmarkRow{}, fmt.Errorf("creating bookmark: %w", err)
	}

	return bm, nil
}

// SetBookmarkFolder moves an existing bookmark into a folder (BL-011).
// Pass a zero pgtype.UUID to unfile the bookmark. Platform-scoped for
// defense-in-depth (BE-05).
func (s *Service) SetBookmarkFolder(ctx context.Context, userID pgtype.UUID, entityType string, entityID, folderID pgtype.UUID) error {
	if err := s.write.SetBookmarkFolder(ctx, gen.SetBookmarkFolderParams{
		FolderID: folderID,

		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("setting bookmark folder: %w", err)
	}

	return nil
}

// ValidateFolderOwnership checks that a bookmark folder belongs to the given
// user and platform. Returns nil if the folder exists and is owned by the user.
// Returns ErrNotFound if the folder does not exist or is owned by another user.
// A zero/invalid folderID is treated as "unfiled" and returns nil (BL-013).
func (s *Service) ValidateFolderOwnership(ctx context.Context, folderID, userID pgtype.UUID) error {
	if !folderID.Valid || folderID.Bytes == [16]byte{} {
		return nil
	}

	_, err := s.read.GetBookmarkFolder(ctx, gen.GetBookmarkFolderParams{
		ID: folderID,

		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("%w: folder not owned by user", errpkg.ErrNotFound)
	}

	return nil
}

// RemoveBookmark deletes a user's bookmark for the given entity.
// Platform-scoped for defense-in-depth (BE-05).
func (s *Service) RemoveBookmark(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.write.DeleteBookmark(ctx, gen.DeleteBookmarkParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("deleting bookmark: %w", err)
	}

	return nil
}

// IsBookmarked checks if a user has bookmarked the given entity.
func (s *Service) IsBookmarked(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	result, err := s.read.IsBookmarked(ctx, gen.IsBookmarkedParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return false, fmt.Errorf("checking bookmark: %w", err)
	}

	return result, nil
}

// ListBookmarks returns all bookmarks for a user.
func (s *Service) ListBookmarks(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.ListBookmarksByUserRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	bms, err := s.read.ListBookmarksByUser(ctx, gen.ListBookmarksByUserParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing bookmarks: %w", err)
	}

	return bms, nil
}

// ---------------------------------------------------------------------------
// Related Content (pgvector cosine similarity)
// ---------------------------------------------------------------------------

// GetRelatedContent returns up to limit content items most similar to the given
// content, ranked by cosine distance on the relevance_embedding vector.
// An empty contentType returns related items across all types.
func (s *Service) GetRelatedContent(ctx context.Context, excludeID pgtype.UUID, embedding pgvector.Vector, contentType string, limit int32) ([]gen.GetRelatedContentRow, error) {
	if limit <= 0 || limit > 12 {
		limit = 12
	}

	contents, err := s.read.GetRelatedContent(ctx, gen.GetRelatedContentParams{
		ExcludeID:   excludeID,
		Embedding:   &embedding,
		ContentType: contentType,
		PageLimit:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("getting related content: %w", err)
	}

	return contents, nil
}

// ---------------------------------------------------------------------------
// Content Spaces (multi-space assignment — Section 14)
// ---------------------------------------------------------------------------

// GetContentSpaces returns all space assignments for a content item, with
// the primary space first (Section 14: content_spaces).
func (s *Service) GetContentSpaces(ctx context.Context, contentID pgtype.UUID) ([]gen.GetContentSpacesRow, error) {
	return s.read.GetContentSpaces(ctx, contentID)
}

// UpdateContentSpaces replaces the full set of space assignments for a
// content item. The primary space is mirrored on contents.space_id and
// contents.geo_space_id for backward-compatible queries, and a
// content_spaces row is inserted for each assigned space (exactly one with
// is_primary = true). All space IDs are verified to exist in the platform
// before the transaction commits (BUG-018). The entire operation runs in a
// transaction.
func (s *Service) UpdateContentSpaces(ctx context.Context, contentID, userID, primarySpaceID pgtype.UUID, additionalSpaceIDs []pgtype.UUID, bypassSpaceCheck bool) error {
	return db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		// Validate that the primary and all additional spaces exist in
		// this platform before mutating anything (BUG-018).
		spaceIDs := append([]pgtype.UUID{primarySpaceID}, additionalSpaceIDs...)
		for _, sid := range spaceIDs {
			if !sid.Valid {
				return ErrInvalidSpace
			}

			if _, err := q.GetSpaceByID(ctx,
				sid,
			); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrInvalidSpace
				}

				return fmt.Errorf("validating content space: %w", err)
			}
		}

		// Enforce the space posting-permission / membership axis for
		// non-admin callers (Section 45 / BUG-018).
		if !bypassSpaceCheck && s.spaceChecker != nil {
			for _, sid := range spaceIDs {
				ok, err := s.spaceChecker.CanPostInSpace(ctx, sid, userID)
				if err != nil {
					return fmt.Errorf("checking space permission: %w", err)
				}

				if !ok {
					return ErrNotSpaceMember
				}
			}
		}

		// §79: a story always federates into its destination country Space
		// feed — keep story_meta.destination_space_id in the assignment set
		// even when the caller (e.g. the composer's space editor) replaces it,
		// or every save would silently un-federate the story. The destination
		// never requires a posting grant and never displaces the primary.
		if row, err := q.GetContentByID(ctx, contentID); err == nil && row.ContentType == TypeStory {
			if meta, err := q.GetStoryMetaByContent(ctx, contentID); err == nil && meta.DestinationSpaceID.Valid && !isSameUUID(meta.DestinationSpaceID, primarySpaceID) {
				present := false
				for _, sid := range additionalSpaceIDs {
					if isSameUUID(sid, meta.DestinationSpaceID) {
						present = true
						break
					}
				}
				if !present {
					additionalSpaceIDs = append(additionalSpaceIDs, meta.DestinationSpaceID)
				}
			}
		}

		// Clear existing assignments.
		if err := q.DeleteContentSpaces(ctx, contentID); err != nil {
			return fmt.Errorf("deleting content_spaces: %w", err)
		}

		// Insert primary space and keep the contents row in sync.
		if err := q.AddContentSpace(ctx, gen.AddContentSpaceParams{
			ContentID: contentID,
			SpaceID:   primarySpaceID,
			IsPrimary: true,
		}); err != nil {
			return fmt.Errorf("adding primary content_space: %w", err)
		}

		if err := q.SetContentSpaceID(ctx, gen.SetContentSpaceIDParams{
			ContentID: contentID,
			SpaceID:   primarySpaceID,
		}); err != nil {
			return fmt.Errorf("setting primary space on content: %w", err)
		}

		// Insert additional spaces.
		for _, sid := range additionalSpaceIDs {
			if isSameUUID(sid, primarySpaceID) {
				continue // don't duplicate the primary
			}

			if err := q.AddContentSpace(ctx, gen.AddContentSpaceParams{
				ContentID: contentID,
				SpaceID:   sid,
				IsPrimary: false,
			}); err != nil {
				return fmt.Errorf("adding content_space: %w", err)
			}
		}

		return nil
	})
}

// isSameUUID returns true when two pgtype.UUID values are equal and valid.
func isSameUUID(a, b pgtype.UUID) bool {
	return a.Valid && b.Valid && a.Bytes == b.Bytes
}

// hasPrimarySpace returns nil when the content has at least one primary
// space assignment in content_spaces, otherwise ErrMissingPrimarySpace.
func hasPrimarySpace(ctx context.Context, q *gen.Queries, contentID pgtype.UUID) error {
	spaces, err := q.GetContentSpaces(ctx, contentID)
	if err != nil {
		return fmt.Errorf("getting content spaces: %w", err)
	}

	for _, sp := range spaces {
		if sp.IsPrimary {
			return nil
		}
	}

	return ErrMissingPrimarySpace
}

// SetContentSpaceID updates only the space_id column on a content item.
// Used by the auto-assign feature to set the primary space after keyword
// matching. Ref: sections/05-geo-first-space-system.md.
func (s *Service) SetContentSpaceID(ctx context.Context, contentID, spaceID pgtype.UUID) error {
	return s.write.SetContentSpaceID(ctx, gen.SetContentSpaceIDParams{
		ContentID: contentID,
		SpaceID:   spaceID,
	})
}

// forumThreadIndexStats returns the forum-thread signals the §43 sitemap
// quality gate needs for a content row: reply count and solved state.
// isThread is false when no forum_threads row backs the content.
func (s *Service) forumThreadIndexStats(ctx context.Context, contentID pgtype.UUID) (replies int, solved, isThread bool) {
	var rc int32
	err := s.db.Reader.QueryRow(ctx,
		`SELECT reply_count, solved FROM forum_threads
		  WHERE content_id = $1 AND deleted_at IS NULL`, contentID).Scan(&rc, &solved)
	if err != nil {
		return 0, false, false
	}

	return int(rc), solved, true
}
