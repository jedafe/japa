package content

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
)

type stubContentChecker struct {
	enabled bool
	eval    features.EvalResult
}

func (s stubContentChecker) IsEnabled(context.Context, string) bool { return s.enabled }
func (s stubContentChecker) EvalFlag(context.Context, string, features.CallerScope) features.EvalResult {
	return s.eval
}

func TestContentTypeFlagPredicates(t *testing.T) {
	scoped := features.WithScope(context.Background(), features.CallerScope{Surface: "main"})

	t.Run("unreachable gate parity", func(t *testing.T) {
		h := &Handlers{checker: stubContentChecker{
			enabled: true,
			eval:    features.EvalResult{Enabled: true, Visible: false}, // hidden-on-surface
		}}
		// Hidden-but-enabled still serves — the .Enabled Gate contract.
		if h.contentTypeUnreachable(scoped, "story") {
			t.Error("hidden flag must NOT make story unreachable")
		}
		if !h.contentTypeHidden(scoped, "story") {
			t.Error("hidden flag must prune story from projections")
		}
	})

	t.Run("scoped disabled gates both", func(t *testing.T) {
		h := &Handlers{checker: stubContentChecker{
			enabled: true,
			eval:    features.EvalResult{Enabled: false, Visible: false},
		}}
		if !h.contentTypeUnreachable(scoped, "story") || !h.contentTypeHidden(scoped, "story") {
			t.Error("scoped-disabled flag must gate reachability and projection")
		}
	})

	t.Run("unattached ctx uses global toggle", func(t *testing.T) {
		off := &Handlers{checker: stubContentChecker{enabled: false}}
		if !off.contentTypeUnreachable(context.Background(), "story") {
			t.Error("unattached ctx must use the global toggle (off → unreachable)")
		}
		on := &Handlers{checker: stubContentChecker{enabled: true}}
		if on.contentTypeUnreachable(context.Background(), "story") || on.contentTypeHidden(context.Background(), "story") {
			t.Error("unattached ctx must use the global toggle (on → serves)")
		}
	})

	t.Run("unmapped type never gates", func(t *testing.T) {
		h := &Handlers{checker: stubContentChecker{eval: features.EvalResult{Enabled: false, Visible: false}}}
		if h.contentTypeUnreachable(scoped, "guide") || h.contentTypeHidden(scoped, "guide") {
			t.Error("guide has no mapped flag — must never gate")
		}
	})

	t.Run("nil checker and nil settings fail open", func(t *testing.T) {
		h := &Handlers{}
		if h.contentTypeUnreachable(scoped, "story") || h.contentTypeHidden(scoped, "story") {
			t.Error("no checker and no settings reader must not gate")
		}
	})
}

func TestFilterFlaggedListItems(t *testing.T) {
	scoped := features.WithScope(context.Background(), features.CallerScope{Surface: "main"})
	h := &Handlers{checker: stubContentChecker{
		enabled: true,
		eval:    features.EvalResult{Enabled: false, Visible: false}, // story scoped-off
	}}

	rows := []gen.ListContentsRow{
		{ContentType: "story"},
		{ContentType: "guide"},
		{ContentType: "story"},
	}
	got, ok := h.filterFlaggedListItems(scoped, rows).([]gen.ListContentsRow)
	if !ok {
		t.Fatal("filter must preserve the concrete row type")
	}
	if len(got) != 1 || got[0].ContentType != "guide" {
		t.Fatalf("expected only the ungated guide row, got %+v", got)
	}

	// Unknown row types pass through untouched.
	type other struct{ X int }
	if h.filterFlaggedListItems(scoped, []other{{1}}).([]other)[0].X != 1 {
		t.Error("unhandled row types must pass through")
	}

	// Nil checker + nil settings → the predicate is nil → input unchanged.
	bare := &Handlers{}
	kept, _ := bare.filterFlaggedListItems(scoped, rows).([]gen.ListContentsRow)
	if len(kept) != len(rows) {
		t.Error("no flagging source must keep every row")
	}
}
