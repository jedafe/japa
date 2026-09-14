package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
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

// whitespaceRun matches any run of whitespace characters (spaces, tabs,
// newlines) for collapsing to a single space during content_hash
// normalisation. Ref: Rule 12 / §14 — hex(sha256(strip_markdown(body)
//
//	.trim().replaceAll(/\s+/g, ' ').toLowerCase()))
var whitespaceRun = regexp.MustCompile(`\s+`)

// markdownPatterns strip common Markdown / markup syntax so the content hash
// is based on the actual text rather than formatting characters. Ref: §41.
var (
	markdownHeader     = regexp.MustCompile(`(?m)^#{1,6}\s*`)
	markdownBold       = regexp.MustCompile(`\*\*(.+?)\*\*|` + "`" + `__(.+?)__`)
	markdownItalic     = regexp.MustCompile(`\*(.+?)\*|_(.+?)_`)
	markdownLinkOrImg  = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	markdownInlineCode = regexp.MustCompile("`+([^`]+)`+")
	markdownCodeFence  = regexp.MustCompile("```[\\s\\S]*?```")
	markdownList       = regexp.MustCompile(`(?m)^\s*(?:[-*+]|\d+\.)\s+`)
	markdownBlockquote = regexp.MustCompile(`(?m)^\s*>\s*`)
	htmlTag            = regexp.MustCompile(`<[^>]+>`)
)

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
func (s *Service) EnsureUniqueSlug(ctx context.Context, slug string) (string, error) {
	return s.ensureUniqueSlugWithQueries(ctx, s.read, slug)
}

// ensureUniqueSlugWithQueries is the shared slug-uniqueness check that
// accepts a *gen.Queries so it can run inside a transaction (BUG-001).
func (s *Service) ensureUniqueSlugWithQueries(ctx context.Context, q *gen.Queries, slug string) (string, error) {
	base := slug
	for i := 2; i <= MaxSlugAttempts; i++ {
		candidate := base
		if i > 2 {
			candidate = fmt.Sprintf("%s-%d", base, i-1)
		}

		_, err := q.GetContentBySlug(ctx,
			candidate,
		)
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
// The standard rate is 200 words per minute, rounded up, clamped to int32.
func ComputeReadTime(body string) int32 {
	wordCount := int64(len(strings.Fields(body)))
	if wordCount == 0 {
		return 0
	}

	minutes := wordCount / 200
	if wordCount%200 > 0 {
		minutes++
	}

	if minutes > math.MaxInt32 {
		minutes = math.MaxInt32
	}

	return int32(minutes) // #nosec
}

// ComputeContentHash returns a SHA-256 hex digest of the normalised body.
// Used to detect duplicate content and skip no-op revisions.
// Ref: Rule 12 / §14 / §41 — hex(sha256(strip_markdown(body)
//
//	.trim().replaceAll(/\s+/g, ' ').toLowerCase()))
//
// The input may be plain text, Markdown, HTML, or a TipTap/ProseMirror JSON
// document. All markup and formatting characters are stripped so the hash
// reflects the underlying textual content.
func ComputeContentHash(body string) string {
	normalised := stripMarkdown(body)
	normalised = strings.TrimSpace(normalised)
	normalised = whitespaceRun.ReplaceAllString(normalised, " ")
	normalised = strings.ToLower(normalised)
	h := sha256.Sum256([]byte(normalised))

	return hex.EncodeToString(h[:])
}

// ComputeContentHashWithDistinguisher returns a SHA-256 hex digest of the
// normalised body, appending a video source id when present. This allows two
// posts with identical body text but different video sources to coexist
// without colliding.
func ComputeContentHashWithDistinguisher(body, videoSourceID string) string {
	normalised := stripMarkdown(body)
	normalised = strings.TrimSpace(normalised)
	normalised = whitespaceRun.ReplaceAllString(normalised, " ")
	normalised = strings.ToLower(normalised)

	if videoSourceID != "" {
		normalised += "|" + videoSourceID
	}

	h := sha256.Sum256([]byte(normalised))

	return hex.EncodeToString(h[:])
}

// stripMarkdown removes Markdown, HTML, and TipTap JSON structure from a body
// so only the underlying text remains. Ref: §41 (duplicate content detection).
func stripMarkdown(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	// If the body is a TipTap / ProseMirror JSON document, extract text nodes.
	first := strings.TrimLeftFunc(body, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	if (first != "" && (first[0] == '{' || first[0] == '[')) || strings.HasPrefix(first, "\"") {
		if text, ok := extractTipTapText([]byte(body)); ok {
			return text
		}
	}

	// Strip fenced code blocks first so their inner backticks don't leak.
	body = markdownCodeFence.ReplaceAllString(body, " ")

	// Remove HTML tags before Markdown so Markdown inside tags is dropped.
	body = htmlTag.ReplaceAllString(body, " ")

	// Strip Markdown formatting while preserving the meaningful text.
	body = markdownHeader.ReplaceAllString(body, "")
	body = markdownBold.ReplaceAllString(body, "$1$2")
	body = markdownItalic.ReplaceAllString(body, "$1$2")
	body = markdownLinkOrImg.ReplaceAllString(body, "$1")
	body = markdownInlineCode.ReplaceAllString(body, "$1")
	body = markdownList.ReplaceAllString(body, "")
	body = markdownBlockquote.ReplaceAllString(body, "")

	return body
}

// ExtractTipTapText walks a TipTap/ProseMirror JSON document and concatenates
// all "text" leaf values. It is exported so cross-package callers (e.g. forum)
// can derive a plain-text preview from a raw TipTap body.
func ExtractTipTapText(b []byte) (string, bool) {
	return extractTipTapText(b)
}

// extractTipTapText is the private implementation shared by ExtractTipTapText
// and ComputeContentHash.
func extractTipTapText(b []byte) (string, bool) {
	var doc struct {
		Content []tipTapNode `json:"content"`
	}

	if err := json.Unmarshal(b, &doc); err != nil {
		// Could be a bare array of nodes.
		var nodes []tipTapNode
		if err2 := json.Unmarshal(b, &nodes); err2 != nil {
			return "", false
		}

		doc.Content = nodes
	}

	var out strings.Builder
	for _, node := range doc.Content {
		extractNodeText(&out, &node)
	}

	return out.String(), true
}

type tipTapNode struct {
	Type    string       `json:"type"`
	Text    string       `json:"text"`
	Content []tipTapNode `json:"content"`
	Attrs   tipTapAttrs  `json:"attrs"`
}

type tipTapAttrs struct {
	Alt string `json:"alt"`
}

func extractNodeText(out *strings.Builder, node *tipTapNode) {
	if node.Text != "" {
		out.WriteString(node.Text)
		out.WriteByte(' ')

		return
	}

	if node.Attrs.Alt != "" {
		out.WriteString(node.Attrs.Alt)
		out.WriteByte(' ')
	}

	for i := range node.Content {
		extractNodeText(out, &node.Content[i])
	}
}
