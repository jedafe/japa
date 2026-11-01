// linkout_test.go — §125 external link redirect coverage: public-config URL
// rewrite, affiliate seam behavior, dedup-free click path, open-redirect
// guard. Valkey dedup is exercised e2e (valkeycompat.BoolCmd can't be faked
// — same constraint as the §56 affiliate redirect).
package tips

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jedafe/japa/internal/db/gen"
)

type fakeAffResolver struct {
	resolved string
	matched  bool
	err      error
	calls    int
}

func (f *fakeAffResolver) ResolveAffiliateLinkByURL(_ context.Context, _ string) (string, bool, error) {
	f.calls++
	return f.resolved, f.matched, f.err
}

type fakeFeatures struct{ on map[string]bool }

func (f fakeFeatures) IsEnabled(_ context.Context, key string) bool { return f.on[key] }

type fakeAsynq struct {
	tasks []*asynq.Task
}

func (f *fakeAsynq) Enqueue(t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.tasks = append(f.tasks, t)
	return &asynq.TaskInfo{}, nil
}

func TestPublicConfigLinkURLRewrite(t *testing.T) {
	fs := newFakeStore()
	fs.mentor = true
	owner := UserOwner(uid(t, "11111111-1111-1111-1111-111111111111"))
	settingID := uid(t, "22222222-2222-2222-2222-222222222222")
	linkID := uid(t, "33333333-3333-3333-3333-333333333333")
	fs.settings[key(owner.Type, owner.ID)] = gen.TipSetting{
		ID: settingID, OwnerType: owner.Type, OwnerID: owner.ID, TipsEnabled: true,
		AcceptCredits: false, AcceptFiat: false, AcceptExternal: true,
	}
	fs.links[settingID] = []gen.TipExternalLink{
		{ID: linkID, Kind: "other", Label: "x", Url: "https://example.com/pay"},
	}

	svc := &Service{store: fs, siteURL: "https://japa.to"}
	cfg, err := svc.PublicConfig(context.Background(), owner)
	if err != nil {
		t.Fatalf("PublicConfig: %v", err)
	}
	want := "https://japa.to/api/v1/tips/links/" + linkID.String() + "/out"
	if len(cfg.ExternalLinks) != 1 || cfg.ExternalLinks[0].URL != want {
		t.Fatalf("public link should be the click redirect %q, got %+v", want, cfg.ExternalLinks)
	}

	// Owner dashboard keeps the raw stored URL.
	links, err := svc.ListLinks(context.Background(), owner)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 1 || links[0].URL != "https://example.com/pay" {
		t.Fatalf("owner list should keep raw URL, got %+v", links)
	}
}

func TestResolveAffiliateURLSeam(t *testing.T) {
	raw := "https://example.com/pay"

	// Nil seam → passthrough.
	svc := &Service{}
	got, matched, err := svc.ResolveAffiliateURL(context.Background(), raw)
	if err != nil || matched || got != raw {
		t.Fatalf("nil seam: got (%q,%v,%v)", got, matched, err)
	}

	// Matched → injected.
	svc = &Service{affResolver: &fakeAffResolver{resolved: raw + "?tag=jc", matched: true}}
	got, matched, _ = svc.ResolveAffiliateURL(context.Background(), raw)
	if !matched || got != raw+"?tag=jc" {
		t.Fatalf("matched: got (%q,%v)", got, matched)
	}

	// feature.affiliate_program off → resolver not consulted.
	res := &fakeAffResolver{resolved: raw + "?tag=jc", matched: true}
	svc = &Service{affResolver: res, features: fakeFeatures{on: map[string]bool{}}}
	got, matched, _ = svc.ResolveAffiliateURL(context.Background(), raw)
	if matched || got != raw || res.calls != 0 {
		t.Fatalf("flag off should passthrough without resolver call: (%q,%v) calls=%d", got, matched, res.calls)
	}

	// Resolver error → degrade to raw URL.
	svc = &Service{affResolver: &fakeAffResolver{err: errors.New("db down")}, features: fakeFeatures{on: map[string]bool{"feature.affiliate_program": true}}}
	got, matched, err = svc.ResolveAffiliateURL(context.Background(), raw)
	if err != nil || matched || got != raw {
		t.Fatalf("resolver error should degrade: (%q,%v,%v)", got, matched, err)
	}
}

func TestRedirectExternalLink(t *testing.T) {
	linkID := uid(t, "33333333-3333-3333-3333-333333333333")
	fs := newFakeStore()
	fs.linkByID[linkID] = gen.TipExternalLink{ID: linkID, Kind: "other", Url: "https://example.com/pay"}
	svc := &Service{store: fs}
	aq := &fakeAsynq{}
	h := &Handlers{service: svc, AsynqClient: aq}

	// Bad id → "/" and no enqueue.
	r := httptest.NewRequest("GET", "/out", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.RedirectExternalLink(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx)))
	if w.Result().Header.Get("Location") != "/" || len(aq.tasks) != 0 {
		t.Fatalf("bad id should redirect /, got %q tasks=%d", w.Result().Header.Get("Location"), len(aq.tasks))
	}

	// Unknown id → "/".
	rctx2 := chi.NewRouteContext()
	rctx2.URLParams.Add("id", uid(t, "44444444-4444-4444-4444-444444444444").String())
	w = httptest.NewRecorder()
	h.RedirectExternalLink(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx2)))
	if w.Result().Header.Get("Location") != "/" {
		t.Fatalf("unknown link should redirect /, got %q", w.Result().Header.Get("Location"))
	}

	// Found link, no provider match → raw destination + click enqueued.
	rctx3 := chi.NewRouteContext()
	rctx3.URLParams.Add("id", linkID.String())
	w = httptest.NewRecorder()
	h.RedirectExternalLink(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx3)))
	if w.Result().Header.Get("Location") != "https://example.com/pay" {
		t.Fatalf("expected raw destination, got %q", w.Result().Header.Get("Location"))
	}
	if len(aq.tasks) != 1 || aq.tasks[0].Type() != "tips:record_link_click" {
		t.Fatalf("expected 1 record_link_click task, got %+v", aq.tasks)
	}

	// Provider match → injected destination.
	svc.SetAffiliateResolver(&fakeAffResolver{resolved: "https://example.com/pay?tag=jc", matched: true})
	w = httptest.NewRecorder()
	h.RedirectExternalLink(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx3)))
	if w.Result().Header.Get("Location") != "https://example.com/pay?tag=jc" {
		t.Fatalf("expected injected destination, got %q", w.Result().Header.Get("Location"))
	}
}

func TestSanitizeOutURL(t *testing.T) {
	if got := sanitizeOutURL("javascript:alert(1)"); got != "/" {
		t.Fatalf("javascript: must fall back to /, got %q", got)
	}
	if got := sanitizeOutURL("https://example.com/x"); got != "https://example.com/x" {
		t.Fatalf("https must pass through, got %q", got)
	}
	if got := sanitizeOutURL("::bad"); got != "/" {
		t.Fatalf("unparseable must fall back to /, got %q", got)
	}
}
