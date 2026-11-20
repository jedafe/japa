// Package worker — support_inbox.go polls the support IMAP mailbox and
// converts inbound mail to tickets (§38 Inbound Email Bridge, §11 task
// support:poll_inbox). Users replying to a ticket confirmation thread back
// onto the ticket when their From address matches a registered account;
// everything else becomes a new ticket under the configured type. Auto-
// submitted mail (bounces, auto-replies, list traffic) is skipped so the
// bridge can't loop with the platform's own outbound mail.
//
// Env: JC_SUPPORT_IMAP_HOST (unset = no-op), JC_SUPPORT_IMAP_PORT (993),
// JC_SUPPORT_IMAP_USER, JC_SUPPORT_IMAP_PASS, JC_SUPPORT_IMAP_MAILBOX
// (INBOX), JC_SUPPORT_IMAP_TLS (true — false refused in production),
// JC_SUPPORT_IMAP_BATCH (200), JC_SUPPORT_TICKET_TYPE (slug,
// "demo-ticket-type-general" — the seeded General Support type).
//
// Ref: japaconnect-engineering/sections/38-ticket-system.md
package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// supportIMAPConfig carries the support-mailbox connection + filing
// settings.
type supportIMAPConfig struct {
	host       string
	port       string
	user       string
	pass       string
	mailbox    string
	tls        bool
	batch      int
	ticketType string
}

// readSupportIMAPConfig reads support-mailbox settings from
// JC_SUPPORT_IMAP_* env vars. Returns nil when the host is unset
// (feature off).
func readSupportIMAPConfig() *supportIMAPConfig {
	host := strings.TrimSpace(os.Getenv("JC_SUPPORT_IMAP_HOST"))
	if host == "" {
		return nil
	}

	port := strings.TrimSpace(os.Getenv("JC_SUPPORT_IMAP_PORT"))
	if port == "" {
		port = "993"
	}

	mailbox := strings.TrimSpace(os.Getenv("JC_SUPPORT_IMAP_MAILBOX"))
	if mailbox == "" {
		mailbox = "INBOX"
	}

	tls := !strings.EqualFold(os.Getenv("JC_SUPPORT_IMAP_TLS"), "false")

	batch := 200
	if v := strings.TrimSpace(os.Getenv("JC_SUPPORT_IMAP_BATCH")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			batch = n
		}
	}

	ticketType := strings.TrimSpace(os.Getenv("JC_SUPPORT_TICKET_TYPE"))
	if ticketType == "" {
		ticketType = "demo-ticket-type-general" // seeded "General Support"
	}

	return &supportIMAPConfig{
		host:       host,
		port:       port,
		user:       os.Getenv("JC_SUPPORT_IMAP_USER"),
		pass:       os.Getenv("JC_SUPPORT_IMAP_PASS"),
		mailbox:    mailbox,
		tls:        tls,
		batch:      batch,
		ticketType: ticketType,
	}
}

// ticketRefRe matches the human ticket reference (JC-1042) carried in
// confirmation-mail subjects and user replies.
var ticketRefRe = regexp.MustCompile(`(?i)\bJC-(\d+)\b`)

// SupportTicketInput carries one inbound message to file as a ticket.
type SupportTicketInput struct {
	TypeID          pgtype.UUID
	Subject         string
	Description     string
	SubmitterUserID pgtype.UUID
	SubmitterEmail  string
}

// SupportTicketBridge adapts the tickets service for this poller. The
// server bootstrap registers it via RegisterSupportTicketBridge — worker
// cannot import internal/tickets (import cycle through internal/auth,
// same constraint as the submission linker).
type SupportTicketBridge interface {
	// TicketTypeID resolves a ticket_type slug to its ID.
	TicketTypeID(ctx context.Context, slug string) (pgtype.UUID, error)
	// FileTicket creates a ticket from an inbound message.
	FileTicket(ctx context.Context, in SupportTicketInput) error
	// FileComment appends a public comment to the ticket a JC-<n>
	// reference names. Errors mean the comment could not be stored.
	FileComment(ctx context.Context, ref string, userID pgtype.UUID, body string) error
}

var supportTicketBridge SupportTicketBridge

// RegisterSupportTicketBridge wires the tickets service into the support
// poller. Called once at bootstrap; nil-safe (poll errors when unset).
func RegisterSupportTicketBridge(b SupportTicketBridge) {
	supportTicketBridge = b
}

