package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
)

// ---------------------------------------------------------------------------
// Phase-3 inbound email — MIME normalization + the intel:inbound_email task.
// ---------------------------------------------------------------------------

// TestParseMIMEMessage_Plain covers a single-part text message.
func TestParseMIMEMessage_Plain(t *testing.T) {
	t.Parallel()

	raw := []byte("From: Alice <a@b.example>\r\nTo: intel@inbound.japa\r\nSubject: Visa update\r\nMessage-Id: <m1@b>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nSlots open Friday.\r\n")

	env, err := parseMIMEMessage(raw)
	if err != nil {
		t.Fatalf("parseMIMEMessage: %v", err)
	}
	if env.From != "Alice <a@b.example>" || env.Subject != "Visa update" || env.MessageID != "<m1@b>" {
		t.Errorf("envelope = %+v", env)
	}
	if !strings.Contains(env.Text, "Slots open Friday.") {
		t.Errorf("text = %q", env.Text)
	}
}

// TestParseMIMEMessage_MultipartPrefersPlain walks a multipart/alternative
// message and keeps the text/plain part over text/html.
func TestParseMIMEMessage_MultipartPrefersPlain(t *testing.T) {
	t.Parallel()

	raw := []byte(`From: a@b.example
Content-Type: multipart/alternative; boundary="BND"
Subject: Multi

--BND
Content-Type: text/html

<p>html <b>version</b></p>
--BND
Content-Type: text/plain

plain version
--BND--
`)

	env, err := parseMIMEMessage(raw)
	if err != nil {
		t.Fatalf("parseMIMEMessage: %v", err)
	}
	if !strings.Contains(env.Text, "plain version") {
		t.Errorf("text = %q, want the text/plain part", env.Text)
	}
	if strings.Contains(env.Text, "<b>") {
		t.Errorf("text leaked html part: %q", env.Text)
	}
}

// TestParseMIMEMessage_OnlyHTMLOnly strips tags when no text/plain part
// exists — the fallback the curator/LLM consumes.
func TestParseMIMEMessage_OnlyHTMLOnly(t *testing.T) {
	t.Parallel()

	raw := []byte("From: a@b\r\nContent-Type: text/html\r\n\r\n<p>Hello <b>world</b> https://x.example/a</p>\r\n")

	env, err := parseMIMEMessage(raw)
	if err != nil {
		t.Fatalf("parseMIMEMessage: %v", err)
	}
	if strings.Contains(env.Text, "<b>") || !strings.Contains(env.Text, "Hello") {
		t.Errorf("text = %q, want stripped html", env.Text)
	}
}

// TestIntelInboundEmailEnvelope_HTMLFallback covers a form delivery that
// carried only stripped-html/body-html: the worker folds the HTML into
// Text so the curator does not see an empty delivery.
func TestIntelInboundEmailEnvelope_HTMLFallback(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]string{
		"from": "a@b", "subject": "s", "html": "<p>Only <b>html</b> here</p>",
	})
	env, err := intelInboundEmailEnvelope(gen.IntelInboundEvent{
		Payload:     payload,
		ContentType: strPtr("application/x-intel-email"),
	})
	if err != nil {
		t.Fatalf("intelInboundEmailEnvelope: %v", err)
	}
	if !strings.Contains(env.Text, "Only") || strings.Contains(env.Text, "<b>") {
		t.Errorf("text = %q, want tag-stripped html", env.Text)
	}
}

