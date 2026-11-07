package publicapi

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/features"
)

type stubAPIChecker struct {
	enabled bool
	eval    features.EvalResult
}

func (s stubAPIChecker) IsEnabled(context.Context, string) bool { return s.enabled }
func (s stubAPIChecker) EvalFlag(context.Context, string, features.CallerScope) features.EvalResult {
	return s.eval
}

func TestContentTypeScopeSemantics(t *testing.T) {
	scoped := features.WithScope(context.Background(), features.CallerScope{Surface: "main"})
	svc := &Service{flags: stubAPIChecker{
		enabled: true, // globally on
		eval:    features.EvalResult{Enabled: true, Visible: false},
	}}

	// Projection: hidden-on-surface gates the listing/search surfaces.
	if !svc.contentTypeHidden(scoped, "story") {
		t.Error("scoped hidden flag must gate story on projections")
	}
	// Reachability: hidden-but-enabled still serves — Gate parity.
	if svc.contentTypeUnreachable(scoped, "story") {
		t.Error("scoped hidden flag must NOT make story unreachable")
	}
	// Ungated type never gates.
	if svc.contentTypeHidden(scoped, "guide") || svc.contentTypeUnreachable(scoped, "guide") {
		t.Error("guide is ungated — must never gate")
	}
	// Unattached ctx: global enabled → not gated (worker semantics).
	if svc.contentTypeHidden(context.Background(), "story") ||
		svc.contentTypeUnreachable(context.Background(), "story") {
		t.Error("unattached ctx must use the global toggle")
	}
	// Nil checker → not gated (fail-open).
	if (&Service{}).contentTypeHidden(scoped, "story") || (&Service{}).contentTypeUnreachable(scoped, "story") {
		t.Error("nil checker must not gate")
	}
	// Nil-safe handler delegate.
	if (&Handlers{}).contentTypeUnreachable(scoped, "story") {
		t.Error("nil Service handler must not gate")
	}
}

func TestContentTypeUnreachableScopedOff(t *testing.T) {
	scoped := features.WithScope(context.Background(), features.CallerScope{Surface: "main"})
	svc := &Service{flags: stubAPIChecker{
		enabled: true, // globally on
		eval:    features.EvalResult{Enabled: false, Visible: false},
	}}
	if !svc.contentTypeUnreachable(scoped, "story") || !svc.contentTypeHidden(scoped, "story") {
		t.Error("scoped-disabled flag must gate both reachability and projection")
	}
}

// §42: a cached digest must be re-filtered under the CURRENT caller scope —
// items from a flag that was scoped away after the cache write must not
// leak through, and stale scam alerts must be re-checked against
// scam_alert.enabled.
func TestFilterGatedDigest(t *testing.T) {
	scoped := features.WithScope(context.Background(), features.CallerScope{Surface: "main"})

	svc := &Service{
		flags: stubAPIChecker{
			enabled: true,
			eval:    features.EvalResult{Enabled: false, Visible: false},
		},
		safetyReader: stubSafetyReader{disabled: true},
	}

	d := &PublicDigest{
		ForYou: DigestFeedSection{Items: []PublicFeedItem{
			{ID: "1", ContentType: "story"}, // gated → dropped
			{ID: "2", ContentType: "guide"}, // ungated → kept
			{ID: "3", ContentType: "event"}, // calendar.enabled flag → dropped
		}},
		Community: DigestCommunity{Popular: []PublicFeedItem{
			{ID: "4", ContentType: "forum_thread"}, // feature.forum → dropped
			{ID: "5", ContentType: "blog_post"},    // feature.blog → dropped
		}},
		Destination: &DigestDestination{
			ScamAlerts: []PublicScamReport{{ID: "a"}},
		},
	}

	svc.filterGatedDigest(scoped, d)

	if len(d.ForYou.Items) != 1 || d.ForYou.Items[0].ID != "2" {
		t.Errorf("ForYou: want only the ungated item, got %+v", d.ForYou.Items)
	}
	if len(d.Community.Popular) != 0 {
		t.Errorf("Community.Popular: want empty, got %+v", d.Community.Popular)
	}
	if len(d.Destination.ScamAlerts) != 0 {
		t.Error("disabled scam_alert.enabled must strip cached scam alerts")
	}
}
