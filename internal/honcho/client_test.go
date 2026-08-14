package honcho

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testUserID = "user-123"

// newTestClient starts a test server with the given handler and returns a
// Client pointed at it.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return NewClient(srv.URL + "/")
}

// statusHandler replies with the given status code and an empty JSON body.
func statusHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}
}

func TestNewClientTrimsTrailingSlashes(t *testing.T) {
	t.Parallel()

	if got := NewClient("http://localhost:8001///").baseURL; got != "http://localhost:8001" {
		t.Errorf("baseURL = %q, want http://localhost:8001", got)
	}
}

func TestDeleteUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "success", status: http.StatusNoContent},
		{name: "already purged", status: http.StatusNotFound},
		{name: "server error", status: http.StatusInternalServerError, wantErr: true},
		{name: "bad request", status: http.StatusBadRequest, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath, gotMethod string

			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotMethod = r.URL.Path, r.Method

				statusHandler(tt.status)(w, r)
			})

			err := client.DeleteUser(t.Context(), testUserID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DeleteUser() error = %v, wantErr %v", err, tt.wantErr)
			}

			if gotMethod != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", gotMethod)
			}

			if want := "/users/" + testUserID; gotPath != want {
				t.Errorf("path = %s, want %s", gotPath, want)
			}
		})
	}
}

func TestDeleteUserRequiresUserID(t *testing.T) {
	t.Parallel()

	client := NewClient("http://localhost:8001")
	if err := client.DeleteUser(t.Context(), ""); err == nil {
		t.Error("DeleteUser(\"\") error = nil, want error")
	}
}

func TestGetContext(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if want := "/users/" + testUserID + "/context"; r.URL.Path != want {
			t.Errorf("path = %s, want %s", r.URL.Path, want)
		}

		_, _ = w.Write([]byte(`[{"id":"e1","user_id":"user-123","content":"hello"}]`))
	})

	entries, err := client.GetContext(t.Context(), testUserID)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	if entries[0].Content != "hello" {
		t.Errorf("content = %q, want hello", entries[0].Content)
	}
}

func TestGetContextNotFoundReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, statusHandler(http.StatusNotFound))

	entries, err := client.GetContext(t.Context(), testUserID)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}

	if entries == nil {
		t.Fatal("entries = nil, want empty slice")
	}

	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestGetContextErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		userID  string
	}{
		{name: "missing user id", handler: statusHandler(http.StatusOK)},
		{name: "server error", handler: statusHandler(http.StatusInternalServerError), userID: testUserID},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`not json`))
			},
			userID: testUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, tt.handler)

			if _, err := client.GetContext(t.Context(), tt.userID); err == nil {
				t.Error("GetContext() error = nil, want error")
			}
		})
	}
}

func TestAddMemory(t *testing.T) {
	t.Parallel()

	var got AddMemoryInput

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}

		_, _ = w.Write([]byte(`{"id":"e1","user_id":"user-123","content":"remembered"}`))
	})

	entry, err := client.AddMemory(t.Context(), testUserID, AddMemoryInput{
		Content:  "remembered",
		Metadata: map[string]any{"source": "chat"},
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	if entry.ID != "e1" {
		t.Errorf("entry.ID = %q, want e1", entry.ID)
	}

	if got.Content != "remembered" {
		t.Errorf("request content = %q, want remembered", got.Content)
	}

	if got.Metadata["source"] != "chat" {
		t.Errorf("request metadata source = %v, want chat", got.Metadata["source"])
	}
}

func TestAddMemoryErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		userID string
		input  AddMemoryInput
	}{
		{name: "missing user id", input: AddMemoryInput{Content: "x"}},
		{name: "missing content", userID: testUserID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, statusHandler(http.StatusOK))

			if _, err := client.AddMemory(t.Context(), tt.userID, tt.input); err == nil {
				t.Error("AddMemory() error = nil, want error")
			}
		})
	}
}

