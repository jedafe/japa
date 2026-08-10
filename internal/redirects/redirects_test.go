package redirects

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

const validSourcePath = "/old-path"

func TestParsePageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  int32
	}{
		{name: "absent", query: "", want: DefaultPageSize},
		{name: "valid", query: "page_size=7", want: 7},
		{name: "at max", query: fmt.Sprintf("page_size=%d", MaxPageSize), want: MaxPageSize},
		{name: "above max is clamped", query: "page_size=1000", want: MaxPageSize},
		{name: "zero falls back", query: "page_size=0", want: DefaultPageSize},
		{name: "negative falls back", query: "page_size=-1", want: DefaultPageSize},
		{name: "non numeric falls back", query: "page_size=x", want: DefaultPageSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/redirects?"+tt.query, nil)

			if got := parsePageSize(req); got != tt.want {
				t.Errorf("parsePageSize(%q) = %d, want %d", tt.query, got, tt.want)
			}
		})
	}
}

func TestParsePageOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  int32
	}{
		{name: "absent", query: "", want: 0},
		{name: "valid", query: "page_offset=40", want: 40},
		{name: "negative falls back", query: "page_offset=-5", want: 0},
		{name: "non numeric falls back", query: "page_offset=x", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/redirects?"+tt.query, nil)

			if got := parsePageOffset(req); got != tt.want {
				t.Errorf("parsePageOffset(%q) = %d, want %d", tt.query, got, tt.want)
			}
		})
	}
}

func TestParseUUIDParam(t *testing.T) {
	t.Parallel()

	// Without a chi route context the URL param resolves to "", which is not a
	// valid UUID — the handler must surface that as an error rather than a zero ID.
	req := httptest.NewRequest(http.MethodGet, "/redirects/not-a-uuid", nil)

	if _, err := parseUUIDParam(req, "id"); err == nil {
		t.Error("parseUUIDParam() error = nil, want error")
	}
}

func TestHandleServiceError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: ErrRedirectNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{
			name:       "invalid status",
			err:        ErrInvalidHTTPStatus,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_error",
		},
		{
			name:       "duplicate source path",
			err:        fmt.Errorf("creating: %w", ErrDuplicateSourcePath),
			wantStatus: http.StatusConflict,
			wantCode:   "already_exists",
		},
		{
			name:       "required field",
			err:        errpkg.ErrRequired,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_error",
		},
		{
			name:       "unknown error",
			err:        stderrors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			handleServiceError(rec, tt.err)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}

			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			if body.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", body.Error.Code, tt.wantCode)
			}
		})
	}
}

// Validation runs before any store access, so a zero-value Service is enough to
// exercise the rejection paths without a database.
func TestCreateRedirectValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sourcePath string
		httpStatus int16
		wantErr    error
	}{
		{name: "unsupported status", sourcePath: validSourcePath, httpStatus: 308, wantErr: ErrInvalidHTTPStatus},
		{name: "zero status", sourcePath: validSourcePath, httpStatus: 0, wantErr: ErrInvalidHTTPStatus},
		{name: "relative source path", sourcePath: "old-path", httpStatus: 301},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &Service{}

			_, err := svc.CreateRedirect(t.Context(), pgtype.UUID{}, tt.sourcePath, "/new-path", tt.httpStatus, nil)
			if err == nil {
				t.Fatal("CreateRedirect() error = nil, want error")
			}

			if tt.wantErr != nil && !stderrors.Is(err, tt.wantErr) {
				t.Errorf("CreateRedirect() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateRedirectValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{}

	_, err := svc.UpdateRedirect(t.Context(), pgtype.UUID{}, pgtype.UUID{}, validSourcePath, "/new", 418, nil, nil)
	if !stderrors.Is(err, ErrInvalidHTTPStatus) {
		t.Errorf("UpdateRedirect() error = %v, want ErrInvalidHTTPStatus", err)
	}

	_, err = svc.UpdateRedirect(t.Context(), pgtype.UUID{}, pgtype.UUID{}, "old-path", "/new", 301, nil, nil)
	if err == nil {
		t.Error("UpdateRedirect() with relative source path error = nil, want error")
	}
}

func TestValidateTargetURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		targetURL string
		wantErr   bool
	}{
		{"root-relative path", "/new-path", false},
		{"absolute https", "https://example.com/path", false},
		{"absolute http", "http://example.com/path", false},
		{"empty", "", true},
		{"protocol-relative", "//evil.com", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"data scheme", "data:text/html,<script>", true},
		{"file scheme", "file:///etc/passwd", true},
		{"relative without slash", "new-path", true},
		{"no scheme", "example.com/path", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateTargetURL(tt.targetURL)
			if tt.wantErr && err == nil {
				t.Errorf("validateTargetURL(%q) expected error, got nil", tt.targetURL)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateTargetURL(%q) unexpected error: %v", tt.targetURL, err)
			}
		})
	}
}

func TestCreateRedirectRejectsInvalidTargetURL(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	_, err := svc.CreateRedirect(t.Context(), pgtype.UUID{}, validSourcePath, "javascript:alert(1)", 301, nil)
	if !stderrors.Is(err, ErrInvalidTargetURL) {
		t.Errorf("CreateRedirect() with javascript: URL error = %v, want ErrInvalidTargetURL", err)
	}
}

func TestBulkActionRejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input BulkActionInput
	}{
		{name: "invalid action", input: BulkActionInput{Action: "explode"}},
		{name: "empty action", input: BulkActionInput{}},
		{name: "invalid id", input: BulkActionInput{Action: "activate", Ids: []string{"nope"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &Service{}

			if err := svc.BulkAction(t.Context(), pgtype.UUID{}, tt.input); err == nil {
				t.Error("BulkAction() error = nil, want error")
			}
		})
	}
}

func TestNoopEnqueuer(t *testing.T) {
	t.Parallel()

	if err := (&noopEnqueuer{}).Enqueue("task", nil); err != nil {
		t.Errorf("noopEnqueuer.Enqueue() = %v, want nil", err)
	}
}

func TestSetEnqueuer(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	custom := &recordingEnqueuer{}

	svc.SetEnqueuer(custom)
	svc.enqueueRecordHit(pgtype.UUID{}, "redirect-1")

	if custom.calls != 1 {
		t.Errorf("enqueuer calls = %d, want 1", custom.calls)
	}
}

type recordingEnqueuer struct{ calls int }

func (e *recordingEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error {
	e.calls++

	return nil
}
