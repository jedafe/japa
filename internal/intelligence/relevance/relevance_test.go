package relevance

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeCache is an in-memory Cache implementation — a map for verdicts and
// a map for counters. Incr/Expire mimic Valkey semantics closely enough
// for gate tests.
type fakeCache struct {
	mu       sync.Mutex
	vals     map[string]string
	counters map[string]int64
	getErr   error // inject a backend failure
}

func newFakeCache() *fakeCache {
	return &fakeCache{vals: map[string]string{}, counters: map[string]int64{}}
}

func (f *fakeCache) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getErr != nil {
		return "", f.getErr
	}
	if v, ok := f.vals[key]; ok {
		return v, nil
	}

	return "", ErrCacheMiss
}

func (f *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vals[key] = value

	return nil
}

func (f *fakeCache) Incr(_ context.Context, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counters[key]++

	return f.counters[key], nil
}

func (f *fakeCache) Expire(_ context.Context, _ string, _ time.Duration) error {
	return nil
}

// fakeClassifier records calls and returns a canned verdict.
type fakeClassifier struct {
	calls   int
	onTopic bool
	err     error
}

func (f *fakeClassifier) Classify(_ context.Context, _ ItemInput, _ []string) (bool, error) {
	f.calls++

	return f.onTopic, f.err
}

// chID returns a deterministic valid pgtype.UUID for cap-key tests.
func chID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

func filteredChannel(topics, patterns []string) sources.Channel {
	return sources.Channel{
		RelevanceMode:        string(sources.RelevanceFiltered),
		RelevanceTopics:      topics,
		RelevanceUrlPatterns: patterns,
	}
}

// ---------------------------------------------------------------------------
// URL pattern short-circuit (spec §4 step 1)
// ---------------------------------------------------------------------------

func TestEvaluate_URLPatternShortCircuits(t *testing.T) {
	t.Parallel()

	ch := filteredChannel(nil, []string{"nairaland.com/travel/*"})

	// Off-topic-scoring text but a whitelisted URL → on_topic immediately.
	item := ItemInput{Title: "Football results: Arsenal wins again", URL: "https://www.nairaland.com/travel/9912-visa-thread"}

	v, err := Evaluate(context.Background(), item, ch, nil)
	if err != nil || v != VerdictOnTopic {
		t.Fatalf("Evaluate = %v, %v; want on_topic", v, err)
	}
}