func TestAddMemoryServerError(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, statusHandler(http.StatusInternalServerError))

	if _, err := client.AddMemory(t.Context(), testUserID, AddMemoryInput{Content: "x"}); err == nil {
		t.Error("AddMemory() error = nil, want error")
	}
}

func TestDeleteEntry(t *testing.T) {
	t.Parallel()

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		statusHandler(http.StatusNoContent)(w, r)
	})

	if err := client.DeleteEntry(t.Context(), testUserID, "e1"); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}

	if want := "/users/" + testUserID + "/context/e1"; gotPath != want {
		t.Errorf("path = %s, want %s", gotPath, want)
	}
}

func TestDeleteEntryErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		userID  string
		entryID string
		status  int
	}{
		{name: "missing user id", entryID: "e1", status: http.StatusOK},
		{name: "missing entry id", userID: testUserID, status: http.StatusOK},
		{name: "server error", userID: testUserID, entryID: "e1", status: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, statusHandler(tt.status))

			if err := client.DeleteEntry(t.Context(), tt.userID, tt.entryID); err == nil {
				t.Error("DeleteEntry() error = nil, want error")
			}
		})
	}
}

// TestDeleteEntryNotFoundIsIdempotent verifies that a 404 response is treated
// as success (the entry was already deleted), consistent with DeleteUser.
func TestDeleteEntryNotFoundIsIdempotent(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, statusHandler(http.StatusNotFound))

	if err := client.DeleteEntry(t.Context(), testUserID, "e1"); err != nil {
		t.Errorf("DeleteEntry() on 404 error = %v, want nil (idempotent)", err)
	}
}

func TestRequestsCarryContext(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, statusHandler(http.StatusOK))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := client.DeleteUser(ctx, testUserID); err == nil {
		t.Error("DeleteUser with cancelled context error = nil, want error")
	}
}

func TestHealthConnectivityOnly(t *testing.T) {
	t.Parallel()

	// The current Honcho /health endpoint returns {"status":"ok"} with no
	// metrics — the report should be healthy with nil SyncLag/StoreSize.
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	report, err := client.Health(t.Context())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !report.Healthy {
		t.Error("report.Healthy = false, want true")
	}
	if report.SyncLag != nil {
		t.Errorf("report.SyncLag = %v, want nil", *report.SyncLag)
	}
	if report.StoreSize != nil {
		t.Errorf("report.StoreSize = %v, want nil", *report.StoreSize)
	}
	if report.Details == "" {
		t.Error("report.Details = empty, want non-empty")
	}
}

func TestHealthWithMetrics(t *testing.T) {
	t.Parallel()

	// A future Honcho version might include metrics in /health. The client
	// should extract them from any of the known field names.
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","pending_work_units":12,"total_users":3456}`))
	})

	report, err := client.Health(t.Context())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !report.Healthy {
		t.Error("report.Healthy = false, want true")
	}
	if report.SyncLag == nil || *report.SyncLag != 12 {
		t.Errorf("report.SyncLag = %v, want 12", report.SyncLag)
	}
	if report.StoreSize == nil || *report.StoreSize != 3456 {
		t.Errorf("report.StoreSize = %v, want 3456", report.StoreSize)
	}
	if report.Details == "" {
		t.Error("report.Details = empty, want non-empty")
	}
}

func TestHealthUnhealthyStatus(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
	})

	report, err := client.Health(t.Context())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if report.Healthy {
		t.Error("report.Healthy = true, want false")
	}
}

func TestHealthServerError(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, statusHandler(http.StatusInternalServerError))

	if _, err := client.Health(t.Context()); err == nil {
		t.Error("Health() on 500 error = nil, want error")
	}
}

func TestHealthNonJSONBody(t *testing.T) {
	t.Parallel()

	// A non-JSON body should not cause an error — connectivity is confirmed.
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`OK`))
	})

	report, err := client.Health(t.Context())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !report.Healthy {
		t.Error("report.Healthy = false, want true")
	}
}
