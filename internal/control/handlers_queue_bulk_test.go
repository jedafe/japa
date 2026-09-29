package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	req := httptest.NewRequest(method, "/control/queue/"+queueName+"/"+suffix, nil)
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
