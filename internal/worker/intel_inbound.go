package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	"unicode"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/sources"
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
		// Terminal or claimed: processed/failed/ignored retries are no-ops,
		// and 'processing' means another worker holds the claim (a crashed
		// claimant leaves a stuck event the admin redrive rescues).
		slog.Info("task:intel:inbound_email: event already claimed/terminal, skipping", "event_id", p.EventID, "status", event.Status)
		return nil
	}

	// Atomic claim: received → processing. Two concurrent runs of the same
	// event (duplicate enqueue, overlapping retry) cannot both win — only
	// the claim holder materializes the delivery.
	claimed, err := intelSourcesStore.ClaimInboundEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("task:intel:inbound_email: claim event: %w", err)
	}
	if !claimed {
		slog.Info("task:intel:inbound_email: claim lost, another run owns the event", "event_id", p.EventID)
		return nil
	}

	mark := func(status string) {
		_ = intelSourcesStore.MarkInboundEvent(ctx, gen.MarkIntelInboundEventParams{ID: event.ID, Status: status})
	}
	// Transient failures (DB/queue blips) release the claim back to
	// 'received' and return the error so the task's configured retries can
	// win the CAS again — marking 'failed' on a recoverable error would
	// strand the delivery on manual redrive while the remaining retries
	// no-op against the terminal status.
	release := func() {
		_ = intelSourcesStore.ReleaseInboundEvent(ctx, event.ID)
	}

	ch, err := intelSourcesStore.GetChannel(ctx, event.ChannelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			mark("ignored") // channel deleted between ingest and task — permanently unprocessable
			return nil
		}
		release()
		return fmt.Errorf("task:intel:inbound_email: load channel: %w", err)
	}
	// The channel may have been disabled or repurposed between ingest and
	// task execution — materializing an email envelope under a non-email
	// method would feed it to the wrong curator. 'ignored' keeps the
	// delivery auditable and admin-redrivable if the channel comes back.
	if !ch.Enabled || ch.CrawlMethod != string(sources.CrawlMethodEmail) {
		mark("ignored")
		slog.Info("task:intel:inbound_email: channel disabled or repurposed, ignoring", "event_id", p.EventID, "channel_id", ch.ID.String())
		return nil
	}
	src, err := intelSourcesStore.Get(ctx, ch.SourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			mark("ignored") // source deleted between ingest and task
			return nil
		}
		release()
		return fmt.Errorf("task:intel:inbound_email: load source: %w", err)
	}
	if !src.Enabled || src.Status != "active" {
		mark("ignored")
		slog.Info("task:intel:inbound_email: parent source not active, ignoring", "event_id", p.EventID, "source_id", src.ID.String())
		return nil
	}

	env, err := intelInboundEmailEnvelope(event)
	if err != nil {
		// The stored payload is immutable — a retry can never parse it.
		// 'failed' is the honest terminal state; the event row is the audit.
		mark("failed")
		slog.Warn("task:intel:inbound_email: unparseable delivery", "event_id", p.EventID, "error", err)
		return nil
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

	// Idempotent materialization: the deterministic docURL + content hash
	// mean a redrive after a crash-between-insert-and-mark (or any second
	// run of this event) reuses the existing doc instead of duplicating.
	contentHashHex := hex.EncodeToString(contentHash[:])
	rawDocID, found, err := intelStores.GetIntelRawDocIDByChannelURLHash(ctx, ch.ID, docURL, contentHashHex)
	if err != nil {
		release()
		return fmt.Errorf("task:intel:inbound_email: lookup existing raw doc: %w", err)
	}
	if !found {
		rawDoc, err := intelStores.CreateIntelRawDocument(ctx, gen.CreateIntelRawDocumentParams{
			SourceID:    src.ID,
			ChannelID:   ch.ID,
			Url:         docURL,
			HttpStatus:  200,
			HttpHeaders: headersJSON,
			ContentType: strPtr(intelInboundEmailContentType),
			Body:        body,
			ContentHash: contentHashHex,
		})
		if err != nil {
			release()
			return fmt.Errorf("task:intel:inbound_email: persist raw doc: %w", err)
		}
		rawDocID = rawDoc.ID
	} else {
		// Re-materialization (redrive / crash between insert and mark): the
		// existing doc is already this channel's latest, so priorHash
		// self-references and detect_changes would decide 'unchanged' —
		// starving the pipeline and marking the event processed without
		// producing records. An empty prior forces the pipeline to run;
		// (source_id, external_key) dedup keeps re-curation idempotent.
		priorHash = ""
	}

	payload := marshalPayload(intelDetectChangesPayload{
		Version:   intelPayloadVersion,
		SourceID:  ch.SourceID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDocID.String(),
		PriorHash: priorHash,
	})
	if payload == nil {
		mark("failed")
		return fmt.Errorf("task:intel:inbound_email: marshal detect_changes payload")
	}
	// The hand-off must not be fire-and-forget: unlike the crawl path there
	// is no next cycle to resume a dropped chain, and 'processed' is
	// terminal — an enqueue failure here would lose the delivery forever.
	if _, err := intelAsynqClient.EnqueueContext(ctx, asynq.NewTask(TypeIntelDetectChanges, payload),
		TaskOpts(TypeIntelDetectChanges)...); err != nil {
		release()
		return fmt.Errorf("task:intel:inbound_email: enqueue detect_changes: %w", err)
	}

	mark("processed")
	slog.Info("task:intel:inbound_email: processed", "event_id", p.EventID, "raw_doc_id", rawDocID.String(), "channel_id", ch.ID.String())

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
	body = decodeMIMETransfer(body, msg.Header.Get("Content-Transfer-Encoding"))
	if mediaType == "text/html" {
		env.Text = stripHTMLWorker(string(body))
	} else {
		env.Text = string(body)
	}

	return env, nil
}

