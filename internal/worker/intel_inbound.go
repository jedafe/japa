package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/db/gen"
)

// intelInboundEmailContentType marks a raw doc built from an inbound email
// delivery — body is the normalized JSON envelope (subject/from/to/
// message_id/text) the email curator parses.
const intelInboundEmailContentType = "application/x-intel-email"

// intelEmailEnvelope is the normalized shape both delivery forms produce:
// provider form posts are marshaled at ingest; raw MIME is parsed into the
// same envelope by HandleIntelInboundEmail.
type intelEmailEnvelope struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
	// HTML is the provider's html-part counterpart to Text (Mailgun
	// stripped-html/body-html). When Text is empty the HTML is
	// tag-stripped into Text so html-only deliveries are not dropped.
	HTML string `json:"html,omitempty"`
}

// HandleIntelInboundEmail is the intel:inbound_email task handler
// (intel-channels Phase 3): the HTTP endpoint persisted the delivery as an
// intel_inbound_events row; this task normalizes it into a raw document
// (application/x-intel-email envelope), stamps the event processed, and
// hands off to intel:detect_changes like the fetch path.
func HandleIntelInboundEmail(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelInboundEmail")
	slog.Info("task:intel:inbound_email")

	if intelStores == nil || intelSourcesStore == nil {
		slog.Warn("task:intel:inbound_email: dependencies not wired, skipping")
		return nil
	}

	var p struct {
		Version string `json:"version"`
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:intel:inbound_email: payload: %w", err)
	}
	eventID, err := scanUUID(p.EventID)
	if err != nil {
		return fmt.Errorf("task:intel:inbound_email: event_id: %w", err)
	}

	event, err := intelSourcesStore.GetInboundEvent(ctx, eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:inbound_email: event no longer exists, skipping", "event_id", p.EventID)
			return nil
		}
		return fmt.Errorf("task:intel:inbound_email: load event: %w", err)
	}
	if event.Status != "received" {
		// A retry of an already-terminal event is a no-op — the raw doc
		// (if any) was already produced by the first run.
		slog.Info("task:intel:inbound_email: event already terminal, skipping", "event_id", p.EventID, "status", event.Status)
		return nil
	}

	mark := func(status string) {
		_ = intelSourcesStore.MarkInboundEvent(ctx, gen.MarkIntelInboundEventParams{ID: event.ID, Status: status})
	}

	ch, err := intelSourcesStore.GetChannel(ctx, event.ChannelID)
	if err != nil {
		mark("failed")
		return fmt.Errorf("task:intel:inbound_email: load channel: %w", err)
	}
	src, err := intelSourcesStore.Get(ctx, ch.SourceID)
	if err != nil {
		mark("failed")
		return fmt.Errorf("task:intel:inbound_email: load source: %w", err)
	}

	env, err := intelInboundEmailEnvelope(event)
	if err != nil {
		mark("failed")
		return fmt.Errorf("task:intel:inbound_email: parse delivery: %w", err)
	}
	env.HTML = "" // already folded into Text — don't duplicate it in the raw doc

	body, _ := json.Marshal(env)
	contentHash := sha256.Sum256(body)

	docURL := "email:" + ch.ID.String() + ":" + firstNonEmptyWorker(env.MessageID, event.ID.String())
	headersJSON, _ := json.Marshal(map[string]string{
		"from":       env.From,
		"to":         env.To,
		"subject":    env.Subject,
		"message_id": env.MessageID,
	})

	// Prior hash BEFORE insert — detect_changes diffs against the channel's
	// previous delivery, not the doc being persisted.
	priorHash, _ := intelStores.GetLatestIntelRawDocHashForChannel(ctx, ch.ID)

	rawDoc, err := intelStores.CreateIntelRawDocument(ctx, gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         docURL,
		HttpStatus:  200,
		HttpHeaders: headersJSON,
		ContentType: strPtr(intelInboundEmailContentType),
		Body:        body,
		ContentHash: hex.EncodeToString(contentHash[:]),
	})
	if err != nil {
		mark("failed")
		return fmt.Errorf("task:intel:inbound_email: persist raw doc: %w", err)
	}

	payload := marshalPayload(intelDetectChangesPayload{
		Version:   intelPayloadVersion,
		SourceID:  ch.SourceID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
		PriorHash: priorHash,
	})
	if payload != nil {
		enqueueIntel(TypeIntelDetectChanges, payload)
	}

	mark("processed")
	slog.Info("task:intel:inbound_email: processed", "event_id", p.EventID, "raw_doc_id", rawDoc.ID.String(), "channel_id", ch.ID.String())

	return nil
}

// intelInboundEmailEnvelope normalizes a stored delivery to the shared
// envelope: form posts arrive pre-marshaled (application/x-intel-email),
// raw MIME (message/rfc822) is walked for the best text part.
func intelInboundEmailEnvelope(event gen.IntelInboundEvent) (intelEmailEnvelope, error) {
	ct := ""
	if event.ContentType != nil {
		ct = *event.ContentType
	}

	if ct == intelInboundEmailContentType {
		var env intelEmailEnvelope
		if err := json.Unmarshal(event.Payload, &env); err != nil {
			return env, fmt.Errorf("decode envelope: %w", err)
		}
		if env.Text == "" && env.HTML != "" {
			env.Text = stripHTMLWorker(env.HTML)
		}
		return env, nil
	}

	return parseMIMEMessage(event.Payload)
}

// parseMIMEMessage extracts headers + the preferred text body from a raw
// RFC822 message. Multipart trees prefer text/plain; text/html falls back
// tag-stripped; a lone html body is stripped the same way.
func parseMIMEMessage(raw []byte) (intelEmailEnvelope, error) {
	var env intelEmailEnvelope

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return env, err
	}

	env.From = msg.Header.Get("From")
	env.To = msg.Header.Get("To")
	env.Subject = msg.Header.Get("Subject")
	env.MessageID = msg.Header.Get("Message-Id")

	mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "multipart/") {
		env.Text = bestMIMEPartText(multipart.NewReader(msg.Body, params["boundary"]))
		return env, nil
	}

	body, err := io.ReadAll(io.LimitReader(msg.Body, 2<<20))
	if err != nil {
		return env, err
	}
	if mediaType == "text/html" {
		env.Text = stripHTMLWorker(string(body))
	} else {
		env.Text = string(body)
	}

	return env, nil
}

// bestMIMEPartText walks a multipart tree and returns the preferred body:
// the first text/plain wins; otherwise the first text/html stripped of
// tags; otherwise the first non-attachment part's bytes.
func bestMIMEPartText(r *multipart.Reader) string {
	var html, first []byte

	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}
		b, err := io.ReadAll(io.LimitReader(part, 2<<20))
		if err != nil {
			continue
		}
		ct := part.Header.Get("Content-Type")
		mt, _, _ := mime.ParseMediaType(ct)
		switch {
		case mt == "text/plain":
			return string(b)
		case mt == "text/html" && html == nil:
			html = b
		case first == nil && part.FileName() == "":
			first = b
		}
	}

	switch {
	case html != nil:
		return stripHTMLWorker(string(html))
	default:
		return string(first)
	}
}

// stripHTMLWorker drops tags and collapses whitespace — enough for an
// email fallback body the curator/LLM consumes.
func stripHTMLWorker(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}

	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}

func firstNonEmptyWorker(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
