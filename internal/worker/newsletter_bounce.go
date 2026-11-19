// Package worker — newsletter_bounce.go polls an IMAP bounce/complaint
// mailbox and applies subscriber suppression (§62 self-hosted MTA
// transport, ADR-080 §4). Provider transports deliver bounce/complaint
// webhooks to POST /newsletter/webhook; a bare SMTP MTA returns DSNs and
// ARF reports to the envelope return-path mailbox instead. This task
// parses them: delivery-status reports (Action: failed / Status: 5.x)
// mark the recipient 'bounced'; feedback reports (Feedback-Type: abuse)
// mark 'complained'; the X-Failed-Recipients header covers non-DSN
// notices. Processed messages are flagged \Seen — unparseable mail is
// logged and marked seen too, so a poison message cannot loop the poll.
//
// Env: JC_BOUNCE_IMAP_HOST (unset = no-op), JC_BOUNCE_IMAP_PORT (993),
// JC_BOUNCE_IMAP_USER, JC_BOUNCE_IMAP_PASS, JC_BOUNCE_IMAP_MAILBOX
// (INBOX), JC_BOUNCE_IMAP_TLS (true), JC_BOUNCE_IMAP_BATCH (200).
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/hibiken/asynq"
	"github.com/jedafe/japa/internal/db/gen"
)

// bounceIMAPConfig carries the bounce-mailbox connection settings.
type bounceIMAPConfig struct {
	host    string
	port    string
	user    string
	pass    string
	mailbox string
	tls     bool
	batch   int
}

// readBounceIMAPConfig reads bounce-mailbox settings from JC_BOUNCE_IMAP_*
// env vars. Returns nil when the host is unset (feature off).
func readBounceIMAPConfig() *bounceIMAPConfig {
	host := strings.TrimSpace(os.Getenv("JC_BOUNCE_IMAP_HOST"))
	if host == "" {
		return nil
	}

	port := strings.TrimSpace(os.Getenv("JC_BOUNCE_IMAP_PORT"))
	if port == "" {
		port = "993"
	}

	mailbox := strings.TrimSpace(os.Getenv("JC_BOUNCE_IMAP_MAILBOX"))
	if mailbox == "" {
		mailbox = "INBOX"
	}

	tls := !strings.EqualFold(os.Getenv("JC_BOUNCE_IMAP_TLS"), "false")

	batch := 200
	if v := strings.TrimSpace(os.Getenv("JC_BOUNCE_IMAP_BATCH")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			batch = n
		}
	}

	return &bounceIMAPConfig{
		host:    host,
		port:    port,
		user:    os.Getenv("JC_BOUNCE_IMAP_USER"),
		pass:    os.Getenv("JC_BOUNCE_IMAP_PASS"),
		mailbox: mailbox,
		tls:     tls,
		batch:   batch,
	}
}

// bounceEvent is one extracted suppression: an address and the status it
// should be moved to ("bounced" or "complained").
type bounceEvent struct {
	email string
	kind  string
}

// HandleNewsletterPollBounces polls the configured IMAP bounce mailbox,
// parses DSN/ARF/non-DSN notices, and suppresses the extracted addresses
// via MarkBounced/MarkComplained — the self-hosted-transport counterpart
// of POST /newsletter/webhook (§62). Scheduled every 15 min on the low
// tier; singleton across replicas via jc:newsletter:bounce_poll_lock.
// No-ops cleanly when JC_BOUNCE_IMAP_HOST is unset.
func HandleNewsletterPollBounces(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:newsletter:poll_bounces")

	if dbPool == nil {
		slog.Warn("task:newsletter:poll_bounces: no database, skipping")
		return nil
	}

	// Honor the feature.newsletter kill-switch — no subscriber mutation
	// when the flag is off (same gate as newsletter:send/digest, §62).
	if featureChecker != nil && !featureChecker.IsEnabled(ctx, "feature.newsletter") {
		slog.Info("task:newsletter:poll_bounces: feature.newsletter disabled, skipping")
		return nil
	}

	cfg := readBounceIMAPConfig()
	if cfg == nil {
		slog.Debug("task:newsletter:poll_bounces: JC_BOUNCE_IMAP_HOST unset, skipping")
		return nil
	}

	// Singleton lock — the scheduler fires on every replica, but only one
	// should hold the IMAP session per window (SET NX, 10m TTL — the lock
	// self-heals on crash).
	if rdb != nil {
		taskID, _ := asynq.GetTaskID(ctx)
		if taskID == "" {
			taskID = "poll"
		}

		ok, err := rdb.SetNX(ctx, "jc:newsletter:bounce_poll_lock", taskID, 10*time.Minute).Result()
		if err != nil {
			slog.Warn("task:newsletter:poll_bounces: lock check failed, proceeding unlocked", "error", err)
		} else if !ok {
			slog.Info("task:newsletter:poll_bounces: another replica holds the lock, skipping")
			return nil
		}
	}

	if err := pollBounceMailbox(ctx, cfg); err != nil {
		return fmt.Errorf("task:newsletter:poll_bounces: %w", err)
	}

	return nil
}

