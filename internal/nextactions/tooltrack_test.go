package nextactions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/analytics"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
)

type fakeTracker struct{ events []analytics.TrackEventInput }

func (f *fakeTracker) TrackEvent(_ context.Context, in analytics.TrackEventInput) (gen.CreatePlatformEventRow, error) {
	f.events = append(f.events, in)
	return gen.CreatePlatformEventRow{}, nil
}

func tracked(t *testing.T, tracker *fakeTracker, method, path string, status int, user *auth.AuthenticatedUser) {
	t.Helper()

	h := TrackToolUsage(tracker)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))

	r := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	if user != nil {
		r = r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, user))
	}

	h.ServeHTTP(httptest.NewRecorder(), r)
}

func TestTrackToolUsage_EmitsOnMatch(t *testing.T) {
	t.Parallel()

	tr := &fakeTracker{}
	tracked(t, tr, "POST", "/api/v1/crs/calculate", 200, nil)

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}

	ev := tr.events[0]
	if ev.EventType != analytics.EventToolUsed {
		t.Errorf("event type = %q, want tool.used", ev.EventType)
	}

	if ev.Meta["tool"] != "crs-calculator" {
		t.Errorf("meta.tool = %v, want crs-calculator", ev.Meta["tool"])
	}
}

func TestTrackToolUsage_MethodAndPrefix(t *testing.T) {
	t.Parallel()

	tr := &fakeTracker{}

	// GET on a POST-only route → no event.
	tracked(t, tr, "GET", "/api/v1/crs/calculate", 200, nil)
	// Unknown path → no event.
	tracked(t, tr, "GET", "/api/v1/contents", 200, nil)
	// Guide detail GET → emits.
	tracked(t, tr, "GET", "/api/v1/housing-guide/abc-123", 200, nil)
	// First match wins — /jce/comparisons/{id} POST maps once, not twice.
	tracked(t, tr, "POST", "/api/v1/jce/comparisons/abc/items", 200, nil)

	if len(tr.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(tr.events))
	}

	if tr.events[0].Meta["tool"] != "housing-guide" {
		t.Errorf("event0 tool = %v, want housing-guide", tr.events[0].Meta["tool"])
	}

	if tr.events[1].Meta["tool"] != "comparison-tool" {
		t.Errorf("event1 tool = %v, want comparison-tool", tr.events[1].Meta["tool"])
	}
}

func TestTrackToolUsage_SkipsErrors(t *testing.T) {
	t.Parallel()

	tr := &fakeTracker{}
	tracked(t, tr, "POST", "/api/v1/crs/calculate", 400, nil)
	tracked(t, tr, "POST", "/api/v1/crs/calculate", 500, nil)

	if len(tr.events) != 0 {
		t.Errorf("error responses must not emit, got %d", len(tr.events))
	}
}

func TestTrackToolUsage_UserIdentity(t *testing.T) {
	t.Parallel()

	tr := &fakeTracker{}
	uid := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	tracked(t, tr, "POST", "/api/v1/crs/calculate", 200,
		&auth.AuthenticatedUser{UserID: uid, SessionID: "jti-1"})

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}

	if tr.events[0].UserID != uid {
		t.Errorf("user_id = %v, want %v", tr.events[0].UserID, uid)
	}

	if tr.events[0].SessionID != "jti-1" {
		t.Errorf("session = %q, want jti-1", tr.events[0].SessionID)
	}
}

func TestTrackToolUsage_NilTrackerSafe(t *testing.T) {
	t.Parallel()

	h := TrackToolUsage(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/crs/calculate", nil)
	h.ServeHTTP(httptest.NewRecorder(), r) // must not panic
}
