package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// TaskIntelDetectChanges is the Asynq task name enqueued after an inbound
// (WebSub push) delivery is materialized into a raw document — the same
// hand-off the fetch path uses (intel-channels Phase 3).
const TaskIntelDetectChanges = "intel:detect_changes"

// TaskIntelInboundEmail is the Asynq task name the email inbound endpoint
// enqueues; the worker task MIME-parses the stored event into a raw doc.
const TaskIntelInboundEmail = "intel:inbound_email"

// TaskIntelWebSubSubscribe is the Asynq task name admin channel-create
// enqueues so a webhook channel's hub subscription starts immediately.
// Mirrors worker.TypeIntelWebSubSubscribe without importing the worker
// package (import cycle).
const TaskIntelWebSubSubscribe = "intel:websub_subscribe"

const (
	// inboundSigSkew bounds the delivery timestamp for replay protection.
	inboundSigSkew = 5 * time.Minute
	// inboundMaxBody caps push/email payloads accepted by the endpoints.
	inboundMaxBody = 8 << 20 // 8 MiB
	// inboundIdemTTL is the dedupe window for jc:intel:inbound:idem:{hash}.
	inboundIdemTTL = 24 * time.Hour
)

// ---------------------------------------------------------------------------
// Email inbound (intel-channels Phase 3, spec §3/§5)
// ---------------------------------------------------------------------------