// pollBounceMailbox runs one IMAP session: select, search UNSEEN, fetch
// up to cfg.batch messages, apply extracted suppressions, flag all
// processed messages \Seen.
func pollBounceMailbox(ctx context.Context, cfg *bounceIMAPConfig) error {
	addr := cfg.host + ":" + cfg.port

	var client *imapclient.Client

	var err error

	if cfg.tls {
		client, err = imapclient.DialTLS(addr, nil)
	} else {
		client, err = imapclient.DialInsecure(addr, nil)
	}

	if err != nil {
		return fmt.Errorf("imap dial %s: %w", addr, err)
	}

	defer func() {
		if err := client.Close(); err != nil {
			slog.Warn("task:newsletter:poll_bounces: client close failed", "error", err)
		}
	}()

	if err := client.Login(cfg.user, cfg.pass).Wait(); err != nil {
		return fmt.Errorf("imap login %s: %w", cfg.host, err)
	}

	if _, err := client.Select(cfg.mailbox, nil).Wait(); err != nil {
		return fmt.Errorf("imap select %s: %w", cfg.mailbox, err)
	}

	searchData, err := client.Search(&imap.SearchCriteria{
		NotFlag: []imap.Flag{imap.FlagSeen},
	}, nil).Wait()
	if err != nil {
		return fmt.Errorf("imap search: %w", err)
	}

	seqNums := searchData.AllSeqNums()
	if len(seqNums) == 0 {
		return nil
	}

	if len(seqNums) > cfg.batch {
		seqNums = seqNums[:cfg.batch]
	}

	bodySection := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierNone, Peek: true}

	msgs, err := client.Fetch(imap.SeqSetNum(seqNums...), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}).Collect()
	if err != nil {
		return fmt.Errorf("imap fetch: %w", err)
	}

	q := gen.New(dbPool.Writer)
	processed := make([]uint32, 0, len(msgs))
	bounced, complained := 0, 0

	for _, msg := range msgs {
		raw := msg.FindBodySection(bodySection)

		markFailed := false

		for _, ev := range parseBounceEvents(raw) {
			var markErr error

			switch ev.kind {
			case "bounced":
				markErr = q.MarkBounced(ctx, ev.email)
			case "complained":
				markErr = q.MarkComplained(ctx, ev.email)
			default:
				continue
			}

			if markErr != nil {
				slog.Warn("task:newsletter:poll_bounces: mark failed", "email", ev.email, "kind", ev.kind, "error", markErr)
				markFailed = true
				continue
			}

			if ev.kind == "bounced" {
				bounced++
			} else {
				complained++
			}
		}

		// Messages are flagged \Seen once fully handled: unparseable mail
		// is marked seen so a poison message can't re-enter the poll
		// forever, but a message whose suppression hit a transient DB
		// error stays UNSEEN so the next poll retries the mark instead
		// of dropping the bounce event.
		if !markFailed {
			processed = append(processed, msg.SeqNum)
		}
	}

	if len(processed) > 0 {
		if err := client.Store(imap.SeqSetNum(processed...), &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Silent: true,
			Flags:  []imap.Flag{imap.FlagSeen},
		}, nil).Close(); err != nil {
			return fmt.Errorf("imap store \\Seen: %w", err)
		}
	}

	slog.Info("task:newsletter:poll_bounces: mailbox processed",
		"mailbox", cfg.mailbox, "messages", len(msgs),
		"bounced", bounced, "complained", complained)

	return nil
}

