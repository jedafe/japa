package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jedafe/japa/internal/worker"
)

func seedHandlers(t *testing.T) (*Handlers, *asynq.Inspector) {
	t.Helper()
	_, client, inspector := startQueueTestRedis(t)
	return &Handlers{Client: client}, inspector
}

func callEnqueue(t *testing.T, h *Handlers, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/control/seeds/enqueue", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.EnqueueSeed(rec, req)
	var out map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&out)
	return rec.Code, out
}

func errCode(t *testing.T, body map[string]any) string {
	t.Helper()
	e, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope, got %v", body)
	}
	c, _ := e["code"].(string)
	return c
}

func TestEnqueueSeed_Success(t *testing.T) {
	h, inspector := seedHandlers(t)

	code, body := callEnqueue(t, h,
		`{"kind":"populate_geo","params":{"geo_code":"ZZ","level":"university","limit":"5"}}`)
	if code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%v)", code, body)
	}

	// The op must land on the task's registered queue tier.
	tasks, err := inspector.ListPendingTasks(worker.QueueForTask(worker.TypeGeoPopulateScope))
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	found := false
	for _, task := range tasks {
		if task.Type == worker.TypeGeoPopulateScope {
			found = true
			var p map[string]any
			if err := json.Unmarshal(task.Payload, &p); err != nil {
				t.Fatalf("payload not json: %v", err)
			}
			if p["geo_code"] != "ZZ" || p["level"] != "university" || p["limit"] != float64(5) {
				t.Fatalf("payload = %v", p)
			}
		}
	}
	if !found {
		t.Fatalf("task %s not found in pending queue", worker.TypeGeoPopulateScope)
	}
}

func TestEnqueueSeed_StaticMergedBeforeParams(t *testing.T) {
	h, inspector := seedHandlers(t)

	code, body := callEnqueue(t, h,
		`{"kind":"populate_space","params":{"space_id":"fafd3512-2f41-4c4e-a5d8-e09aeecf843c"}}`)
	if code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%v)", code, body)
	}
	tasks, err := inspector.ListPendingTasks(worker.QueueForTask(worker.TypeSpaceAutoPopulate))
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	for _, task := range tasks {
		if task.Type == worker.TypeSpaceAutoPopulate {
			var p map[string]any
			_ = json.Unmarshal(task.Payload, &p)
			if p["version"] != "1" {
				t.Fatalf("static version not merged: %v", p)
			}
			return
		}
	}
	t.Fatalf("task %s not found", worker.TypeSpaceAutoPopulate)
}

func TestEnqueueSeed_Validation(t *testing.T) {
	h, _ := seedHandlers(t)

	cases := []struct {
		name     string
		body     string
		wantCode string
	}{
		{"missing required", `{"kind":"populate_geo","params":{}}`, "missing_param"},
		{"empty required", `{"kind":"populate_geo","params":{"geo_code":""}}`, "missing_param"},
		{"non-numeric limit", `{"kind":"populate_geo","params":{"geo_code":"NG","limit":"abc"}}`, "invalid_param"},
		{"bad select option", `{"kind":"populate_geo","params":{"geo_code":"NG","level":"ctiy"}}`, "invalid_param"},
		{"string for checkbox", `{"kind":"wiki_attractions","params":{"parent_space_id":"x","dry_run":"yes"}}`, "invalid_param"},
		{"number for text", `{"kind":"populate_space","params":{"space_id":42}}`, "invalid_param"},
		{"unknown kind", `{"kind":"nope","params":{}}`, "unknown_seed_op"},
		{"cli-only op not enqueueable", `{"kind":"geo_cities","params":{}}`, "unknown_seed_op"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := callEnqueue(t, h, tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%v)", code, body)
			}
			if c := errCode(t, body); c != tc.wantCode {
				t.Fatalf("code = %q, want %q", c, tc.wantCode)
			}
		})
	}
}

func TestEnqueueSeed_OptionalEmptySkipped(t *testing.T) {
	h, inspector := seedHandlers(t)

	code, body := callEnqueue(t, h,
		`{"kind":"populate_geo","params":{"geo_code":"NG","limit":""}}`)
	if code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%v)", code, body)
	}
	tasks, err := inspector.ListPendingTasks(worker.QueueForTask(worker.TypeGeoPopulateScope))
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	for _, task := range tasks {
		if task.Type == worker.TypeGeoPopulateScope {
			var p map[string]any
			_ = json.Unmarshal(task.Payload, &p)
			if _, present := p["limit"]; present {
				t.Fatalf("empty optional limit should be omitted, got %v", p)
			}
			return
		}
	}
	t.Fatalf("task not found")
}

func TestClampSeedOps(t *testing.T) {
	ops := []SeedOp{
		{Kind: "ok", Enqueueable: true, TaskType: worker.TypeGeoPopulateScope},
		{Kind: "no_task", Enqueueable: true},
		{Kind: "not_allowlisted", Enqueueable: true, TaskType: "email:send"},
		{Kind: "display", Enqueueable: false},
	}
	out := clampSeedOps(ops)
	if !out[0].Enqueueable {
		t.Fatal("allowlisted op was clamped")
	}
	if out[1].Enqueueable || out[2].Enqueueable {
		t.Fatal("non-allowlisted op kept enqueueable")
	}
	if out[3].Enqueueable {
		t.Fatal("display op flipped enqueueable")
	}
}

func TestSeedParamAsInt(t *testing.T) {
	cases := []struct {
		in    any
		want  int
		wantE bool
	}{
		{"200", 200, false},
		{" 42 ", 42, false},
		{float64(7), 7, false},
		{"abc", 0, true},
		{true, 0, true},
	}
	for _, tc := range cases {
		got, err := seedParamAsInt(tc.in)
		if (err != nil) != tc.wantE {
			t.Fatalf("seedParamAsInt(%v) err = %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("seedParamAsInt(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestListSeedOps_Envelope(t *testing.T) {
	h, _ := seedHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/control/seeds", nil)
	rec := httptest.NewRecorder()
	h.ListSeedOps(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Data []SeedOp `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) == 0 {
		t.Fatal("empty catalog")
	}
	for _, op := range body.Data {
		if op.Kind == "" || op.Title == "" {
			t.Fatalf("op missing kind/title: %+v", op)
		}
		if op.Enqueueable && (op.TaskType == "" || !seedTaskAllowlist[op.TaskType]) {
			t.Fatalf("op %s enqueueable without allowlisted task", op.Kind)
		}
	}
}