// HandleInboundEmail is the provider-facing inbound webhook
// (POST /api/v1/intel/inbound/email). Deliveries are verified with the
// Mailgun-compatible signature triple: signature =
// hex(HMAC-SHA256(signing_key, timestamp + token)), timestamp within ±5m.
// The signing key comes from intel.email.signing_key (AES-256-GCM at rest —
// the payment-secret pattern); an unset key disables the endpoint (503).
//
// Accepted payloads:
//   - application/x-www-form-urlencoded / multipart/form-data (Mailgun
//     forward-style posts: sender/from, recipient, subject, body-mime or
//     stripped-text + stripped-html, Message-Id)
//   - message/rfc822 raw MIME with the signature triple in
//     X-Intel-Email-{Timestamp,Token,Signature} headers
//
// The recipient resolves to the enabled email channel via
// provider_config.inbound_address; the event is deduped on
// jc:intel:inbound:idem:{hash}, stored as an intel_inbound_events row, and
// intel:inbound_email is enqueued — always 202 on accepted/dropped
// deliveries so providers never retry-loop.
func (h *Handlers) HandleInboundEmail(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.asynqClient == nil || h.settings == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "inbound_unavailable", "Inbound email is not configured")
		return
	}

	signingKey, err := inboundEmailSigningKey(r.Context(), h.settings)
	if err != nil || signingKey == "" {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "inbound_disabled", "Inbound email signing key is not configured")
		return
	}

	var timestamp, token, signature string
	var rawMIME []byte
	var formOK bool

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "message/rfc822") {
		rawMIME, err = io.ReadAll(http.MaxBytesReader(w, r.Body, inboundMaxBody))
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Could not read delivery body")
			return
		}
		timestamp = r.Header.Get("X-Intel-Email-Timestamp")
		token = r.Header.Get("X-Intel-Email-Token")
		signature = r.Header.Get("X-Intel-Email-Signature")
	} else {
		// Hard body cap BEFORE parsing: ParseMultipartForm's argument is a
		// memory ceiling — oversized file parts would stream to disk
		// unbounded without MaxBytesReader on the body itself.
		r.Body = http.MaxBytesReader(w, r.Body, inboundMaxBody)
		if err := r.ParseMultipartForm(inboundMaxBody); err != nil {
			if perr := r.ParseForm(); perr != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Could not parse delivery")
				return
			}
		}
		timestamp = r.FormValue("timestamp")
		token = r.FormValue("token")
		signature = r.FormValue("signature")
		formOK = true
	}

	if !verifyInboundSignature(signingKey, timestamp, token, signature) {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Delivery signature verification failed")
		return
	}

	// Normalize the delivery to the shared email envelope. Form posts
	// (Mailgun) carry parsed fields; raw MIME carries the message bytes.
	var payload []byte
	var sender, recipient, subject, messageID, bodyText, bodyHTML string
	var eventCT string

	if formOK {
		sender = firstNonEmpty(r.FormValue("sender"), r.FormValue("from"), r.FormValue("From"))
		recipient = firstNonEmpty(r.FormValue("recipient"), r.FormValue("to"), r.FormValue("To"))
		subject = r.FormValue("subject")
		messageID = firstNonEmpty(r.FormValue("Message-Id"), r.FormValue("message-id"))
		bodyText = firstNonEmpty(r.FormValue("stripped-text"), r.FormValue("body-plain"), r.FormValue("text"))
		bodyHTML = firstNonEmpty(r.FormValue("stripped-html"), r.FormValue("body-html"))

		if mimeBody := r.FormValue("body-mime"); mimeBody != "" {
			// Mailgun's forward webhook can post the complete RFC822
			// message in body-mime — hand it to the task's MIME parser
			// instead of the JSON envelope so no content is lost. When the
			// form omits routing fields, fall back to the MIME headers.
			payload = []byte(mimeBody)
			eventCT = "message/rfc822"
			if msg, merr := inboundMIMEHeaders(payload); merr == nil {
				if sender == "" {
					sender = msg.Get("From")
				}
				if recipient == "" {
					recipient = msg.Get("To")
				}
				if subject == "" {
					subject = msg.Get("Subject")
				}
				if messageID == "" {
					messageID = msg.Get("Message-Id")
				}
			}
		} else {
			payload, _ = json.Marshal(map[string]string{
				"envelope":   "form",
				"from":       sender,
				"to":         recipient,
				"subject":    subject,
				"message_id": messageID,
				"text":       bodyText,
				"html":       bodyHTML,
			})
			eventCT = "application/x-intel-email"
		}
	} else {
		// Raw MIME: the task parses headers out of the payload; recipient
		// must come from headers here to route the channel.
		msg, merr := inboundMIMEHeaders(rawMIME)
		if merr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_mime", "Could not parse MIME message")
			return
		}
		sender = msg.Get("From")
		recipient = msg.Get("To")
		subject = msg.Get("Subject")
		messageID = msg.Get("Message-Id")
		payload = rawMIME
		eventCT = "message/rfc822"
	}

	if recipient == "" {
		errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"data": map[string]interface{}{"received": false, "reason": "missing_recipient"},
		})
		return
	}

	ch, err := h.intelSources.GetEmailChannelByAddress(r.Context(), firstAddress(recipient))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
				"data": map[string]interface{}{"received": false, "reason": "unknown_recipient"},
			})
			return
		}
		slog.Error("intel: inbound email channel lookup failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "inbound_failed", "Delivery routing failed")
		return
	}

	// Idempotency: channel + provider token (or message-id fallback) —
	// a retried delivery of the same message is a no-op. The claim sits
	// before the sender gate so retried *rejections* dedupe too instead
	// of piling up 'ignored' events.
	idemBasis := token
	if idemBasis == "" {
		idemBasis = messageID
	}
	if idemBasis == "" {
		sum := sha256.Sum256(payload)
		idemBasis = hex.EncodeToString(sum[:])
	}
	idemKey, dup := h.claimInbound(r, "email:"+ch.ID.String()+":"+idemBasis)
	if dup {
		errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"data": map[string]interface{}{"received": true, "duplicate": true},
		})
		return
	}

	if !inboundSenderAllowed(r.Context(), h.settings, ch.ProviderConfig, sender) {
		slog.Info("intel: inbound email rejected by sender gate", "channel_id", ch.ID.String(), "from", sender)
		// Persist an 'ignored' event so rejected deliveries leave an audit
		// trail (plan Task 1: sender gate → event 'ignored').
		rejectHeaders, _ := json.Marshal(map[string]string{
			"from":    inboundHeaderCap(sender),
			"to":      inboundHeaderCap(recipient),
			"subject": inboundHeaderCap(subject),
		})
		rejectSum := sha256.Sum256(payload)
		if ev, evErr := h.intelSources.CreateInboundEvent(r.Context(), gen.CreateIntelInboundEventParams{
			ChannelID:   ch.ID,
			Payload:     payload,
			Headers:     rejectHeaders,
			ContentType: strPtrInbound(eventCT),
			ContentHash: hex.EncodeToString(rejectSum[:]),
		}); evErr == nil {
			_ = h.intelSources.MarkInboundEvent(r.Context(), gen.MarkIntelInboundEventParams{ID: ev.ID, Status: "ignored"})
		}
		errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"data": map[string]interface{}{"received": false, "reason": "sender_not_allowed"},
		})
		return
	}

	headersJSON, _ := json.Marshal(map[string]string{
		"from":       inboundHeaderCap(sender),
		"to":         inboundHeaderCap(recipient),
		"subject":    inboundHeaderCap(subject),
		"message_id": inboundHeaderCap(messageID),
	})
	contentHash := sha256.Sum256(payload)

	event, err := h.intelSources.CreateInboundEvent(r.Context(), gen.CreateIntelInboundEventParams{
		ChannelID:   ch.ID,
		Payload:     payload,
		Headers:     headersJSON,
		ContentType: strPtrInbound(eventCT),
		ContentHash: hex.EncodeToString(contentHash[:]),
	})
	if err != nil {
		h.releaseInbound(r, idemKey)
		slog.Error("intel: inbound email event persist failed", "channel_id", ch.ID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "inbound_failed", "Could not record the delivery")
		return
	}

	taskPayload, _ := json.Marshal(map[string]string{
		"version":  "v1",
		"event_id": event.ID.String(),
	})
	if _, err := h.asynqClient.EnqueueContext(r.Context(), asynq.NewTask(TaskIntelInboundEmail, taskPayload),
		worker.TaskOpts(TaskIntelInboundEmail)...); err != nil {
		// Free the dedupe slot so the provider's redelivery is not a silent
		// duplicate, and fail the orphan event row — a redelivery creates
		// a fresh event rather than resurrecting this one.
		_ = h.intelSources.MarkInboundEvent(r.Context(), gen.MarkIntelInboundEventParams{ID: event.ID, Status: "failed"})
		h.releaseInbound(r, idemKey)
		slog.Error("intel: inbound email enqueue failed", "event_id", event.ID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Could not queue delivery processing")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"data": map[string]interface{}{"received": true, "event_id": event.ID.String()},
	})
}

