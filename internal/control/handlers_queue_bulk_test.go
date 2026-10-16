package control

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
)

// startQueueTestRedis spins up an in-memory Valkey plus a real asynq
// client/inspector pair so the bulk handlers exercise actual queue state.
func startQueueTestRedis(t *testing.T) (*miniredis.Miniredis, *asynq.Client, *asynq.Inspector) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}

	t.Cleanup(mr.Close)

	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)

	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close() })

	return mr, client, inspector
}

func enqueue(t *testing.T, client *asynq.Client, taskType, queue string, opts ...asynq.Option) {
	t.Helper()

	opts = append(opts, asynq.Queue(queue))
	if _, err := client.Enqueue(asynq.NewTask(taskType, nil), opts...); err != nil {
		t.Fatalf("enqueue %s: %v", taskType, err)
	}
}

func callBulkHandler(t *testing.T, h *Handlers, method, queueName, suffix string) (int, map[string]int) {
	t.Helper()

	var reqBody io.Reader

	if suffix == "purge" {
		// Purge is irreversible — the handler requires {"confirm":true}.
		reqBody = strings.NewReader(`{"confirm":true}`)
	}

	req := httptest.NewRequest(method, "/control/queue/"+queueName+"/"+suffix, reqBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("queue_name", queueName)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	switch suffix {
	case "archive":
		h.ArchiveAllQueueTasks(rec, req)
	case "requeue":
		h.RequeueAllQueueTasks(rec, req)
	case "purge":
		h.PurgeQueueTasks(rec, req)
	}

	var body struct {
		Data struct {
			Archived int      `json:"archived"`
			Requeued int      `json:"requeued"`
			Purged   int      `json:"purged"`
			Errors   []string `json:"errors"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	return rec.Code, map[string]int{
		"archived": body.Data.Archived,
		"requeued": body.Data.Requeued,
		"purged":   body.Data.Purged,
		"__errors": len(body.Data.Errors),
	}
}

func TestQueueBulkHandlers_ArchiveAndRequeue(t *testing.T) {
	_, client, inspector := startQueueTestRedis(t)
	h := &Handlers{Inspector: inspector}

	enqueue(t, client, "test:a", "default")
	enqueue(t, client, "test:b", "default")

	code, data := callBulkHandler(t, h, http.MethodPost, "default", "archive")
	if code != http.StatusOK || data["archived"] != 2 {
		t.Fatalf("archive: code=%d data=%v", code, data)
	}

	if qi, _ := inspector.GetQueueInfo("default"); qi.Pending != 0 || qi.Archived != 2 {
		t.Fatalf("after archive: pending=%d archived=%d", qi.Pending, qi.Archived)
	}

	code, data = callBulkHandler(t, h, http.MethodPost, "default", "requeue")
	if code != http.StatusOK || data["requeued"] != 2 {
		t.Fatalf("requeue: code=%d data=%v", code, data)
	}

	if qi, _ := inspector.GetQueueInfo("default"); qi.Pending != 2 || qi.Archived != 0 {
		t.Fatalf("after requeue: pending=%d archived=%d", qi.Pending, qi.Archived)
	}
}

func TestQueueBulkHandlers_Purge(t *testing.T) {
	_, client, inspector := startQueueTestRedis(t)
	h := &Handlers{Inspector: inspector}

	enqueue(t, client, "test:a", "low")
	callBulkHandler(t, h, http.MethodPost, "low", "archive")

	// Irreversible without an explicit {"confirm":true} body — a bare POST
	// must be refused and leave the archived task untouched.
	req := httptest.NewRequest(http.MethodPost, "/control/queue/low/purge", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("queue_name", "low")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.PurgeQueueTasks(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("purge without confirm: code=%d", rec.Code)
	}

	if qi, _ := inspector.GetQueueInfo("low"); qi.Archived != 1 {
		t.Fatalf("unconfirmed purge deleted tasks: archived=%d", qi.Archived)
	}

	code, data := callBulkHandler(t, h, http.MethodPost, "low", "purge")
	if code != http.StatusOK || data["purged"] != 1 {
		t.Fatalf("purge: code=%d data=%v", code, data)
	}

	if qi, _ := inspector.GetQueueInfo("low"); qi.Archived != 0 {
		t.Fatalf("after purge: archived=%d", qi.Archived)
	}
}

func TestQueueBulkHandlers_AllQueuesAndValidation(t *testing.T) {
	_, client, inspector := startQueueTestRedis(t)
	h := &Handlers{Inspector: inspector}

	enqueue(t, client, "test:a", "default")
	enqueue(t, client, "test:b", "low")

	code, data := callBulkHandler(t, h, http.MethodPost, "all", "archive")
	if code != http.StatusOK || data["archived"] != 2 {
		t.Fatalf("archive all: code=%d data=%v", code, data)
	}
	// The test redis only holds default/low — every other configured queue
	// reports a per-queue failure which must surface in the response, not
	// be silently swallowed.
	if data["__errors"] == 0 {
		t.Fatal("archive all: partial failures not reported in errors[]")
	}

	code, _ = callBulkHandler(t, h, http.MethodPost, "bogus", "archive")
	if code != http.StatusBadRequest {
		t.Fatalf("bad queue: want 400, got %d", code)
	}
}

// TestQueueBulkHandlers_DiscardAllDLQRequiresConfirm — discard-all drains
// every dead-letter queue permanently, so it must carry the same
// {"confirm":true} gate as purge; a bare POST refuses.
func TestQueueBulkHandlers_DiscardAllDLQRequiresConfirm(t *testing.T) {
	t.Parallel()

	_, client, inspector := startQueueTestRedis(t)
	h := &Handlers{Inspector: inspector}

	enqueue(t, client, "test:dlq", "low")
	callBulkHandler(t, h, http.MethodPost, "low", "archive")

	req := httptest.NewRequest(http.MethodPost, "/control/queue/dlq/discard-all", nil)
	rec := httptest.NewRecorder()
	h.DiscardAllDLQ(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("discard-all without confirm: code=%d", rec.Code)
	}

	if qi, _ := inspector.GetQueueInfo("low"); qi.Archived != 1 {
		t.Fatalf("unconfirmed discard deleted tasks: archived=%d", qi.Archived)
	}

	// Confirmed run drains the queue.
	req = httptest.NewRequest(http.MethodPost, "/control/queue/dlq/discard-all",
		strings.NewReader(`{"confirm":true}`))
	rec = httptest.NewRecorder()
	h.DiscardAllDLQ(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("confirmed discard-all: code=%d", rec.Code)
	}

	if qi, _ := inspector.GetQueueInfo("low"); qi.Archived != 0 {
		t.Fatalf("after discard-all: archived=%d", qi.Archived)
	}
}

// TestQueueBulkHandlers_DiscardAllDLQDrainsBeyondPage — discard-all must
// drain more than asynq's 30-task list page. Regresses the bug where the
// handler listed the first page of archived tasks and deleted only those,
// silently leaving the remainder while reporting success.
func TestQueueBulkHandlers_DiscardAllDLQDrainsBeyondPage(t *testing.T) {
	t.Parallel()

	_, client, inspector := startQueueTestRedis(t)
	h := &Handlers{Inspector: inspector}

	const total = 35
	for range total {
		enqueue(t, client, "test:dlq", "low")
	}

	callBulkHandler(t, h, http.MethodPost, "low", "archive")

	if qi, _ := inspector.GetQueueInfo("low"); qi.Archived != total {
		t.Fatalf("fixture: archived=%d, want %d", qi.Archived, total)
	}

	req := httptest.NewRequest(http.MethodPost, "/control/queue/dlq/discard-all",
		strings.NewReader(`{"confirm":true}`))
	rec := httptest.NewRecorder()
	h.DiscardAllDLQ(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("discard-all: code=%d", rec.Code)
	}

	var body struct {
		Data struct {
			Discarded int `json:"discarded"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body.Data.Discarded != total {
		t.Fatalf("discarded=%d, want %d — DLQ page cap still truncates", body.Data.Discarded, total)
	}

	if qi, _ := inspector.GetQueueInfo("low"); qi.Archived != 0 {
		t.Fatalf("after discard-all: archived=%d", qi.Archived)
	}
}

// TestQueueBulkHandlers_ListDLQPaginatesPastThirty — the DLQ listing must
// return every archived task, not just asynq's default 30-row page.
func TestQueueBulkHandlers_ListDLQPaginatesPastThirty(t *testing.T) {
	t.Parallel()

	_, client, inspector := startQueueTestRedis(t)
	h := &Handlers{Inspector: inspector}

	const total = 35
	for range total {
		enqueue(t, client, "test:dlq", "low")
	}

	callBulkHandler(t, h, http.MethodPost, "low", "archive")

	req := httptest.NewRequest(http.MethodGet, "/control/queue/dlq", nil)
	rec := httptest.NewRecorder()
	h.ListDLQ(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list dlq: code=%d", rec.Code)
	}

	var body struct {
		Data []struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(body.Data) != total {
		t.Fatalf("listed %d archived tasks, want %d — pagination truncates", len(body.Data), total)
	}
}
