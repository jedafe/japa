package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// invalidSlugChars matches any character that is not a lowercase letter,
// digit, or hyphen. These are stripped during slug normalization.
var invalidSlugChars = regexp.MustCompile(`[^a-z0-9-]+`)

// multipleHyphens matches consecutive hyphens, replaced with a single one.
var multipleHyphens = regexp.MustCompile(`-{2,}`)

// leadingTrailingHyphens matches hyphens at the start or end of a string.
var leadingTrailingHyphens = regexp.MustCompile(`^-+|-+$`)

// Slugify converts a title into a URL-safe slug: lowercase, hyphenated,
// alphanumeric only. Empty or whitespace-only input returns "untitled".
func Slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = invalidSlugChars.ReplaceAllString(s, "-")
	s = multipleHyphens.ReplaceAllString(s, "-")
	s = leadingTrailingHyphens.ReplaceAllString(s, "")

	if s == "" {
		return "untitled"
	}

	return s
}

// MaxSlugAttempts bounds the number of suffix candidates tried by
// EnsureUniqueSlug to prevent infinite loops when collisions are common.
const MaxSlugAttempts = 100

// EnsureUniqueSlug checks whether the given slug is already in use for the
// tenant. If it is, a numeric suffix is appended (-2, -3, ...) until a
// unique slug is found. The check is limited to MaxSlugAttempts iterations
// to prevent infinite loops.
func (s *Service) EnsureUniqueSlug(ctx context.Context, slug string, tenantID pgtype.UUID) (string, error) {
	base := slug
	for i := 2; i <= MaxSlugAttempts; i++ {
		candidate := base
		if i > 2 {
			candidate = fmt.Sprintf("%s-%d", base, i-1)
		}

		_, err := s.read.GetContentBySlug(ctx, gen.GetContentBySlugParams{
			Slug:     candidate,
			TenantID: tenantID,
		})
		if err != nil {
			// pgx.ErrNoRows means the slug is available.
			return candidate, nil //nolint:nilerr // intentional: no row means slug is unique
		}
	}

	return "", fmt.Errorf("could not find unique slug after %d attempts", MaxSlugAttempts)
}

// RecordSlugChange writes a slug_history entry when a content's slug
// changes. This enables 301 redirects from old URLs to new ones,
// preserving SEO equity. The call is silent on error — a failed history
// write should not block a content update.
func (s *Service) RecordSlugChange(ctx context.Context, entityType string, entityID pgtype.UUID, oldSlug, newSlug string) {
	if oldSlug == newSlug || oldSlug == "" {
		return
	}

	if _, err := s.write.CreateSlugHistory(ctx, gen.CreateSlugHistoryParams{
		EntityType: entityType,
		EntityID:   entityID,
		OldSlug:    oldSlug,
		NewSlug:    newSlug,
	}); err != nil {
		// A failed history entry should not block the content update.
		slog.Error("recording slug change",
			"error", err, "entity_type", entityType, "entity_id", entityID.String(),
			"old_slug", oldSlug, "new_slug", newSlug)
	}
}

// ResolveSlugRedirect looks up the most recent slug_history entry for the
// given entity type and old slug. If found, it returns the new slug and
// true. If no redirect exists, it returns an empty string and false.
func (s *Service) ResolveSlugRedirect(ctx context.Context, entityType, oldSlug string) (string, bool) {
	history, err := s.read.GetSlugRedirect(ctx, gen.GetSlugRedirectParams{
		EntityType: entityType,
		OldSlug:    oldSlug,
	})
	if err != nil {
		return "", false
	}

	return history.NewSlug, true
}

// ComputeReadTime estimates reading time in minutes from a body of text.
// The standard rate is 200 words per minute, rounded up.
func ComputeReadTime(body string) int32 {
	wordCount := len(strings.Fields(body))
	if wordCount == 0 {
		return 0
	}

	minutes := wordCount / 200
	if wordCount%200 > 0 {
		minutes++
	}

	return int32(minutes) //nolint:gosec // G115: minutes bounded by time calculation
}

// ComputeContentHash returns a SHA-256 hex digest of the normalised body.
// Used to detect duplicate content and skip no-op revisions.
func ComputeContentHash(body string) string {
	normalised := strings.TrimSpace(body)
	h := sha256.Sum256([]byte(normalised))

	return hex.EncodeToString(h[:])
}
