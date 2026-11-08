package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestBodyLimit_CapsJSONBody(t *testing.T) {
	var gotErr error
	h := RequestBodyLimit(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotErr = io.ReadAll(r.Body)
	}))
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 32)))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotErr == nil {
		t.Fatal("expected body over limit to error")
	}
}

func TestRequestBodyLimit_UnderLimitPasses(t *testing.T) {
	var n int
	h := RequestBodyLimit(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		n = len(b)
	}))
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("tiny"))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if n != 4 {
		t.Fatalf("expected 4 bytes read, got %d", n)
	}
}

func TestRequestBodyLimit_ExemptsMultipartOnUploadPaths(t *testing.T) {
	var gotErr error
	h := RequestBodyLimit(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotErr = io.ReadAll(r.Body)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/upload", strings.NewReader(strings.Repeat("a", 32)))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotErr != nil {
		t.Fatalf("multipart upload path must not be capped by the backstop: %v", gotErr)
	}
}

func TestRequestBodyLimit_CapsSpoofedMultipartOnJSONPath(t *testing.T) {
	// A multipart Content-Type on a route that doesn't consume multipart
	// must not bypass the backstop — otherwise it smuggles unbounded bodies.
	var gotErr error
	h := RequestBodyLimit(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotErr = io.ReadAll(r.Body)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(strings.Repeat("a", 32)))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotErr == nil {
		t.Fatal("spoofed multipart content-type on a JSON route must stay capped")
	}
}
