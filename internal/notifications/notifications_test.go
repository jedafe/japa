package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestNotificationTypeConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		TypeNewFollower:        "follow.received",
		TypeContentComment:     "content.comment",
		TypeCommentReply:       "comment.reply",
		TypeMentionContent:     "mention.content",
		TypeMentionComment:     "mention.comment",
		TypeContentReaction:    "content.reaction",
		TypeDMMessage:          "dm.message",
		TypeDMRequest:          "dm.request",
		TypeSystemAnnouncement: "broadcast",
	}
	for got, want := range want {
		if got != want {
			t.Errorf("notification type constant mismatch: got %q want %q", got, want)
		}
	}
}

func TestActorTypeConstants(t *testing.T) {
	t.Parallel()

	if ActorTypeUser != "user" {
		t.Errorf("ActorTypeUser = %q, want user", ActorTypeUser)
	}

	if ActorTypeSystem != "system" {
		t.Errorf("ActorTypeSystem = %q, want system", ActorTypeSystem)
	}
}

func TestBatchWindowMinutes(t *testing.T) {
	t.Parallel()

	if BatchWindowMinutes != 5 {
		t.Errorf("BatchWindowMinutes = %d, want 5", BatchWindowMinutes)
	}
}

func TestPageSizeConstants(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

// ---------------------------------------------------------------------------
// writeJSON / writeError
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if got["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, got["status"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	errMap, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope, got %v", got)
	}

	if errMap["code"] != "invalid_body" {
		t.Errorf("error.code = %v, want invalid_body", errMap["code"])
	}

	if errMap["message"] != "Invalid JSON body" {
		t.Errorf("error.message = %v, want 'Invalid JSON body'", errMap["message"])
	}
}

// ---------------------------------------------------------------------------
// parsePageSize
// ---------------------------------------------------------------------------

func TestParsePageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  int32
	}{
		{"", DefaultPageSize},
		{"0", DefaultPageSize},
		{"-1", DefaultPageSize},
		{"abc", DefaultPageSize},
		{"10", 10},
		{"50", 50},
		{"100", 100},
		{"101", MaxPageSize},
		{"999", MaxPageSize},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/notifications?page_size="+tt.query, nil)

		got := parsePageSize(req)
		if got != tt.want {
			t.Errorf("parsePageSize(page_size=%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parsePageOffset
// ---------------------------------------------------------------------------

func TestParsePageOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  int32
	}{
		{"", 0},
		{"0", 0},
		{"-1", 0},
		{"abc", 0},
		{"10", 10},
		{"50", 50},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/notifications?page_offset="+tt.query, nil)

		got := parsePageOffset(req)
		if got != tt.want {
			t.Errorf("parsePageOffset(page_offset=%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// BuildBatchTitle
// ---------------------------------------------------------------------------

func TestBuildBatchTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		actorName string
		count     int32
		action    string
		want      string
	}{
		{"single actor", "Alice", 1, "reacted", "Alice reacted"},
		{"two actors", "Alice", 2, "reacted", "Alice and 1 other reacted"},
		{"three actors", "Alice", 3, "reacted", "Alice and 2 others reacted"},
		{"ten actors", "Alice", 10, "reacted", "Alice and 9 others reacted"},
		{"hundred actors", "Bob", 100, "followed you", "Bob and 99 others followed you"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BuildBatchTitle(tt.actorName, tt.count, tt.action)
			if got != tt.want {
				t.Errorf("BuildBatchTitle(%q, %d, %q) = %q, want %q", tt.actorName, tt.count, tt.action, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SplitNames
// ---------------------------------------------------------------------------

func TestSplitNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Alice", "Alice"},
		{"Alice,Bob", "Alice"},
		{"Alice, Bob, Charlie", "Alice"},
		{"", ""},
	}
	for _, tt := range tests {
		got := SplitNames(tt.input)
		if got != tt.want {
			t.Errorf("SplitNames(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// pluralize
// ---------------------------------------------------------------------------

func TestPluralize(t *testing.T) {
	t.Parallel()

	if got := pluralize(1); got != "" {
		t.Errorf("pluralize(1) = %q, want empty", got)
	}

	if got := pluralize(2); got != "s" {
		t.Errorf("pluralize(2) = %q, want s", got)
	}

	if got := pluralize(0); got != "s" {
		t.Errorf("pluralize(0) = %q, want s", got)
	}
}

// ---------------------------------------------------------------------------
// NewService constructor
// ---------------------------------------------------------------------------

func TestNewServiceNotNil(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	if svc.store != nil {
		t.Error("expected nil store for zero-value Service")
	}
}

// ---------------------------------------------------------------------------
// NotificationInfo / PreferenceInfo struct tags
// ---------------------------------------------------------------------------

func TestNotificationInfoJSONTags(t *testing.T) {
	t.Parallel()

	ni := NotificationInfo{}

	data, err := json.Marshal(ni)
	if err != nil {
		t.Fatalf("marshal NotificationInfo: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Verify the JSON keys are snake_case
	if _, ok := got["notification_type"]; !ok {
		t.Error("expected notification_type key in JSON output")
	}
}

func TestDispatchResultJSONTags(t *testing.T) {
	t.Parallel()

	dr := DispatchResult{Action: "created"}

	data, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal DispatchResult: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["action"] != "created" {
		t.Errorf("action = %v, want created", got["action"])
	}
}

// ---------------------------------------------------------------------------
// NotificationDispatchPayload
// ---------------------------------------------------------------------------

func TestNotificationDispatchPayloadJSONTags(t *testing.T) {
	t.Parallel()

	p := NotificationDispatchPayload{WindowID: "abc-123"}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal NotificationDispatchPayload: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["window_id"] != "abc-123" {
		t.Errorf("window_id = %v, want abc-123", got["window_id"])
	}
}

// ---------------------------------------------------------------------------
// requireTenant (unit test without DB)
// ---------------------------------------------------------------------------

func TestRequireTenantNoTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	// No tenant in context — should return false.
	_, ok := h.requireTenant(rr, req)
	if ok {
		t.Error("expected ok=false when no tenant in context")
	}

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// requireUser (unit test without DB)
// ---------------------------------------------------------------------------

func TestRequireUserNoUser(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	// No user in context — should return false.
	_, ok := h.requireUser(rr, req)
	if ok {
		t.Error("expected ok=false when no user in context")
	}

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