// mimeLeaf is one decoded leaf part of a (possibly nested) multipart tree.
type mimeLeaf struct {
	mediaType  string
	attachment bool
	body       []byte
}

// walkMIMEParts flattens a multipart tree into its leaf parts in document
// order — real-world mail routinely nests multipart/alternative inside
// multipart/mixed (any mail with an attachment), and multipart.Related/
// NextPart do not recurse, so without the descent the leaf is the raw
// nested-MIME blob. Depth and part count are capped so a pathological
// tree cannot loop the worker.
func walkMIMEParts(r *multipart.Reader, out *[]mimeLeaf, depth int) {
	if depth > 8 || len(*out) >= 128 {
		return
	}
	for {
		part, err := r.NextPart()
		if err != nil {
			return // io.EOF at the end of this level, or a malformed boundary
		}
		mt, params, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if mt == "" {
			mt = "text/plain" // RFC 2046 §5.1: parts default to text/plain
		}
		if strings.HasPrefix(mt, "multipart/") && params["boundary"] != "" {
			walkMIMEParts(multipart.NewReader(part, params["boundary"]), out, depth+1)
			continue
		}
		b, err := io.ReadAll(io.LimitReader(part, 2<<20))
		if err != nil {
			continue
		}
		*out = append(*out, mimeLeaf{
			mediaType:  mt,
			attachment: part.FileName() != "",
			body:       decodeMIMETransfer(b, part.Header.Get("Content-Transfer-Encoding")),
		})
	}
}

// decodeMIMETransfer applies a part's Content-Transfer-Encoding: base64
// bodies are decoded (folding whitespace stripped first — CTE base64 is
// CRLF-wrapped and strict DecodeString only tolerates \r\n); quoted-
// printable is already transparently decoded by multipart.Reader;
// 7bit/8bit/binary pass through.
func decodeMIMETransfer(b []byte, cte string) []byte {
	if !strings.EqualFold(strings.TrimSpace(cte), "base64") {
		return b
	}
	compact := bytes.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, b)
	if d, err := base64.StdEncoding.DecodeString(string(compact)); err == nil {
		return d
	}

	return b
}

// bestMIMEPartText picks the preferred body across the flattened leaf
// parts: the first non-attachment text/plain wins; otherwise the first
// text/plain (even an attachment — better than nothing); otherwise the
// first non-attachment text/html stripped of tags; otherwise the first
// non-attachment part's bytes.
func bestMIMEPartText(r *multipart.Reader) string {
	var parts []mimeLeaf
	walkMIMEParts(r, &parts, 0)

	var plain, html, first []byte
	for _, p := range parts {
		switch {
		case p.mediaType == "text/plain" && !p.attachment:
			return string(p.body)
		case p.mediaType == "text/plain" && plain == nil:
			plain = p.body
		case p.mediaType == "text/html" && !p.attachment && html == nil:
			html = p.body
		case first == nil && !p.attachment:
			first = p.body
		}
	}

	switch {
	case plain != nil:
		return string(plain)
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

// HandleIntelPurgeInboundEvents is the intel:purge_inbound_events task
// handler (daily cron, low queue): deletes intel_inbound_events rows whose
// received_at is older than intel.inbound.retention_days (default 30). Raw
// delivery payloads carry sender PII (addresses, subjects, bodies), so
// retention is bounded — terminal rows are history, and a received or
// processing row that old is a lost delivery whose redrive value has
// expired. retention_days <= 0 disables the sweep entirely.
func HandleIntelPurgeInboundEvents(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelPurgeInboundEvents")
	slog.Info("task:intel:purge_inbound_events")

	if intelSourcesStore == nil {
		slog.Warn("task:intel:purge_inbound_events: dependencies not wired, skipping")
		return nil
	}

	days := 30
	if appSettings != nil {
		days = appSettings.GetIntOr(ctx, "intel.inbound.retention_days", 30)
	}
	if days <= 0 {
		slog.Info("task:intel:purge_inbound_events: retention disabled (retention_days <= 0), skipping")
		return nil
	}

	deleted, err := intelSourcesStore.PurgeInboundEvents(ctx, int32(days))
	if err != nil {
		return fmt.Errorf("task:intel:purge_inbound_events: %w", err)
	}

	slog.Info("task:intel:purge_inbound_events: sweep complete", "deleted", deleted, "retention_days", days)

	return nil
}
