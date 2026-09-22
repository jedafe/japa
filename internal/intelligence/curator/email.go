package curator

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Email curates email crawl-method channels: the raw doc body is the
// normalized application/x-intel-email envelope the intel:inbound_email
// task produced ({from, to, subject, message_id, text}). One record per
// delivery — the subject is the title, the body text the summary/body,
// and the first http(s) link in the text the article_url.
type Email struct{}

// Handles returns the crawl method this curator serves.
func (Email) Handles() string { return string(sources.CrawlMethodEmail) }

// emailEnvelope mirrors worker's intelEmailEnvelope — the two packages
// can't share a type (import cycle through internal/ai), so the JSON
// contract is duplicated at the seam.
type emailEnvelope struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

// emailSummaryCap bounds the summary field; the full body still lands in
// the body field for the LLM/reviewer.
const emailSummaryCap = 4000

var emailLinkRe = regexp.MustCompile(`https?://[^\s<>"')\]]+`)

// Curate normalizes the email envelope into a single record keyed on
// {channel_id}:{message-id}.
func (Email) Curate(_ context.Context, body []byte, _ string, ch sources.Channel) ([]Record, error) {
	var env emailEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, errors.New("curator: email: decode envelope: " + err.Error())
	}
	if env.Subject == "" && env.Text == "" {
		// A blank delivery is noise, not a parse failure — no records, no
		// review stub. The inbound event row already carries the audit trail.
		return nil, nil
	}

	entityType := resolveEntityType("", ch)
	ext := make(extractor.Extraction)

	field(ext, "title", env.Subject, env.Subject)

	text := strings.TrimSpace(env.Text)
	if text != "" {
		summary := text
		if len(summary) > emailSummaryCap {
			summary = summary[:emailSummaryCap]
		}
		field(ext, "summary", summary, text)
		field(ext, "body", text, text)
	}

	if link := firstEmailLink(text); link != "" {
		field(ext, "article_url", link, link)
	}
	if env.From != "" {
		field(ext, "author", extractAuthorName(env.From), env.From)
	}

	key := env.MessageID
	if key == "" {
		key = "hash:" + hashKey(env.Subject, env.From, text)
	}

	return []Record{{
		Extraction:  validate(ext, entityType),
		ExternalKey: ch.ID.String() + ":" + key,
		EntityType:  entityType,
	}}, nil
}

// firstEmailLink returns the first absolute http(s) URL in the body text.
func firstEmailLink(text string) string {
	for _, m := range emailLinkRe.FindAllString(text, 3) {
		m = strings.TrimRight(m, ".,;:!?")
		if u, err := url.Parse(m); err == nil && u.Host != "" {
			return m
		}
	}

	return ""
}