// parseBounceEvents extracts suppression events from a raw RFC 5322
// message: DSN per-recipient blocks (multipart/report +
// message/delivery-status), ARF feedback reports
// (message/feedback-report), and the X-Failed-Recipients header used by
// Postfix/Gmail-style non-DSN notices. Addresses are returned
// deduplicated — the first event kind wins when both appear.
func parseBounceEvents(raw []byte) []bounceEvent {
	if len(raw) == 0 {
		return nil
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil
	}

	events := make([]bounceEvent, 0, 4)
	seen := make(map[string]bool)

	add := func(addr, kind string) {
		addr = strings.ToLower(strings.TrimSpace(addr))
		if addr == "" || !strings.Contains(addr, "@") || seen[addr] {
			return
		}

		seen[addr] = true

		events = append(events, bounceEvent{email: addr, kind: kind})
	}

	// X-Failed-Recipients — Postfix/Gmail-style non-DSN delivery notices.
	for _, hdr := range msg.Header["X-Failed-Recipients"] {
		for _, addr := range strings.Split(hdr, ",") {
			add(strings.TrimSpace(addr), "bounced")
		}
	}

	mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(msg.Body, params["boundary"])

		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}

			partType := part.Header.Get("Content-Type")

			switch {
			case strings.HasPrefix(partType, "message/delivery-status"):
				for _, ev := range parseDeliveryStatus(part) {
					add(ev.email, ev.kind)
				}
			case strings.HasPrefix(partType, "message/feedback-report"):
				for _, ev := range parseFeedbackReport(part) {
					add(ev.email, ev.kind)
				}
			}
		}
	}

	return events
}

// parseDeliveryStatus walks the per-recipient blocks of a
// message/delivery-status part (RFC 3464): the first header block is
// per-message; each subsequent block carries Final-Recipient, Action and
// Status. Action "failed" or a 5.x Status means permanent failure —
// 'bounced'. Transient (4.x, delayed, expanded, relayed, delivered) is
// ignored: suppressing on a soft bounce would kill deliverable addresses.
func parseDeliveryStatus(part *multipart.Part) []bounceEvent {
	data, err := io.ReadAll(io.LimitReader(part, 1<<20))
	if err != nil {
		return nil
	}

	// Blocks are separated by a blank line in either CRLF or LF form.
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	blocks := strings.Split(normalized, "\n\n")

	var events []bounceEvent

	seenFirst := false // first non-empty block is per-message (Reporting-MTA etc.)

	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		if !seenFirst {
			seenFirst = true
			continue
		}

		sub, err := mail.ReadMessage(strings.NewReader(block))
		if err != nil {
			continue
		}

		// Final-Recipient carries the bounced address; some MTAs emit only
		// Original-Recipient. Both use the "rfc822;addr" form.
		final := sub.Header.Get("Final-Recipient")
		if final == "" {
			final = sub.Header.Get("Original-Recipient")
		}

		action := strings.ToLower(strings.TrimSpace(sub.Header.Get("Action")))
		status := strings.TrimSpace(sub.Header.Get("Status"))

		// Final-Recipient format: "rfc822;user@example.com"
		addr := final
		if idx := strings.Index(final, ";"); idx >= 0 {
			addr = final[idx+1:]
		}

		addr = strings.TrimSpace(addr)

		if action == "failed" || strings.HasPrefix(status, "5") {
			events = append(events, bounceEvent{email: addr, kind: "bounced"})
		}
	}

	return events
}

// parseFeedbackReport extracts the complained-about recipient from an
// ARF (message/feedback-report, RFC 6449) part. Only Feedback-Type
// "abuse" suppresses (§62): fraud/virus reports concern message content,
// not the recipient's subscription — suppressing them would drop a
// victim address that never opted out. Original-Rcpt-To carries the
// original recipient in CFBL reports.
func parseFeedbackReport(part *multipart.Part) []bounceEvent {
	sub, err := mail.ReadMessage(part)
	if err != nil {
		return nil
	}

	fbType := strings.ToLower(strings.TrimSpace(sub.Header.Get("Feedback-Type")))
	if fbType != "abuse" {
		return nil
	}

	addr := strings.TrimSpace(sub.Header.Get("Original-Rcpt-To"))
	if addr == "" {
		// Some FBLs omit Original-Rcpt-To; User-Agent-style generators put
		// it in Original-Recipient or leave only the embedded original —
		// in which case suppression can't be attributed safely.
		addr = strings.TrimSpace(sub.Header.Get("Original-Recipient"))
	}

	if addr == "" {
		return nil
	}

	return []bounceEvent{{email: addr, kind: "complained"}}
}