// HandleSupportPollInbox polls the support IMAP mailbox and files inbound
// mail as tickets (§38 Inbound Email Bridge). Scheduled every 5 min on the
// low tier; singleton across replicas via jc:support:inbox_poll_lock.
// No-ops cleanly when JC_SUPPORT_IMAP_HOST is unset.
func HandleSupportPollInbox(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:support:poll_inbox")

	if dbPool == nil {
		slog.Warn("task:support:poll_inbox: no database, skipping")
		return nil
	}

	// Honor the feature.tickets kill-switch — no ticket creation while the
	// ticket surface is disabled.
	if featureChecker != nil && !featureChecker.IsEnabled(ctx, "feature.tickets") {
		slog.Info("task:support:poll_inbox: feature.tickets disabled, skipping")
		return nil
	}

	cfg := readSupportIMAPConfig()
	if cfg == nil {
		slog.Debug("task:support:poll_inbox: JC_SUPPORT_IMAP_HOST unset, skipping")
		return nil
	}

	// Singleton lock — same rationale as the bounce poller: the scheduler
	// fires on every replica, only one should hold the IMAP session. The
	// 10m TTL is crash-safety only — the lock is released on completion,
	// otherwise it would outlive the 5m cron and self-skip every other
	// tick.
	if rdb != nil {
		taskID, _ := asynq.GetTaskID(ctx)
		if taskID == "" {
			taskID = "poll"
		}

		ok, err := rdb.SetNX(ctx, "jc:support:inbox_poll_lock", taskID, 10*time.Minute).Result()
		if err != nil {
			slog.Warn("task:support:poll_inbox: lock check failed, proceeding unlocked", "error", err)
		} else if !ok {
			slog.Info("task:support:poll_inbox: another replica holds the lock, skipping")
			return nil
		} else {
			defer func() {
				// Release only if we still own the lock — a poll running
				// past the 10m TTL may have already been superseded.
				bctx := context.Background()
				if v, gerr := rdb.Get(bctx, "jc:support:inbox_poll_lock").Result(); gerr == nil && v == taskID {
					if _, err := rdb.Del(bctx, "jc:support:inbox_poll_lock").Result(); err != nil {
						slog.Debug("task:support:poll_inbox: lock release failed", "error", err)
					}
				}
			}()
		}
	}

	if err := pollSupportMailbox(ctx, cfg); err != nil {
		return fmt.Errorf("task:support:poll_inbox: %w", err)
	}

	return nil
}

