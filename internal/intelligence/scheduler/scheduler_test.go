package scheduler

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db/gen"
)

// startTestValkey spins up an in-memory Valkey (miniredis) and returns a
// go-redis-compatible client so the scheduler can be tested without a real
// server. Mirrors the pattern in internal/intelligence/fetcher.
func startTestValkey(t *testing.T) valkeycompat.Cmdable {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}

	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}

	t.Cleanup(func() { vk.Close() })

	return valkeycompat.NewAdapter(vk)
}

// fakeLister is a stand-in DueSourceLister so SelectDue can be tested without
// a *db.DB. It returns a fixed slice and (optionally) a recorded error.
type fakeLister struct {
	rows []gen.IntelSource
	err  error
	last int32
}

func (f *fakeLister) ListDue(_ context.Context, limit int32) ([]gen.IntelSource, error) {
	f.last = limit
	if f.err != nil {
		return nil, f.err
	}

	return f.rows, nil
}

func TestSelectDue_PassesBatchSizeAndReturnsRows(t *testing.T) {
	t.Parallel()

	rows := []gen.IntelSource{
		{Name: "src-a", Domain: "a.example.com"},
		{Name: "src-b", Domain: "b.example.com"},
	}
	fl := &fakeLister{rows: rows}
	s := NewScheduler(fl, nil, startTestValkey(t))

	got, err := s.SelectDue(t.Context(), 42)
	if err != nil {
		t.Fatalf("SelectDue: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}

	if got[0].Name != "src-a" || got[1].Name != "src-b" {
		t.Errorf("unexpected rows: %+v", got)
	}

	// The store limit is batchSize × dueOverFetch — the tuned-interval
	// post-filter can drop statically-due rows, so the query over-fetches.
	if fl.last != 42*dueOverFetch {
		t.Errorf("batch size forwarded to store: got %d, want %d", fl.last, 42*dueOverFetch)
	}
}

func TestSelectDue_PropagatesStoreError(t *testing.T) {
	t.Parallel()

	fl := &fakeLister{err: errors.New("boom")}
	s := NewScheduler(fl, nil, startTestValkey(t))

	if _, err := s.SelectDue(t.Context(), 10); err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

// fakeChannelLister is a stand-in DueChannelLister so SelectDueChannels can
// be tested without a *db.DB.
type fakeChannelLister struct {
	rows []gen.IntelSourceChannel
	err  error
	last int32
}

func (f *fakeChannelLister) ListDueChannels(_ context.Context, limit int32) ([]gen.IntelSourceChannel, error) {
	f.last = limit
	if f.err != nil {
		return nil, f.err
	}

	return f.rows, nil
}

func TestSelectDueChannels_PassesBatchSizeAndReturnsRows(t *testing.T) {
	t.Parallel()

	rows := []gen.IntelSourceChannel{
		{Label: "feed", EntryUrl: "https://a.example.com/feed", CrawlMethod: "rss"},
		{Label: "page", EntryUrl: "https://b.example.com/jobs", CrawlMethod: "static"},
	}
	cl := &fakeChannelLister{rows: rows}
	s := NewScheduler(&fakeLister{}, cl, startTestValkey(t))

	got, err := s.SelectDueChannels(t.Context(), 25)
	if err != nil {
		t.Fatalf("SelectDueChannels: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}

	if got[0].EntryUrl != "https://a.example.com/feed" || got[1].CrawlMethod != "static" {
		t.Errorf("unexpected rows: %+v", got)
	}

	if cl.last != 25*dueOverFetch {
		t.Errorf("batch size forwarded to store: got %d, want %d", cl.last, 25*dueOverFetch)
	}
}

// TestSelectDueChannels_TunedIntervalSuppresses covers the volatility
// post-filter: a channel crawled more recently than its tuned effective
// interval is dropped even though the static interval says it is due; a
// never-crawled channel and one past its tuned interval still pass.
func TestSelectDueChannels_TunedIntervalSuppresses(t *testing.T) {
	t.Parallel()

	rdb := startTestValkey(t)
	ctx := t.Context()

	stableID := mustChannelUUID(t, "33333333-3333-4333-8333-333333333333")
	volatileID := mustChannelUUID(t, "44444444-4444-4444-8444-444444444444")

	rows := []gen.IntelSourceChannel{
		{
			ID: stableID, Label: "stable", EntryUrl: "https://a.example.com/feed",
			CrawlMethod: "rss", CrawlFrequencyMins: 60,
			LastCrawledAt: pgtype.Timestamptz{Time: time.Now().Add(-90 * time.Minute), Valid: true},
		},
		{
			ID: volatileID, Label: "volatile", EntryUrl: "https://b.example.com/jobs",
			CrawlMethod: "static", CrawlFrequencyMins: 60,
			LastCrawledAt: pgtype.Timestamptz{Time: time.Now().Add(-90 * time.Minute), Valid: true},
		},
		{Label: "never", EntryUrl: "https://c.example.com", CrawlMethod: "static"},
	}

	// stable is tuned to 240min — crawled 90min ago → suppressed.
	// volatile stays at base 60min → crawled 90min ago → due.
	if err := rdb.Set(ctx, intervalKey(stableID.String()), "240", 0).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cl := &fakeChannelLister{rows: rows}
	s := NewScheduler(&fakeLister{}, cl, rdb)

	got, err := s.SelectDueChannels(ctx, 10)
	if err != nil {
		t.Fatalf("SelectDueChannels: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d channels, want 2 (tuned channel suppressed)", len(got))
	}

	for _, ch := range got {
		if ch.Label == "stable" {
			t.Error("tuner-suppressed channel returned as due")
		}
	}
}

// TestSelectDueChannels_NilValkeyPassesThrough covers the fail-open path:
// with no Valkey client the static SQL batch is returned truncated to
// batchSize — a missing cache must never pause crawling.
func TestSelectDueChannels_NilValkeyPassesThrough(t *testing.T) {
	t.Parallel()

	rows := []gen.IntelSourceChannel{
		{Label: "a", EntryUrl: "https://a.example.com", CrawlMethod: "static"},
		{Label: "b", EntryUrl: "https://b.example.com", CrawlMethod: "static"},
	}
	cl := &fakeChannelLister{rows: rows}
	s := NewScheduler(&fakeLister{}, cl, nil)

	got, err := s.SelectDueChannels(t.Context(), 1)
	if err != nil {
		t.Fatalf("SelectDueChannels: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d channels, want 1 (batch cap without tuner)", len(got))
	}
}

func mustChannelUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

func TestSelectDueChannels_PropagatesStoreError(t *testing.T) {
	t.Parallel()

	cl := &fakeChannelLister{err: errors.New("boom")}
	s := NewScheduler(&fakeLister{}, cl, startTestValkey(t))

	if _, err := s.SelectDueChannels(t.Context(), 10); err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

func TestSelectDueChannels_NilLister(t *testing.T) {
	t.Parallel()

	s := NewScheduler(&fakeLister{}, nil, startTestValkey(t))

	if _, err := s.SelectDueChannels(t.Context(), 10); err == nil {
		t.Fatal("expected error with nil channel lister, got nil")
	}
}

func TestRecordCrawlOutcome_ChangedHalvesInterval(t *testing.T) {
	t.Parallel()
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, nil, rdb)
	ctx := t.Context()

	const base int32 = 60

	// Seed an effective interval of 240 mins (4x base), then observe a change.
	if err := rdb.Set(ctx, intervalKey("s1"), "240", 0).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := s.RecordCrawlOutcome(ctx, "s1", base, true); err != nil {
		t.Fatalf("RecordCrawlOutcome: %v", err)
	}

	got, err := s.EffectiveIntervalMins(ctx, "s1", base)
	if err != nil {
		t.Fatalf("EffectiveIntervalMins: %v", err)
	}

	if got != 120 {
		t.Errorf("effective interval after change: got %d, want 120", got)
	}

	// The unchanged streak counter must be reset.
	if n, _ := rdb.Get(ctx, unchangedCountKey("s1")).Int(); n != 0 {
		t.Errorf("unchanged count after change: got %d, want 0", n)
	}
}

func TestRecordCrawlOutcome_ChangedFloorsAtBase(t *testing.T) {
	t.Parallel()
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, nil, rdb)
	ctx := t.Context()

	const base int32 = 60

	// Effective interval already at base; a change must not go below base.
	if err := s.RecordCrawlOutcome(ctx, "s2", base, true); err != nil {
		t.Fatalf("RecordCrawlOutcome: %v", err)
	}

	got, err := s.EffectiveIntervalMins(ctx, "s2", base)
	if err != nil {
		t.Fatalf("EffectiveIntervalMins: %v", err)
	}

	if got != base {
		t.Errorf("effective interval floored at base: got %d, want %d", got, base)
	}
}

func TestRecordCrawlOutcome_UnchangedIncrementsCounter(t *testing.T) {
	t.Parallel()
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, nil, rdb)
	ctx := t.Context()

	const base int32 = 60

	for i := 1; i < UnchangedThreshold; i++ {
		if err := s.RecordCrawlOutcome(ctx, "s3", base, false); err != nil {
			t.Fatalf("RecordCrawlOutcome #%d: %v", i, err)
		}

		n, _ := rdb.Get(ctx, unchangedCountKey("s3")).Int()
		if int64(n) != int64(i) {
			t.Errorf("after %d unchanged crawls: counter=%d, want %d", i, n, i)
		}
		// Below the threshold the effective interval should be untouched.
		got, _ := s.EffectiveIntervalMins(ctx, "s3", base)
		if got != base {
			t.Errorf("below threshold effective interval: got %d, want base %d", got, base)
		}
	}
}

func TestRecordCrawlOutcome_UnchangedThresholdDoublesInterval(t *testing.T) {
	t.Parallel()
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, nil, rdb)
	ctx := t.Context()

	const base int32 = 60

	// Drive the counter up to the threshold. The 5th unchanged crawl should
	// double the interval from base (60) to 120 and reset the counter.
	for i := range UnchangedThreshold {
		if err := s.RecordCrawlOutcome(ctx, "s4", base, false); err != nil {
			t.Fatalf("RecordCrawlOutcome #%d: %v", i+1, err)
		}
	}

	got, err := s.EffectiveIntervalMins(ctx, "s4", base)
	if err != nil {
		t.Fatalf("EffectiveIntervalMins: %v", err)
	}

	if got != 120 {
		t.Errorf("effective interval after 5 unchanged: got %d, want 120", got)
	}

	if n, _ := rdb.Get(ctx, unchangedCountKey("s4")).Int(); n != 0 {
		t.Errorf("unchanged count after threshold: got %d, want 0 (reset)", n)
	}
}

func TestRecordCrawlOutcome_DoublesCappedAtMax(t *testing.T) {
	t.Parallel()
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, nil, rdb)
	ctx := t.Context()

	const base int32 = 60

	// Seed an interval already near the cap; doubling would exceed it.
	nearCap := MaxEffectiveIntervalMins - 10
	if err := rdb.Set(ctx, intervalKey("s5"), strconv.Itoa(nearCap), 0).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Pre-set the counter to threshold-1 so the next unchanged crawl triggers
	// the doubling path.
	if err := rdb.Set(ctx, unchangedCountKey("s5"), strconv.Itoa(UnchangedThreshold-1), 0).Err(); err != nil {
		t.Fatalf("seed counter: %v", err)
	}

	if err := s.RecordCrawlOutcome(ctx, "s5", base, false); err != nil {
		t.Fatalf("RecordCrawlOutcome: %v", err)
	}

	got, _ := s.EffectiveIntervalMins(ctx, "s5", base)
	if got != MaxEffectiveIntervalMins {
		t.Errorf("capped interval: got %d, want %d", got, MaxEffectiveIntervalMins)
	}
}

func TestRecordCrawlOutcome_RejectsEmptyIDAndBadBase(t *testing.T) {
	t.Parallel()
	s := NewScheduler(&fakeLister{}, nil, startTestValkey(t))
	ctx := t.Context()

	if err := s.RecordCrawlOutcome(ctx, "", 60, false); err == nil {
		t.Error("empty source id: expected error, got nil")
	}

	if err := s.RecordCrawlOutcome(ctx, "s6", 0, false); err == nil {
		t.Error("zero base interval: expected error, got nil")
	}

	if err := s.RecordCrawlOutcome(ctx, "s6", -1, false); err == nil {
		t.Error("negative base interval: expected error, got nil")
	}
}

func TestEffectiveIntervalMins_DefaultsToBaseWhenAbsent(t *testing.T) {
	t.Parallel()
	s := NewScheduler(&fakeLister{}, nil, startTestValkey(t))
	ctx := t.Context()

	const base int32 = 90

	got, err := s.EffectiveIntervalMins(ctx, "never-tuned", base)
	if err != nil {
		t.Fatalf("EffectiveIntervalMins: %v", err)
	}

	if got != base {
		t.Errorf("absent tuning: got %d, want base %d", got, base)
	}
}

func TestRecordCrawlOutcome_ChangeResetsStreakThenUnchangedStartsFresh(t *testing.T) {
	t.Parallel()
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, nil, rdb)
	ctx := t.Context()

	const base int32 = 60

	// Build up a partial streak, then a change resets it, then one more
	// unchanged crawl should start the counter at 1 again.
	for i := range UnchangedThreshold - 1 {
		if err := s.RecordCrawlOutcome(ctx, "s7", base, false); err != nil {
			t.Fatalf("streak build #%d: %v", i+1, err)
		}
	}

	if err := s.RecordCrawlOutcome(ctx, "s7", base, true); err != nil {
		t.Fatalf("change: %v", err)
	}

	if err := s.RecordCrawlOutcome(ctx, "s7", base, false); err != nil {
		t.Fatalf("post-change unchanged: %v", err)
	}

	n, _ := rdb.Get(ctx, unchangedCountKey("s7")).Int()
	if n != 1 {
		t.Errorf("counter after reset + 1 unchanged: got %d, want 1", n)
	}
}