// TestHandleIntelInboundEmail runs the task end-to-end against the live
// test DB: a stored form-envelope event → application/x-intel-email raw
// doc + detect_changes enqueue + event marked processed.
func TestHandleIntelInboundEmail(t *testing.T) {
	database, srcStore, _, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "inbound-email-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(p *gen.CreateIntelSourceChannelParams) {
		p.CrawlMethod = "email"
		p.Pipeline = "curate"
		p.ProviderConfig = []byte(`{"inbound_address":"intel@inbound.japa.example"}`)
	})

	env, _ := json.Marshal(map[string]string{
		"from": "alerts@embassy.example", "to": "intel@inbound.japa.example",
		"subject": "Slots open", "message_id": "<m9@embassy>", "text": "Book https://embassy.example/s/9",
	})
	event, err := srcStore.CreateInboundEvent(t.Context(), gen.CreateIntelInboundEventParams{
		ChannelID:   ch.ID,
		Payload:     env,
		Headers:     []byte(`{"from":"alerts@embassy.example"}`),
		ContentType: strPtr("application/x-intel-email"),
		ContentHash: "hash-ev-1",
	})
	if err != nil {
		t.Fatalf("create inbound event: %v", err)
	}

	payload := marshalPayload(map[string]string{"version": "v1", "event_id": event.ID.String()})
	if err := HandleIntelInboundEmail(t.Context(), asynq.NewTask(TypeIntelInboundEmail, payload)); err != nil {
		t.Fatalf("HandleIntelInboundEmail: %v", err)
	}

	if n := countChannelRawDocs(t, database, ch.ID); n != 1 {
		t.Fatalf("raw docs = %d, want 1", n)
	}

	got, err := srcStore.GetInboundEvent(t.Context(), event.ID)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got.Status != "processed" {
		t.Errorf("event status = %q, want processed", got.Status)
	}

	if tasks := pendingTasks(t, inspector, TypeIntelDetectChanges); len(tasks) == 0 {
		t.Error("no detect_changes task enqueued")
	}

	// A retry of a terminal event is a no-op — no second raw doc.
	if err := HandleIntelInboundEmail(t.Context(), asynq.NewTask(TypeIntelInboundEmail, payload)); err != nil {
		t.Fatalf("retry HandleIntelInboundEmail: %v", err)
	}
	if n := countChannelRawDocs(t, database, ch.ID); n != 1 {
		t.Errorf("raw docs after retry = %d, want 1", n)
	}

	// Terminal status is immutable: a late mark cannot flip a processed
	// event back to failed (CAS on status='received').
	_ = srcStore.MarkInboundEvent(t.Context(), gen.MarkIntelInboundEventParams{ID: event.ID, Status: "failed"})
	got2, err := srcStore.GetInboundEvent(t.Context(), event.ID)
	if err != nil {
		t.Fatalf("reload event after late mark: %v", err)
	}
	if got2.Status != "processed" {
		t.Errorf("event status after late mark = %q, want processed", got2.Status)
	}
}

// ---------------------------------------------------------------------------
// Phase-3 WebSub — the subscribe task POSTs the hub form and arms renewal.
// ---------------------------------------------------------------------------

// TestIntelWebSubSubscribe runs the subscribe task against an httptest
// hub: the form carries callback/mode/topic/secret/lease, and the
// self-renewal intel:websub_renew is armed.
func TestIntelWebSubSubscribe(t *testing.T) {
	_, srcStore, _, inspector := wireIntelChannelDeps(t)

	var gotForm url.Values
	var gotCT string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	src := mkIntelTestSource(t, srcStore, "websub-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(p *gen.CreateIntelSourceChannelParams) {
		p.CrawlMethod = "webhook"
		p.Pipeline = "curate"
		p.ProviderConfig = []byte(fmt.Sprintf(
			`{"hub":%q,"topic":"https://hub.example/feed","secret":"s3","lease_seconds":86400}`, hub.URL))
	})

	oldSite := intelSiteURL
	intelSiteURL = "https://app.japa.example"
	// The httptest hub binds 127.0.0.1 — swap the production SSRF-guarded
	// client for the AllowPrivate opt-out (same pattern as
	// fetcher.NewCompositeWithClient).
	oldClient := webSubHTTPClient
	webSubHTTPClient = fetcher.NewSafeClientOpts(30*time.Second, 1<<20, fetcher.SafeClientOpts{AllowPrivate: true})
	t.Cleanup(func() { intelSiteURL = oldSite; webSubHTTPClient = oldClient })

	payload := marshalPayload(intelWebSubPayload{Version: "v1", ChannelID: ch.ID.String()})
	if err := HandleIntelWebSubSubscribe(t.Context(), asynq.NewTask(TypeIntelWebSubSubscribe, payload)); err != nil {
		t.Fatalf("HandleIntelWebSubSubscribe: %v", err)
	}

	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q", gotCT)
	}
	if gotForm.Get("hub.mode") != "subscribe" {
		t.Errorf("hub.mode = %q", gotForm.Get("hub.mode"))
	}
	if gotForm.Get("hub.topic") != "https://hub.example/feed" {
		t.Errorf("hub.topic = %q", gotForm.Get("hub.topic"))
	}
	if gotForm.Get("hub.secret") != "s3" {
		t.Errorf("hub.secret = %q", gotForm.Get("hub.secret"))
	}
	wantCB := "https://app.japa.example/api/v1/intel/inbound/websub/" + ch.ID.String()
	if gotForm.Get("hub.callback") != wantCB {
		t.Errorf("hub.callback = %q, want %q", gotForm.Get("hub.callback"), wantCB)
	}
	if gotForm.Get("hub.lease_seconds") != "86400" {
		t.Errorf("hub.lease_seconds = %q", gotForm.Get("hub.lease_seconds"))
	}

	// Renewal armed inside the lease window — ProcessIn lands it on the
	// scheduled set, not the pending queue.
	var renewFound bool
	for _, q := range []string{QueueCritical, QueueDefault, QueueLow} {
		infos, err := inspector.ListScheduledTasks(q, asynq.PageSize(100))
		if err != nil {
			continue
		}
		for _, ti := range infos {
			var p intelWebSubPayload
			if err := json.Unmarshal(ti.Payload, &p); err == nil && p.ChannelID == ch.ID.String() {
				renewFound = true
			}
		}
	}
	if !renewFound {
		t.Error("no intel:websub_renew armed")
	}
}