// pollSupportMailbox runs one IMAP session: select, search UNSEEN, fetch
// up to cfg.batch messages, file each as a ticket (or a comment on the
// referenced ticket), flag processed messages \Seen.
func pollSupportMailbox(ctx context.Context, cfg *supportIMAPConfig) error {
	if cfg.user == "" || cfg.pass == "" {
		return errors.New("imap: JC_SUPPORT_IMAP_USER and JC_SUPPORT_IMAP_PASS are required when HOST is set")
	}

	addr := cfg.host + ":" + cfg.port

	var client *imapclient.Client

	var err error

	if cfg.tls {
		client, err = imapclient.DialTLS(addr, nil)
	} else {
		if os.Getenv("JC_ENV") == "production" {
			return errors.New("JC_SUPPORT_IMAP_TLS=false refused in production")
		}

		client, err = imapclient.DialInsecure(addr, nil)
	}

	if err != nil {
		return fmt.Errorf("imap dial %s: %w", addr, err)
	}

	defer func() {
		if err := client.Logout().Wait(); err != nil {
			slog.Debug("task:support:poll_inbox: imap logout failed", "error", err)
		}

		if err := client.Close(); err != nil {
			slog.Warn("task:support:poll_inbox: client close failed", "error", err)
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

	bridge := supportTicketBridge
	if bridge == nil {
		return errors.New("support ticket bridge not registered")
	}

	// The filing type is resolved once per run — a missing/misconfigured
	// slug aborts the poll so messages stay UNSEEN for the next run rather
	// than being dropped or misfiled.
	typeID, err := bridge.TicketTypeID(ctx, cfg.ticketType)
	if err != nil {
		return fmt.Errorf("resolving ticket type %q: %w", cfg.ticketType, err)
	}

	bodySection := &imap.FetchItemBodySection{
		Specifier: imap.PartSpecifierNone,
		Partial:   &imap.SectionPartial{Size: 1 << 20},
		Peek:      true,
	}

	fetchCmd := client.Fetch(imap.SeqSetNum(seqNums...), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{bodySection},
	})

	processed := make([]uint32, 0, len(seqNums))
	filed, commented, skipped, fetched := 0, 0, 0, 0

	for {
		msgData := fetchCmd.Next()
		if msgData == nil {
			break
		}

		fetched++

		var raw []byte

		for {
			item := msgData.Next()
			if item == nil {
				break
			}

			bs, ok := item.(imapclient.FetchItemDataBodySection)
			if !ok || bs.Literal == nil || raw != nil {
				continue
			}

			if b, err := io.ReadAll(bs.Literal); err == nil {
				raw = b
			}
		}

		if len(raw) == 0 {
			// Body literal not returned — leave UNSEEN for the next poll.
			continue
		}

		outcome, fileErr := fileSupportMessage(ctx, bridge, typeID, cfg.user, raw)
		switch {
		case fileErr != nil:
			slog.Warn("task:support:poll_inbox: filing failed — message stays UNSEEN",
				"seq", msgData.SeqNum, "error", fileErr)
			continue
		case outcome == fileSkip:
			skipped++
		case outcome == fileComment:
			commented++
		default:
			filed++
		}

		processed = append(processed, msgData.SeqNum)
	}

	if err := fetchCmd.Close(); err != nil {
		return fmt.Errorf("imap fetch: %w", err)
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

	slog.Info("task:support:poll_inbox: mailbox processed",
		"mailbox", cfg.mailbox, "messages", fetched,
		"tickets", filed, "comments", commented, "skipped", skipped)

	return nil
}

// fileOutcome describes how a message was handled.
type fileOutcome int

const (
	fileTicket  fileOutcome = iota // new ticket created
	fileComment                    // comment appended to an existing ticket
	fileSkip                       // seen, not filed (auto-submitted / malformed)
)

// fileSupportMessage converts one raw message into a ticket or comment.
// selfAddr is the polled mailbox address — mail From it is skipped so the
// bridge can't loop on its own notifications. A non-nil error means
// transient failure — the caller leaves the message UNSEEN so the next
// poll retries.
func fileSupportMessage(ctx context.Context, bridge SupportTicketBridge, typeID pgtype.UUID, selfAddr string, raw []byte) (fileOutcome, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return fileSkip, nil
	}

	if isAutoSubmitted(msg) {
		return fileSkip, nil
	}

	from := senderAddress(msg)
	if from == "" {
		return fileSkip, nil
	}

	if selfAddr != "" && strings.EqualFold(from, selfAddr) {
		return fileSkip, nil
	}

	// Message-ID dedup (jc:support:filed:*) — covers the window where
	// filing succeeded but the \Seen store failed and the next poll
	// re-fetches the message. On filing error the marker is removed so a
	// genuinely unfiled message can retry.
	dedupKey := ""
	if msgID := strings.TrimSpace(msg.Header.Get("Message-ID")); msgID != "" && rdb != nil {
		sum := sha256.Sum256([]byte(msgID))
		dedupKey = "jc:support:filed:" + hex.EncodeToString(sum[:16])
		if ok, derr := rdb.SetNX(ctx, dedupKey, "1", 7*24*time.Hour).Result(); derr == nil && !ok {
			return fileSkip, nil
		}
	}

	subject := decodeHeader(msg.Header.Get("Subject"))
	if subject == "" {
		subject = "(no subject)"
	}

	body := extractPlainText(msg)
	if strings.TrimSpace(body) == "" {
		body = "(empty message)"
	}

	// Resolve the sender to a platform account when possible.
	var submitterID pgtype.UUID
	if dbPool != nil {
		if u, uerr := gen.New(dbPool.Reader).GetUserByEmail(ctx, strings.ToLower(from)); uerr == nil {
			submitterID = u.ID
		}
	}

	// Reply threading: a JC-<n> subject from a known user becomes a public
	// comment on that ticket instead of a new ticket.
	if submitterID.Valid {
		if m := ticketRefRe.FindStringSubmatch(subject); m != nil {
			if cerr := bridge.FileComment(ctx, "JC-"+m[1], submitterID, body); cerr == nil {
				return fileComment, nil
			}
		}
	}

	if terr := bridge.FileTicket(ctx, SupportTicketInput{
		TypeID:          typeID,
		Subject:         subject,
		Description:     body,
		SubmitterUserID: submitterID,
		SubmitterEmail:  from,
	}); terr != nil {
		if dedupKey != "" {
			rdb.Del(ctx, dedupKey)
		}
		return fileTicket, fmt.Errorf("create ticket: %w", terr)
	}

	return fileTicket, nil
}

// isAutoSubmitted reports whether a message is machine-generated and must
// never become a ticket: DSN bounces (empty Return-Path, MAILER-DAEMON),
// auto-replies (Auto-Submitted ≠ no, X-Autorespond), and bulk/list traffic
// (Precedence header).
func isAutoSubmitted(msg *mail.Message) bool {
	h := msg.Header

	if v := h.Get("Auto-Submitted"); v != "" && !strings.EqualFold(v, "no") {
		return true
	}

	for _, hdr := range []string{"X-Autorespond", "X-Autoreply", "X-Auto-Response-Suppress", "List-Id"} {
		if h.Get(hdr) != "" {
			return true
		}
	}

	if p := strings.ToLower(h.Get("Precedence")); p == "bulk" || p == "list" || p == "junk" || p == "auto_reply" {
		return true
	}

	if strings.TrimSpace(h.Get("Return-Path")) == "<>" {
		return true
	}

	from := senderAddress(msg)
	local := strings.ToLower(strings.SplitN(from, "@", 2)[0])
	switch local {
	case "mailer-daemon", "postmaster", "noreply", "no-reply":
		return true
	}

	return false
}

// senderAddress extracts the bare address from the From header (Reply-To
// takes precedence when From is missing).
func senderAddress(msg *mail.Message) string {
	for _, hdr := range []string{"From", "Reply-To"} {
		if a, err := mail.ParseAddress(msg.Header.Get(hdr)); err == nil && a.Address != "" {
			return a.Address
		}
	}
	return ""
}

// decodeHeader unwraps RFC 2047 encoded words (=?UTF-8?Q?...?=) in headers.
func decodeHeader(v string) string {
	if v == "" {
		return ""
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(v)
	if err != nil {
		return v
	}
	return decoded
}

// extractPlainText pulls the body as text — first text/plain part wins;
// text/html-only messages are tag-stripped. Handles the common transfer
// encodings (base64, quoted-printable, 7bit/8bit/binary).
func extractPlainText(msg *mail.Message) string {
	mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))

	if strings.HasPrefix(mediaType, "multipart/") {
		return walkBody(msg.Body, params["boundary"], 0)
	}

	data := decodeTransfer(msg.Body, msg.Header.Get("Content-Transfer-Encoding"))
	if mediaType == "text/html" || mediaType == "" {
		return htmlToText(string(data))
	}
	return string(data)
}

