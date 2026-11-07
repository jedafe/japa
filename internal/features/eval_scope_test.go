package features

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubScopeBuilder struct {
	gotSurface string
	scope      CallerScope
	called     bool
}

func (s *stubScopeBuilder) BuildScope(_ *http.Request, surface string) CallerScope {
	s.called = true
	s.gotSurface = surface
	return s.scope
}

func TestScopeForRequest(t *testing.T) {
	t.Run("scope built from the request's header surface", func(t *testing.T) {
		sb := &stubScopeBuilder{scope: CallerScope{Surface: "main"}}
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(SurfaceHeader, "main")
		sc := ScopeForRequest(sb, r)
		if !sb.called || sb.gotSurface != "main" || sc.Surface != "main" {
			t.Fatalf("expected scope built with header surface; got %+v", sc)
		}
	})

	t.Run("absent surface header yields anonymous scope", func(t *testing.T) {
		sb := &stubScopeBuilder{scope: CallerScope{}}
		r := httptest.NewRequest("GET", "/", nil)
		sc := ScopeForRequest(sb, r)
		if !sb.called || sb.gotSurface != "" {
			t.Fatalf("expected BuildScope called with empty surface; got %q", sb.gotSurface)
		}
		if sc.Surface != "" {
			t.Fatalf("expected anonymous scope; got %+v", sc)
		}
	})

	t.Run("checker without BuildScope yields zero scope", func(t *testing.T) {
		sc := ScopeForRequest(struct{ x int }{}, httptest.NewRequest("GET", "/", nil))
		if sc.Surface != "" || sc.CountryCode != "" || sc.GeoValid || sc.GroupSlugs != nil || sc.Permissions != nil || sc.SuperAdmin || sc.SubjectID != "" {
			t.Fatalf("expected zero scope; got %+v", sc)
		}
	})
}

func TestScopeMiddleware(t *testing.T) {
	sb := &stubScopeBuilder{scope: CallerScope{Surface: "main"}}

	var got CallerScope
	var attached bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, attached = ScopeFrom(r.Context())
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(SurfaceHeader, "main")
	ScopeMiddleware(sb)(next).ServeHTTP(httptest.NewRecorder(), r)

	if !attached || got.Surface != "main" {
		t.Fatalf("middleware must attach the request scope; got %+v attached=%v", got, attached)
	}
	if !sb.called || sb.gotSurface != "main" {
		t.Fatalf("expected BuildScope with header surface; called=%v surface=%q", sb.called, sb.gotSurface)
	}
}

func TestWithScopeScopeFrom(t *testing.T) {
	ctx := context.Background()
	if _, ok := ScopeFrom(ctx); ok {
		t.Fatal("unattached ctx must report no scope")
	}
	want := CallerScope{Surface: "main", CountryCode: "NG", GeoValid: true}
	got, ok := ScopeFrom(WithScope(ctx, want))
	if !ok {
		t.Fatal("attached scope not found")
	}
	if got.Surface != "main" || got.CountryCode != "NG" || !got.GeoValid {
		t.Fatalf("scope round-trip mismatch: %+v", got)
	}
}