// TestIntelWebSubSubscribe_DisabledChannel ends the chain silently —
// disabled/deleted/repurposed channels get no hub call and no renewal.
func TestIntelWebSubSubscribe_DisabledChannel(t *testing.T) {
	_, srcStore, _, inspector := wireIntelChannelDeps(t)

	hubCalled := false
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hubCalled = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	src := mkIntelTestSource(t, srcStore, "websub-off-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(p *gen.CreateIntelSourceChannelParams) {
		p.CrawlMethod = "webhook"
		p.Enabled = false
		p.ProviderConfig = []byte(fmt.Sprintf(`{"hub":%q,"topic":"t"}`, hub.URL))
	})

	payload := marshalPayload(intelWebSubPayload{Version: "v1", ChannelID: ch.ID.String()})
	if err := HandleIntelWebSubSubscribe(t.Context(), asynq.NewTask(TypeIntelWebSubSubscribe, payload)); err != nil {
		t.Fatalf("HandleIntelWebSubSubscribe: %v", err)
	}
	if hubCalled {
		t.Error("hub called for a disabled channel")
	}
	if tasks := pendingTasks(t, inspector, TypeIntelWebSubRenew); len(tasks) != 0 {
		t.Error("renewal armed for a disabled channel")
	}
}

// ---------------------------------------------------------------------------
// Phase-3 search — the poll task runs the shared fetch core.
// ---------------------------------------------------------------------------

// TestHandleIntelSearchPoll_AdapterDisabled exercises the benign-skip
// contract: a search channel with no intel.search.provider configured
// yields ErrAdapterDisabled inside intelFetchChannel — nil return, no raw
// doc, no failure bookkeeping.
func TestHandleIntelSearchPoll_AdapterDisabled(t *testing.T) {
	database, srcStore, _, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "search-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(p *gen.CreateIntelSourceChannelParams) {
		p.CrawlMethod = "search"
		p.Pipeline = "curate"
		p.ProviderConfig = []byte(`{"query":"japa scholarships"}`)
	})

	payload := marshalPayload(intelChannelPayload{
		Version: "v1", SourceID: src.ID.String(), ChannelID: ch.ID.String(),
	})
	if err := HandleIntelSearchPoll(t.Context(), asynq.NewTask(TypeIntelSearchPoll, payload)); err != nil {
		t.Fatalf("HandleIntelSearchPoll: %v", err)
	}
	if n := countChannelRawDocs(t, database, ch.ID); n != 0 {
		t.Errorf("raw docs = %d, want 0 (adapter disabled)", n)
	}
	if tasks := pendingTasks(t, inspector, TypeIntelDetectChanges); len(tasks) != 0 {
		t.Error("detect_changes enqueued for a disabled adapter")
	}
}

// TestHandleIntelFetchChannel_SearchRedirect covers the fetch_channel →
// search_poll redirect: the scheduler's fetch task must not run the fetch
// inline but re-enqueue the dedicated poll task.
func TestHandleIntelFetchChannel_SearchRedirect(t *testing.T) {
	_, srcStore, _, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "search-redir-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(p *gen.CreateIntelSourceChannelParams) {
		p.CrawlMethod = "search"
		p.Pipeline = "curate"
		p.ProviderConfig = []byte(`{"query":"japa"}`)
	})

	payload := marshalPayload(intelChannelPayload{
		Version: "v1", SourceID: src.ID.String(), ChannelID: ch.ID.String(),
	})
	if err := HandleIntelFetchChannel(t.Context(), asynq.NewTask(TypeIntelFetchChannel, payload)); err != nil {
		t.Fatalf("HandleIntelFetchChannel: %v", err)
	}
	if tasks := pendingTasks(t, inspector, TypeIntelSearchPoll); len(tasks) != 1 {
		t.Fatalf("search_poll tasks = %d, want 1", len(tasks))
	}
}
