package scheduler

import (
	"context"
	"errors"
	"strconv"
	"testing"

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

// testTenantID is the default tenant UUID used for tenant-scoped Valkey keys
// in tests. Matches the system default tenant (00000000-0000-0000-0000-000000000001)
// per ADR-112 Amendment 2.
func testTenantID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Valid: true}
}

// fakeLister is a stand-in DueSourceLister so SelectDue can be tested without
// a *db.DB. It returns a fixed slice and (optionally) a recorded error.
type fakeLister struct {
	rows []gen.IntelSource
	err  error
	last int32
}

func (f *fakeLister) ListDue(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]gen.IntelSource, error) {
	f.last = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func TestSelectDue_PassesBatchSizeAndReturnsRows(t *testing.T) {
	rows := []gen.IntelSource{
		{Name: "src-a", Domain: "a.example.com"},
		{Name: "src-b", Domain: "b.example.com"},
	}
	fl := &fakeLister{rows: rows}
	s := NewScheduler(fl, startTestValkey(t), testTenantID())

	got, err := s.SelectDue(context.Background(), 42)
	if err != nil {
		t.Fatalf("SelectDue: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Name != "src-a" || got[1].Name != "src-b" {
		t.Errorf("unexpected rows: %+v", got)
	}
	if fl.last != 42 {
		t.Errorf("batch size forwarded to store: got %d, want 42", fl.last)
	}
}

func TestSelectDue_PropagatesStoreError(t *testing.T) {
	fl := &fakeLister{err: errors.New("boom")}
	s := NewScheduler(fl, startTestValkey(t), testTenantID())

	if _, err := s.SelectDue(context.Background(), 10); err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

func TestRecordCrawlOutcome_ChangedHalvesInterval(t *testing.T) {
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, rdb, testTenantID())
	ctx := context.Background()
	const base int32 = 60

	// Seed an effective interval of 240 mins (4x base), then observe a change.
	if err := rdb.Set(ctx, intervalKey(testTenantID(), "s1"), "240", 0).Err(); err != nil {
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
	if n, _ := rdb.Get(ctx, unchangedCountKey(testTenantID(), "s1")).Int(); n != 0 {
		t.Errorf("unchanged count after change: got %d, want 0", n)
	}
}

func TestRecordCrawlOutcome_ChangedFloorsAtBase(t *testing.T) {
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, rdb, testTenantID())
	ctx := context.Background()
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
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, rdb, testTenantID())
	ctx := context.Background()
	const base int32 = 60

	for i := 1; i < UnchangedThreshold; i++ {
		if err := s.RecordCrawlOutcome(ctx, "s3", base, false); err != nil {
			t.Fatalf("RecordCrawlOutcome #%d: %v", i, err)
		}
		n, _ := rdb.Get(ctx, unchangedCountKey(testTenantID(), "s3")).Int()
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
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, rdb, testTenantID())
	ctx := context.Background()
	const base int32 = 60

	// Drive the counter up to the threshold. The 5th unchanged crawl should
	// double the interval from base (60) to 120 and reset the counter.
	for i := 0; i < UnchangedThreshold; i++ {
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
	if n, _ := rdb.Get(ctx, unchangedCountKey(testTenantID(), "s4")).Int(); n != 0 {
		t.Errorf("unchanged count after threshold: got %d, want 0 (reset)", n)
	}
}

func TestRecordCrawlOutcome_DoublesCappedAtMax(t *testing.T) {
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, rdb, testTenantID())
	ctx := context.Background()
	const base int32 = 60

	// Seed an interval already near the cap; doubling would exceed it.
	nearCap := MaxEffectiveIntervalMins - 10
	if err := rdb.Set(ctx, intervalKey(testTenantID(), "s5"), strconv.Itoa(int(nearCap)), 0).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Pre-set the counter to threshold-1 so the next unchanged crawl triggers
	// the doubling path.
	if err := rdb.Set(ctx, unchangedCountKey(testTenantID(), "s5"), strconv.Itoa(UnchangedThreshold-1), 0).Err(); err != nil {
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
	s := NewScheduler(&fakeLister{}, startTestValkey(t), testTenantID())
	ctx := context.Background()

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
	s := NewScheduler(&fakeLister{}, startTestValkey(t), testTenantID())
	ctx := context.Background()
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
	rdb := startTestValkey(t)
	s := NewScheduler(&fakeLister{}, rdb, testTenantID())
	ctx := context.Background()
	const base int32 = 60

	// Build up a partial streak, then a change resets it, then one more
	// unchanged crawl should start the counter at 1 again.
	for i := 0; i < UnchangedThreshold-1; i++ {
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
	n, _ := rdb.Get(ctx, unchangedCountKey(testTenantID(), "s7")).Int()
	if n != 1 {
		t.Errorf("counter after reset + 1 unchanged: got %d, want 1", n)
	}
}