func TestMatchURLPattern(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pattern, url string
		want         bool
	}{
		{"nairaland.com/travel/*", "https://www.nairaland.com/travel/123-abc", true},
		{"nairaland.com/travel/*", "https://nairaland.com/travel/123/deep/path", true}, // * spans '/'
		{"nairaland.com/travel/*", "https://nairaland.com/politics/123", false},
		{"nairaland.com/travel/*", "https://nairaland.com/travel", false}, // anchored glob needs the path tail
		{"nairaland.com/travel", "https://nairaland.com/travel", true},    // literal → substring
		{"nairaland.com/travel", "https://nairaland.com/travel/x/y", true},
		{"HTTPS://Example.com/JOBS/*", "https://example.com/jobs/9", true}, // case-insensitive
		{"*/visa/*", "https://blog.example.com/visa/guide", true},
		{"example.com/?page=jobs", "https://example.com/?page=jobs", true}, // literal with query
		{"", "https://example.com/x", false},
		{"example.com/*", "", false},
	}

	for _, tc := range cases {
		if got := matchURLPattern(tc.pattern, tc.url); got != tc.want {
			t.Errorf("matchURLPattern(%q, %q) = %v, want %v", tc.pattern, tc.url, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Modes
// ---------------------------------------------------------------------------

func TestEvaluate_Modes(t *testing.T) {
	t.Parallel()

	offTopicItem := ItemInput{Title: "Local football league results announced", URL: "https://news.example.com/sports/1"}
	onTopicItem := ItemInput{Title: "New student visa rules published", URL: "https://news.example.com/visa/2"}
	mapTopics := []string{"visa", "immigration"}

	t.Run("open always on_topic", func(t *testing.T) {
		ch := sources.Channel{RelevanceMode: string(sources.RelevanceOpen)}
		v, err := Evaluate(context.Background(), offTopicItem, ch, nil)
		if err != nil || v != VerdictOnTopic {
			t.Fatalf("open mode = %v, %v; want on_topic", v, err)
		}
	})

	t.Run("custom ignores map topics", func(t *testing.T) {
		// Channel topics do NOT include "visa"; the map does. Custom mode
		// must ignore the map → the visa item scores 0 → off_topic.
		ch := sources.Channel{
			RelevanceMode:   string(sources.RelevanceCustom),
			RelevanceTopics: []string{"football", "league"},
		}
		v, err := Evaluate(context.Background(), onTopicItem, ch, mapTopics)
		if err != nil || v != VerdictOffTopic {
			t.Fatalf("custom mode = %v, %v; want off_topic", v, err)
		}
	})

	t.Run("filtered unions map topics", func(t *testing.T) {
		ch := filteredChannel([]string{"football"}, nil)
		v, err := Evaluate(context.Background(), onTopicItem, ch, mapTopics)
		if err != nil || v != VerdictOnTopic {
			t.Fatalf("filtered mode = %v, %v; want on_topic", v, err)
		}
	})

	t.Run("unknown mode treated as filtered", func(t *testing.T) {
		ch := sources.Channel{RelevanceMode: "bogus", RelevanceTopics: []string{"visa"}}
		v, err := Evaluate(context.Background(), onTopicItem, ch, nil)
		if err != nil || v != VerdictOnTopic {
			t.Fatalf("unknown mode = %v, %v; want on_topic", v, err)
		}
	})
}

// ---------------------------------------------------------------------------
// Deterministic scorer thresholds
// ---------------------------------------------------------------------------

func TestEvaluate_ScorerThresholds(t *testing.T) {
	t.Parallel()

	topics := []string{"visa", "scholarship", "relocation"}
	ch := filteredChannel(topics, nil)

	t.Run("clear hit → on_topic", func(t *testing.T) {
		// "visa" in a 4-token title: hits 2 / total 8 = 0.25 ≥ 0.15.
		item := ItemInput{Title: "Visa requirements for Nigerians"}
		v, err := Evaluate(context.Background(), item, ch, nil)
		if err != nil || v != VerdictOnTopic {
			t.Fatalf("score = %v, %v; want on_topic", v, err)
		}
	})

	t.Run("no hits → off_topic", func(t *testing.T) {
		item := ItemInput{Title: "Local football league results", Summary: "Arsenal won the derby again."}
		v, err := Evaluate(context.Background(), item, ch, nil)
		if err != nil || v != VerdictOffTopic {
			t.Fatalf("score = %v, %v; want off_topic", v, err)
		}
	})

	t.Run("diluted hit → borderline", func(t *testing.T) {
		// One summary hit in a long body: 1 / (title 2*5 + summary ~14) ≈ 0.04 → band.
		item := ItemInput{
			Title:   "Weekly policy roundup published today",
			Summary: "The ministry released new guidance covering permits and the visa bulletin alongside other routine announcements for residents.",
		}
		g := Gate{} // no classifier → Borderline stands
		v, err := g.Evaluate(context.Background(), item, ch, nil)
		if err != nil || v != VerdictBorderline {
			t.Fatalf("score = %v, %v; want borderline", v, err)
		}
	})

	t.Run("phrase match is word-boundary aware", func(t *testing.T) {
		// "advisory" must not match topic "visa"; multi-word topics match
		// only as contiguous runs.
		item := ItemInput{Title: "Travel advisory issued for visage artists"}
		v, err := Evaluate(context.Background(), item, filteredChannel([]string{"visa"}, nil), nil)
		if err != nil || v != VerdictOffTopic {
			t.Fatalf("score = %v, %v; want off_topic (no false phrase hit)", v, err)
		}
	})

	t.Run("multi-word topic phrase", func(t *testing.T) {
		item := ItemInput{Title: "New residence permit process opens"}
		v, err := Evaluate(context.Background(), item, filteredChannel([]string{"residence permit"}, nil), nil)
		if err != nil || v != VerdictOnTopic {
			t.Fatalf("score = %v, %v; want on_topic", v, err)
		}
	})

	t.Run("url path tokens count", func(t *testing.T) {
		// No title/summary text signal, but the URL path carries "visa".
		item := ItemInput{Title: "Just in", URL: "https://news.example.com/visa/bulletin"}
		v, err := Evaluate(context.Background(), item, filteredChannel([]string{"visa"}, nil), nil)
		if err != nil || v != VerdictOnTopic {
			t.Fatalf("score = %v, %v; want on_topic", v, err)
		}
	})

	t.Run("no topics and no patterns → on_topic", func(t *testing.T) {
		v, err := Evaluate(context.Background(), ItemInput{Title: "anything"}, filteredChannel(nil, nil), nil)
		if err != nil || v != VerdictOnTopic {
			t.Fatalf("unconfigured gate = %v, %v; want on_topic", v, err)
		}
	})

	t.Run("patterns configured but no topics → pattern miss is off_topic", func(t *testing.T) {
		ch := filteredChannel(nil, []string{"example.com/visa/*"})
		v, err := Evaluate(context.Background(), ItemInput{Title: "anything", URL: "https://example.com/sports/1"}, ch, nil)
		if err != nil || v != VerdictOffTopic {
			t.Fatalf("pattern-only channel = %v, %v; want off_topic", v, err)
		}
	})

	t.Run("empty item → borderline not off_topic", func(t *testing.T) {
		v, err := Evaluate(context.Background(), ItemInput{}, filteredChannel([]string{"visa"}, nil), nil)
		if err != nil || v != VerdictBorderline {
			t.Fatalf("empty item = %v, %v; want borderline", v, err)
		}
	})
}

// ---------------------------------------------------------------------------
// Borderline band → classifier, cache, cap
// ---------------------------------------------------------------------------

// borderlineItem lands in the band for topics ["visa"]: title 3 tokens
// (weight 2 → 6) + summary 8 tokens + url 2 tokens ≈ 16 total; one
// summary hit = 1/16 ≈ 0.06 → between 0.03 and 0.15.
func borderlineItem() ItemInput {
	return ItemInput{
		Title:   "Weekly ministry roundup",
		Summary: "New guidance covers permits and the visa bulletin plus routine notices for residents.",
		URL:     "https://news.example.com/policy/weekly",
	}
}

func TestEvaluate_BorderlineClassifier(t *testing.T) {
	t.Parallel()

	t.Run("classifier on_topic verdict", func(t *testing.T) {
		cls := &fakeClassifier{onTopic: true}
		g := Gate{ClassifierEnabled: true, BorderlineRateCap: 5, Classifier: cls, Cache: newFakeCache()}

		v, err := g.Evaluate(context.Background(), borderlineItem(), filteredChannel([]string{"visa"}, nil), nil)
		if err != nil || v != VerdictOnTopic {
			t.Fatalf("Evaluate = %v, %v; want on_topic", v, err)
		}
		if cls.calls != 1 {
			t.Fatalf("classifier calls = %d, want 1", cls.calls)
		}
	})

	t.Run("classifier off_topic verdict", func(t *testing.T) {
		cls := &fakeClassifier{onTopic: false}
		g := Gate{ClassifierEnabled: true, BorderlineRateCap: 5, Classifier: cls, Cache: newFakeCache()}

		v, err := g.Evaluate(context.Background(), borderlineItem(), filteredChannel([]string{"visa"}, nil), nil)
		if err != nil || v != VerdictOffTopic {
			t.Fatalf("Evaluate = %v, %v; want off_topic", v, err)
		}
	})

	t.Run("verdict cached — second call skips classifier", func(t *testing.T) {
		cls := &fakeClassifier{onTopic: true}
		cache := newFakeCache()
		ch := filteredChannel([]string{"visa"}, nil)
		ch.ID = chID(t, "11111111-1111-4111-8111-111111111111")
		g := Gate{ClassifierEnabled: true, BorderlineRateCap: 5, Classifier: cls, Cache: cache}

		item := borderlineItem()
		if v, _ := g.Evaluate(context.Background(), item, ch, nil); v != VerdictOnTopic {
			t.Fatalf("first Evaluate = %v, want on_topic", v)
		}
		if v, _ := g.Evaluate(context.Background(), item, ch, nil); v != VerdictOnTopic {
			t.Fatalf("second Evaluate = %v, want on_topic (cached)", v)
		}
		if cls.calls != 1 {
			t.Fatalf("classifier calls = %d, want 1 (verdict cached)", cls.calls)
		}

		// The cache key must be the spec's verdict key shape.
		found := false
		for k := range cache.vals {
			if len(k) > len("jc:intel:relevance:verdict:") && k[:len("jc:intel:relevance:verdict:")] == "jc:intel:relevance:verdict:" {
				found = true
			}
		}
		if !found {
			t.Error("no jc:intel:relevance:verdict:{hash} key written")
		}
	})

	t.Run("cap reached → classifier skipped", func(t *testing.T) {
		cls := &fakeClassifier{onTopic: true}
		cache := newFakeCache()
		ch := filteredChannel([]string{"visa"}, nil)
		ch.ID = chID(t, "22222222-2222-4222-8222-222222222222")
		g := Gate{ClassifierEnabled: true, BorderlineRateCap: 2, Classifier: cls, Cache: cache}

		// Distinct items so the verdict cache never short-circuits.
		for i := 0; i < 4; i++ {
			item := borderlineItem()
			item.URL = fmt.Sprintf("https://news.example.com/policy/item-%d", i)

			v, err := g.Evaluate(context.Background(), item, ch, nil)
			if err != nil {
				t.Fatalf("Evaluate %d: %v", i, err)
			}
			want := VerdictOnTopic
			if i >= 2 { // calls 3,4 exceed cap=2 → Borderline
				want = VerdictBorderline
			}
			if v != want {
				t.Fatalf("Evaluate %d = %v, want %v", i, v, want)
			}
		}
		if cls.calls != 2 {
			t.Fatalf("classifier calls = %d, want 2 (cap)", cls.calls)
		}
	})

	t.Run("classifier disabled → borderline passes through", func(t *testing.T) {
		cls := &fakeClassifier{onTopic: true}
		g := Gate{ClassifierEnabled: false, BorderlineRateCap: 5, Classifier: cls, Cache: newFakeCache()}

		v, err := g.Evaluate(context.Background(), borderlineItem(), filteredChannel([]string{"visa"}, nil), nil)
		if err != nil || v != VerdictBorderline {
			t.Fatalf("Evaluate = %v, %v; want borderline", v, err)
		}
		if cls.calls != 0 {
			t.Fatalf("classifier calls = %d, want 0", cls.calls)
		}
	})

	t.Run("classifier error → borderline + error (fail-open)", func(t *testing.T) {
		cls := &fakeClassifier{err: errors.New("gateway down")}
		g := Gate{ClassifierEnabled: true, BorderlineRateCap: 5, Classifier: cls, Cache: newFakeCache()}

		v, err := g.Evaluate(context.Background(), borderlineItem(), filteredChannel([]string{"visa"}, nil), nil)
		if err == nil {
			t.Fatal("classifier error not propagated")
		}
		if v != VerdictBorderline {
			t.Fatalf("Evaluate verdict = %v, want borderline", v)
		}
	})

	t.Run("nil cache → classifier skipped (cap unenforceable)", func(t *testing.T) {
		cls := &fakeClassifier{onTopic: true}
		g := Gate{ClassifierEnabled: true, BorderlineRateCap: 5, Classifier: cls}

		v, err := g.Evaluate(context.Background(), borderlineItem(), filteredChannel([]string{"visa"}, nil), nil)
		if err != nil || v != VerdictBorderline {
			t.Fatalf("Evaluate = %v, %v; want borderline", v, err)
		}
		if cls.calls != 0 {
			t.Fatalf("classifier calls = %d, want 0", cls.calls)
		}
	})

	t.Run("counter failure → classifier skipped", func(t *testing.T) {
		cls := &fakeClassifier{onTopic: true}
		cache := &fakeCache{vals: map[string]string{}, counters: map[string]int64{}, getErr: nil}
		// Force Incr failure by a broken cache: use a wrapper.
		broken := &brokenIncrCache{inner: cache}
		g := Gate{ClassifierEnabled: true, BorderlineRateCap: 5, Classifier: cls, Cache: broken}

		v, err := g.Evaluate(context.Background(), borderlineItem(), filteredChannel([]string{"visa"}, nil), nil)
		if err != nil || v != VerdictBorderline {
			t.Fatalf("Evaluate = %v, %v; want borderline", v, err)
		}
		if cls.calls != 0 {
			t.Fatalf("classifier calls = %d, want 0", cls.calls)
		}
	})
}

// brokenIncrCache wraps fakeCache with an Incr that always fails.
type brokenIncrCache struct{ inner *fakeCache }

func (b *brokenIncrCache) Get(ctx context.Context, k string) (string, error) {
	return b.inner.Get(ctx, k)
}
func (b *brokenIncrCache) Set(ctx context.Context, k, v string, ttl time.Duration) error {
	return b.inner.Set(ctx, k, v, ttl)
}
func (b *brokenIncrCache) Incr(context.Context, string) (int64, error) {
	return 0, errors.New("valkey down")
}
func (b *brokenIncrCache) Expire(context.Context, string, time.Duration) error { return nil }

// ---------------------------------------------------------------------------
// ValkeyCache adapter (miniredis)
// ---------------------------------------------------------------------------

func TestValkeyCache(t *testing.T) {
	mr := miniredis.RunT(t)

	vk, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{mr.Addr()}, DisableCache: true})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	defer vk.Close()

	c := ValkeyCache{Rdb: valkeycompat.NewAdapter(vk)}
	ctx := context.Background()

	if _, err := c.Get(ctx, "missing"); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("Get(missing) err = %v, want ErrCacheMiss", err)
	}
	if err := c.Set(ctx, "k", "on_topic", time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, err := c.Get(ctx, "k"); err != nil || v != "on_topic" {
		t.Fatalf("Get = %q, %v; want on_topic", v, err)
	}
	if n, err := c.Incr(ctx, "counter"); err != nil || n != 1 {
		t.Fatalf("Incr = %d, %v; want 1", n, err)
	}
	if n, err := c.Incr(ctx, "counter"); err != nil || n != 2 {
		t.Fatalf("Incr = %d, %v; want 2", n, err)
	}
	if err := c.Expire(ctx, "counter", time.Minute); err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if ttl := mr.TTL("counter"); ttl <= 0 {
		t.Errorf("counter TTL = %v, want > 0", ttl)
	}
}

// TestEvaluate_EndToEndWithValkey runs the full gate against miniredis:
// borderline → classifier → cached verdict, and the cap counter key lands
// under the spec's jc:intel:relevance:crawl:{channel_id}:{date} shape.
func TestEvaluate_EndToEndWithValkey(t *testing.T) {
	mr := miniredis.RunT(t)

	vk, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{mr.Addr()}, DisableCache: true})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	defer vk.Close()

	cls := &fakeClassifier{onTopic: false}
	ch := filteredChannel([]string{"visa"}, nil)
	ch.ID = chID(t, "33333333-3333-4333-8333-333333333333")
	g := Gate{
		ClassifierEnabled: true, BorderlineRateCap: 10,
		Classifier: cls, Cache: ValkeyCache{Rdb: valkeycompat.NewAdapter(vk)},
		Now: func() time.Time { return time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC) },
	}

	item := borderlineItem()

	if v, err := g.Evaluate(context.Background(), item, ch, nil); err != nil || v != VerdictOffTopic {
		t.Fatalf("first Evaluate = %v, %v; want off_topic", v, err)
	}
	if v, err := g.Evaluate(context.Background(), item, ch, nil); err != nil || v != VerdictOffTopic {
		t.Fatalf("second Evaluate = %v, %v; want off_topic (cached)", v, err)
	}
	if cls.calls != 1 {
		t.Fatalf("classifier calls = %d, want 1", cls.calls)
	}

	wantCounter := "jc:intel:relevance:crawl:33333333-3333-4333-8333-333333333333:2026-09-20"
	if !mr.Exists(wantCounter) {
		t.Errorf("counter key %q not created; keys=%v", wantCounter, mr.Keys())
	}
}