// ---------------------------------------------------------------------------
// WebSub verify + push (intel-channels Phase 3, spec §3/§5)
// ---------------------------------------------------------------------------

// webSubLease is the subscription state kept at
// jc:intel:websub:lease:{channel_id} for the channel's lease lifetime.
type webSubLease struct {
	Hub       string `json:"hub"`
	Topic     string `json:"topic"`
	ExpiresAt string `json:"expires_at"`
}

// HandleWebSubVerify answers the hub's subscription verification
// (GET /api/v1/intel/inbound/websub/{channel_id}): the topic must match the
// channel's provider_config.topic; subscribe stores the lease,
// unsubscribe drops it, and hub.challenge is echoed verbatim.
func (h *Handlers) HandleWebSubVerify(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "inbound_unavailable", "WebSub is not configured")
		return
	}

	ch, ok := h.webSubChannel(w, r)
	if !ok {
		return
	}

	cfg := channelProviderConfig(ch.ProviderConfig)
	topic := r.URL.Query().Get("hub.topic")
	if topic == "" || topic != inboundCfgString(cfg, "topic") {
		errpkg.WriteError(w, http.StatusNotFound, "topic_mismatch", "hub.topic does not match this channel")
		return
	}

	mode := r.URL.Query().Get("hub.mode")
	challenge := r.URL.Query().Get("hub.challenge")
	if challenge == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_challenge", "hub.challenge is required")
		return
	}

	leaseKey := "jc:intel:websub:lease:" + ch.ID.String()
	switch mode {
	case "subscribe":
		leaseSecs, _ := strconv.ParseInt(r.URL.Query().Get("hub.lease_seconds"), 10, 64)
		if leaseSecs <= 0 {
			leaseSecs = 864000 // hub-omitted lease: treat as the common 10-day term
		}
		lease := webSubLease{
			Hub:       inboundCfgString(cfg, "hub"),
			Topic:     topic,
			ExpiresAt: time.Now().Add(time.Duration(leaseSecs) * time.Second).UTC().Format(time.RFC3339),
		}
		if h.rdb != nil {
			if leaseJSON, err := json.Marshal(lease); err == nil {
				_ = h.rdb.Set(r.Context(), leaseKey, leaseJSON, time.Duration(leaseSecs)*time.Second).Err()
			}
		}
	case "unsubscribe", "denied":
		// denied = the hub refused our subscription request; either way
		// any recorded lease is stale. The challenge still echoes so the
		// hub gets a well-formed verification response.
		if h.rdb != nil {
			_ = h.rdb.Del(r.Context(), leaseKey).Err()
		}
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_mode", "hub.mode must be subscribe or unsubscribe")
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, challenge)
}