// walkBody recurses a multipart body (depth-capped) preferring text/plain.
func walkBody(r io.Reader, boundary string, depth int) string {
	if boundary == "" || depth >= 8 {
		return ""
	}

	mr := multipart.NewReader(r, boundary)

	var htmlPart []byte

	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}

		partType, partParams, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		data := decodeTransfer(part, part.Header.Get("Content-Transfer-Encoding"))

		switch {
		case strings.HasPrefix(partType, "multipart/"):
			if s := walkBody(bytes.NewReader(data), partParams["boundary"], depth+1); s != "" {
				return s
			}
		case partType == "text/plain" || partType == "":
			return string(data)
		case partType == "text/html" && htmlPart == nil:
			htmlPart = data
		}
	}

	if htmlPart != nil {
		return htmlToText(string(htmlPart))
	}
	return ""
}

// decodeTransfer reads a part body honoring Content-Transfer-Encoding.
func decodeTransfer(r io.Reader, cte string) []byte {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		if data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, r)); err == nil {
			return data
		}
	case "quoted-printable":
		if data, err := io.ReadAll(quotedprintable.NewReader(r)); err == nil {
			return data
		}
	}

	data, _ := io.ReadAll(io.LimitReader(r, 1<<20))
	return data
}

// tagRe strips HTML tags for html-only bodies.
var tagRe = regexp.MustCompile(`<[^>]*>`)

// wsRe collapses whitespace runs left by tag stripping.
var wsRe = regexp.MustCompile(`[ \t]+|\n{3,}`)

// htmlToText produces a rough plain-text rendering of an HTML body —
// line breaks for block tags, tags stripped, entities unescaped.
func htmlToText(s string) string {
	for _, tag := range []string{"br", "/p", "/div", "/li", "/tr", "/h1", "/h2", "/h3", "hr"} {
		s = strings.ReplaceAll(s, "<"+tag+">", "\n")
		s = strings.ReplaceAll(s, "<"+tag+" />", "\n")
	}
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = wsRe.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, "\n") {
			return "\n\n"
		}
		return " "
	})
	return strings.TrimSpace(s)
}
