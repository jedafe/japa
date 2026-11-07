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
	t.Run("header surface is used when no override given", func(t *testing.T) {
		sb := &stubScopeBuilder{scope: CallerScope{Surface: "main"}}
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(SurfaceHeader, "main")
		sc := ScopeForRequest(sb, r)
		if !sb.called || sb.gotSurface != "main" || sc.Surface != "main" {
			t.Fatalf("expected scope built with header surface; got %+v", sc)
		}
	})

	t.Run("explicit surface overrides the header", func(t *testing.T) {
		sb := &stubScopeBuilder{scope: CallerScope{Surface: "admin"}}
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(SurfaceHeader, "main")
		sc := ScopeForRequest(sb, r, "admin")
		if sb.gotSurface != "admin" || sc.Surface != "admin" {
			t.Fatalf("expected explicit surface to win; got %+v", sc)
		}
	})

	t.Run("checker without BuildScope yields zero scope", func(t *testing.T) {
		sc := ScopeForRequest(struct{ x int }{}, httptest.NewRequest("GET", "/", nil))
		if sc.Surface != "" || sc.CountryCode != "" || sc.GeoValid || sc.GroupSlugs != nil || sc.Permissions != nil || sc.SuperAdmin || sc.SubjectID != "" {
			t.Fatalf("expected zero scope; got %+v", sc)
		}
	})
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