// HandleWebSubPush accepts a hub content delivery
// (POST /api/v1/intel/inbound/websub/{channel_id}). When the channel's
// provider_config.secret is set, X-Hub-Signature must carry a valid
// sha256= HMAC over the body. Accepted bodies are deduped, persisted as an
// inbound event, materialized straight into a raw document, and handed to
// intel:detect_changes — inline because no external calls are needed.
func (h *Handlers) HandleWebSubPush(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.intelStores == nil || h.asynqClient == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "inbound_unavailable", "WebSub is not configured")
		return
	}

	ch, ok := h.webSubChannel(w, r)
	if !ok {
		return
	}
	cfg := channelProviderConfig(ch.ProviderConfig)

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, inboundMaxBody))
	if err != nil || len(body) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Could not read delivery body")
		return
	}

	secret := inboundCfgString(cfg, "secret")
	if secret != "" {
		sig := r.Header.Get("X-Hub-Signature")
		if !verifyWebSubSignature(secret, sig, body) {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Delivery signature verification failed")
			return
		}
	}

	deliveryHash := sha256.Sum256(body)
	idemKey, dup := h.claimInbound(r, "websub:"+ch.ID.String()+":"+hex.EncodeToString(deliveryHash[:]))
	if dup {
		errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"data": map[string]interface{}{"received": true, "duplicate": true},
		})
		return
	}

	ct := r.Header.Get("Content-Type")
	contentHash := deliveryHash
	headersJSON, _ := json.Marshal(map[string]string{
		"link":         inboundHeaderCap(r.Header.Get("Link")),
		"content_type": inboundHeaderCap(ct),
	})

	event, err := h.intelSources.CreateInboundEvent(r.Context(), gen.CreateIntelInboundEventParams{
		ChannelID:   ch.ID,
		Payload:     body,
		Headers:     headersJSON,
		ContentType: strPtrInbound(ct),
		ContentHash: hex.EncodeToString(contentHash[:]),
	})
	if err != nil {
		h.releaseInbound(r, idemKey)
		slog.Error("intel: websub event persist failed", "channel_id", ch.ID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "inbound_failed", "Could not record the delivery")
		return
	}

	mark := func(status string) {
		_ = h.intelSources.MarkInboundEvent(r.Context(), gen.MarkIntelInboundEventParams{ID: event.ID, Status: status})
	}

	src, err := h.intelSources.Get(r.Context(), ch.SourceID)
	if err != nil {
		mark("failed")
		h.releaseInbound(r, idemKey)
		slog.Error("intel: websub source lookup failed", "channel_id", ch.ID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "inbound_failed", "Could not load delivery source")
		return
	}

	// Prior hash BEFORE insert — detect_changes diffs against the previous
	// delivery, not the doc we just persisted.
	priorHash, _ := h.intelStores.GetLatestIntelRawDocHashForChannel(r.Context(), ch.ID)

	docURL := "websub:" + ch.ID.String() + ":" + hex.EncodeToString(contentHash[:8])
	rawDoc, err := h.intelStores.CreateIntelRawDocument(r.Context(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         docURL,
		HttpStatus:  200,
		HttpHeaders: headersJSON,
		ContentType: strPtrInbound(ct),
		Body:        body,
		ContentHash: hex.EncodeToString(contentHash[:]),
	})
	if err != nil {
		mark("failed")
		h.releaseInbound(r, idemKey)
		slog.Error("intel: websub raw doc persist failed", "channel_id", ch.ID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "inbound_failed", "Could not persist the delivery")
		return
	}

	taskPayload, _ := json.Marshal(map[string]string{
		"version":    "v1",
		"source_id":  ch.SourceID.String(),
		"channel_id": ch.ID.String(),
		"raw_doc_id": rawDoc.ID.String(),
		"prior_hash": priorHash,
	})
	if _, err := h.asynqClient.EnqueueContext(r.Context(), asynq.NewTask(TaskIntelDetectChanges, taskPayload),
		worker.TaskOpts(TaskIntelDetectChanges)...); err != nil {
		mark("failed")
		h.releaseInbound(r, idemKey)
		slog.Error("intel: websub detect_changes enqueue failed", "raw_doc_id", rawDoc.ID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Could not queue delivery processing")
		return
	}

	mark("processed")
	errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"data": map[string]interface{}{"received": true, "raw_doc_id": rawDoc.ID.String()},
	})
}

// ---------------------------------------------------------------------------
// Shared inbound helpers
// ---------------------------------------------------------------------------

// webSubChannel resolves {channel_id} and enforces the webhook-method gate
// for both websub handlers. Writes the error response on failure.
func (h *Handlers) webSubChannel(w http.ResponseWriter, r *http.Request) (gen.IntelSourceChannel, bool) {
	channelID, ok := scanUUIDParam(w, r, "channel_id")
	if !ok {
		return gen.IntelSourceChannel{}, false
	}
	ch, err := h.intelSources.GetChannel(r.Context(), channelID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return gen.IntelSourceChannel{}, false
	}
	if ch.CrawlMethod != "webhook" || !ch.Enabled {
		errpkg.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return gen.IntelSourceChannel{}, false
	}

	return ch, true
}

// claimInbound claims the jc:intel:inbound:idem:{hash} slot for this
// delivery — dup=true means a retry of an already-accepted delivery. A nil
// or erroring Valkey fails open (returns an empty key): dedup on
// content_hash still applies downstream.
func (h *Handlers) claimInbound(r *http.Request, basis string) (key string, dup bool) {
	if h.rdb == nil {
		return "", false
	}
	sum := sha256.Sum256([]byte(basis))
	key = "jc:intel:inbound:idem:" + hex.EncodeToString(sum[:])
	set, err := h.rdb.SetNX(r.Context(), key, "1", inboundIdemTTL).Result()
	if err != nil {
		slog.Warn("intel: inbound idempotency check failed; accepting", "error", err)
		return "", false
	}

	return key, !set
}

// releaseInbound frees a claimed idempotency slot after a post-claim
// failure so a provider redelivery is not silently deduped into a dead end
// (e.g. enqueue failed → 500 → provider retries → must reach the queue).
func (h *Handlers) releaseInbound(r *http.Request, key string) {
	if h.rdb == nil || key == "" {
		return
	}
	_ = h.rdb.Del(r.Context(), key).Err()
}

// inboundHeaderCap bounds individual header values persisted on the
// inbound event — headers arrive from providers and are unbounded
// otherwise (plan Task 1: cap values at 2 KB).
func inboundHeaderCap(s string) string {
	const max = 2048
	if len(s) > max {
		return s[:max]
	}
	return s
}

// verifyInboundSignature checks the Mailgun-compatible triple:
// signature = hex(HMAC-SHA256(key, timestamp+token)), timestamp within
// ±inboundSigSkew of now (replay window).
func verifyInboundSignature(key, timestamp, token, signature string) bool {
	if key == "" || timestamp == "" || token == "" || signature == "" {
		return false
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if skew := time.Since(time.Unix(ts, 0)); skew > inboundSigSkew || skew < -inboundSigSkew {
		return false
	}

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(timestamp))
	mac.Write([]byte(token))
	want := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(strings.ToLower(signature)), []byte(want))
}

// verifyWebSubSignature checks X-Hub-Signature "sha256=<hex>" as
// HMAC-SHA256(secret, body).
func verifyWebSubSignature(secret, sigHeader string, body []byte) bool {
	sig, ok := strings.CutPrefix(strings.TrimSpace(sigHeader), "sha256=")
	if !ok || sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	return hmac.Equal([]byte(strings.ToLower(sig)), []byte(hex.EncodeToString(mac.Sum(nil))))
}

// inboundEmailSigningKey decrypts intel.email.signing_key (the
// payment-secret pattern — stored AES-256-GCM ciphertext via
// crypto.Encrypt). A decrypt failure disables the endpoint rather than
// falling back to plaintext.
func inboundEmailSigningKey(ctx context.Context, settings SettingsReader) (string, error) {
	raw := settings.GetStringOr(ctx, "intel.email.signing_key", "")
	if raw == "" {
		return "", nil
	}

	key, err := crypto.Decrypt(raw)
	if err != nil {
		return "", fmt.Errorf("inbound: decrypt signing key: %w", err)
	}

	return key, nil
}

// inboundMIMEHeaders parses just the header block of a raw MIME message —
// enough to route the delivery (From/To/Subject/Message-Id) before the
// task decodes bodies.
func inboundMIMEHeaders(raw []byte) (mail.Header, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}

	return msg.Header, nil
}

// channelProviderConfig decodes a channel's provider_config jsonb into a
// map for inbound attribute reads.
func channelProviderConfig(raw []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

func inboundCfgString(cfg map[string]any, key string) string {
	if v, ok := cfg[key].(string); ok {
		return v
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func strPtrInbound(s string) *string { return &s }

// firstAddress extracts the bare mailbox from an address header —
// handles display names ("Name <a@b>" → "a@b") and multi-address lists
// (To: headers commonly carry several) by taking the first entry.
// Falls back to the trimmed input for non-parseable values.
func firstAddress(s string) string {
	s = strings.TrimSpace(s)
	if addrs, err := mail.ParseAddressList(s); err == nil && len(addrs) > 0 {
		return strings.ToLower(strings.TrimSpace(addrs[0].Address))
	}
	if i := strings.LastIndex(s, "<"); i >= 0 {
		s = strings.TrimPrefix(s[i:], "<")
		s = strings.TrimSuffix(s, ">")
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// inboundSenderAllowed enforces the sender gate: the channel's
// provider_config.allowed_senders list wins; absent → platform
// intel.email.allowed_senders (jsonb). Entries are exact addresses or
// *@domain globs; empty everywhere allows all senders.
func inboundSenderAllowed(ctx context.Context, settings SettingsReader, providerConfig []byte, sender string) bool {
	sender = firstAddress(sender)
	if sender == "" {
		return false
	}

	cfg := channelProviderConfig(providerConfig)
	allowed := inboundCfgStringSlice(cfg, "allowed_senders")
	if len(allowed) == 0 && settings != nil {
		raw := settings.GetStringOr(ctx, "intel.email.allowed_senders", "[]")
		var platform []string
		if err := json.Unmarshal([]byte(raw), &platform); err == nil {
			allowed = platform
		}
	}
	if len(allowed) == 0 {
		return true
	}

	at := strings.LastIndex(sender, "@")
	senderDomain := ""
	if at >= 0 {
		senderDomain = sender[at:]
	}
	for _, rule := range allowed {
		rule = strings.ToLower(strings.TrimSpace(rule))
		switch {
		case rule == "":
			continue
		case strings.HasPrefix(rule, "*@"):
			if senderDomain == rule[1:] {
				return true
			}
		case rule == sender:
			return true
		}
	}

	return false
}

func inboundCfgStringSlice(cfg map[string]any, key string) []string {
	raw, ok := cfg[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}

	return out
}
