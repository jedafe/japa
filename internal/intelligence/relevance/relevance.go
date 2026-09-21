// Package relevance implements the per-item topicality gate for the
// intel-channels Phase 1 pipeline (spec §4 "Relevance gate"). Multi-niche
// sources (NYT, Nairaland) publish content far outside JapaConnect's
// domain; the gate filters each curated item or extracted record so
// off-topic items are stamped intel_records.status='off_topic' and never
// consume dedup/publish attention.
//
// Evaluation order per item (title + summary + url):
//
//  1. relevance_url_patterns — glob match on the item URL → on_topic.
//  2. Deterministic topic scorer — word-boundary phrase matching over the
//     channel's relevance_topics (filtered mode unions in the
//     category-map topics; custom mode ignores them). Density score ≥
//     on_topic_threshold → on_topic; < off_topic_threshold → off_topic.
//  3. Borderline band → the Classifier port (§20 AI gateway,
//     category="classification"), bounded by a per-channel-per-day rate
//     cap counter (jc:intel:relevance:crawl:{channel_id}:{date}) and
//     cached under jc:intel:relevance:verdict:{sha256(channel_id|title|url)} (7d).
//
// Modes (channel.relevance_mode): open → everything passes; filtered →
// channel topics ∪ category-map topics + classifier; custom → only the
// channel's own topics/patterns. Unknown/empty modes are treated as
// filtered (the schema default).
//
// The Classifier port is satisfied by a server.go adapter over the AI
// dispatcher — this package never imports internal/ai. The Cache port is
// satisfied by ValkeyCache (valkeycompat.Cmdable) or a test fake.
package relevance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Verdict is the gate's per-item decision. OffTopic is the only verdict
// that drops an item; Borderline means the deterministic scorer could not
// resolve the item and no classifier verdict applied — callers treat it
// as pass-through (fail-open keeps recall when the classifier is
// disabled, capped out, or errors).
type Verdict string

// Exported const values.
const (
	VerdictOnTopic    Verdict = "on_topic"
	VerdictOffTopic   Verdict = "off_topic"
	VerdictBorderline Verdict = "borderline"
)

// ItemInput is the scorer's view of one curated item or extracted record.
// Workers derive it from the record's extraction fields
// (title/name/headline, summary/description, article_url/url/…).
type ItemInput struct {
	Title   string
	Summary string
	URL     string
}

// Classifier is the borderline-band AI port (spec §4 step 3): "is this
// item about migration/relocation/travel/study-abroad/jobs/scholarships/
// settlement?" topics carries the effective topic vocabulary so the
// adapter can name the channel's domain. Returns onTopic=true for a
// relevant item. Errors fail open — the gate returns Borderline so the
// caller passes the item through.
type Classifier interface {
	Classify(ctx context.Context, item ItemInput, topics []string) (onTopic bool, err error)
}

// Cache backs the verdict cache and the per-crawl classifier rate
// counter. ValkeyCache adapts valkeycompat.Cmdable; tests substitute an
// in-memory fake. All methods must tolerate missing keys — Get reports
// ErrCacheMiss.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

// ErrCacheMiss is the Cache.Get miss sentinel. ValkeyCache maps
// valkey.Nil onto it; fakes return it directly.
var ErrCacheMiss = errors.New("relevance: cache miss")

// Scoring/threshold defaults. The score is a weighted topic-density:
// sum of matched topic phrase-token weights (title hits count double,
// summary and URL-path hits single; each topic counts once per field)
// over the same weighting of the item's total token count. Defaults match
// the spec's intent — a single topic hit in a short title clears 0.15,
// while a stray keyword buried in a long summary stays below the
// borderline band's top edge.
const (
	// DefaultOnTopicThreshold is the deterministic-match score cutoff
	// (intel.relevance.on_topic_threshold).
	DefaultOnTopicThreshold = 0.15
	// DefaultOffTopicThreshold is the deterministic-reject score cutoff
	// (intel.relevance.off_topic_threshold) — scores strictly below are
	// off_topic, so the default keeps zero-hit items rejected.
	DefaultOffTopicThreshold = 0.03
	// DefaultBorderlineRateCap is the max classifier calls per channel
	// per day (intel.relevance.borderline_rate_cap, spec default 50).
	DefaultBorderlineRateCap = 50

	// VerdictCacheTTL is the jc:intel:relevance:verdict:{hash} lifetime
	// (7d per valkey-keys-catalog Section INTEL).
	VerdictCacheTTL = 7 * 24 * time.Hour
	// crawlCounterTTL bounds the per-channel-per-day rate-cap counter —
	// generous enough to span a full day boundary window.
	crawlCounterTTL = 48 * time.Hour
)

// Gate is a configured relevance evaluator. Build one per handler
// invocation so hot-reloadable system_settings (thresholds, classifier
// toggle, rate cap) apply to every crawl.
type Gate struct {
	// OnTopicThreshold is the deterministic-match cutoff; <=0 resolves
	// to DefaultOnTopicThreshold.
	OnTopicThreshold float64
	// OffTopicThreshold is the deterministic-reject cutoff; <=0 resolves
	// to DefaultOffTopicThreshold.
	OffTopicThreshold float64
	// ClassifierEnabled gates the borderline band's AI call
	// (intel.relevance.classifier_enabled, spec default true).
	ClassifierEnabled bool
	// BorderlineRateCap bounds classifier calls per channel per day
	// (intel.relevance.borderline_rate_cap). <=0 means zero calls — the
	// borderline band stays Borderline (fail-open).
	BorderlineRateCap int
	// Classifier is the borderline-band port; nil leaves borderline
	// items Borderline.
	Classifier Classifier
	// Cache backs the verdict cache + rate counter; nil disables both —
	// the borderline path then skips the classifier entirely since the
	// per-day cap cannot be enforced (spend-safety over recall).
	Cache Cache
	// Now overrides the clock for the rate-counter day bucket (tests).
	Now func() time.Time
}

// Evaluate runs the zero-config gate (defaults, no classifier, no cache)
// — deterministic scoring only. Production callers build a Gate.
func Evaluate(ctx context.Context, item ItemInput, ch sources.Channel, mapTopics []string) (Verdict, error) {
	return (Gate{}).Evaluate(ctx, item, ch, mapTopics)
}

// Evaluate applies the spec §4 evaluation order to one item. mapTopics
// carries the resolved category-map topics (sources.ResolveCategoryMap
// for the parent source's categories); it is ignored in custom mode.
// A nil/absent verdict path always resolves to a concrete Verdict —
// errors are returned only for classifier failures (verdict stays
// Borderline so the caller can fail open).
func (g Gate) Evaluate(ctx context.Context, item ItemInput, ch sources.Channel, mapTopics []string) (Verdict, error) {
	mode := sources.RelevanceMode(ch.RelevanceMode)
	if mode == sources.RelevanceOpen {
		return VerdictOnTopic, nil
	}
	// Unknown/empty modes fall through as 'filtered' — the schema CHECK
	// constrains stored values, and the spec default is filtered.

	// Step 1: relevance_url_patterns glob match on the item URL.
	hasPatterns := false
	for _, p := range ch.RelevanceUrlPatterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		hasPatterns = true
		if matchURLPattern(p, item.URL) {
			return VerdictOnTopic, nil
		}
	}

	// Step 2: deterministic topic scorer. Custom mode uses only the
	// channel's own topics; filtered (and unknown) union in mapTopics.
	topics := dedupTopics(ch.RelevanceTopics)
	if mode != sources.RelevanceCustom {
		topics = dedupTopics(append(topics, mapTopics...))
	}
	if len(topics) == 0 {
		// Nothing to score against. When patterns are configured the
		// channel explicitly whitelists URLs — a miss is off-topic.
		// With neither vocabulary nor patterns there is nothing to gate
		// on, so the item passes (a topic-less filtered channel must not
		// drop everything).
		if hasPatterns {
			return VerdictOffTopic, nil
		}

		return VerdictOnTopic, nil
	}

	score, scorable := scoreItem(item, topics)
	if !scorable {
		// No text to evaluate — unevaluable rather than off-topic; let
		// the borderline path (classifier / fail-open) decide.
		return g.borderline(ctx, item, ch, topics)
	}

	onTh, offTh := g.thresholds()

	switch {
	case score >= onTh:
		return VerdictOnTopic, nil
	case score < offTh:
		return VerdictOffTopic, nil
	default:
		return g.borderline(ctx, item, ch, topics)
	}
}

// thresholds resolves the configured cutoffs, applying defaults for
// non-positive values.
func (g Gate) thresholds() (on, off float64) {
	on = g.OnTopicThreshold
	if on <= 0 {
		on = DefaultOnTopicThreshold
	}
	off = g.OffTopicThreshold
	if off <= 0 {
		off = DefaultOffTopicThreshold
	}
	if off > on {
		// Pathological config (off >= on collapses the band) — clamp so
		// the band order holds: below off → off_topic, else on_topic.
		off = on
	}

	return on, off
}

// borderline resolves the borderline band: cached verdict → classifier
// (bounded by the per-channel-per-day cap) → Borderline fail-open.
func (g Gate) borderline(ctx context.Context, item ItemInput, ch sources.Channel, topics []string) (Verdict, error) {
	cacheKey := verdictCacheKey(item, ch)

	if g.Cache != nil {
		if cached, err := g.Cache.Get(ctx, cacheKey); err == nil {
			switch Verdict(cached) {
			case VerdictOnTopic:
				return VerdictOnTopic, nil
			case VerdictOffTopic:
				return VerdictOffTopic, nil
			}
			// Unknown cached value — ignore and reclassify.
		}
	}

	if !g.ClassifierEnabled || g.Classifier == nil {
		return VerdictBorderline, nil
	}

	// Rate cap: without a counter store the cap cannot be enforced — a
	// nil Cache therefore skips the classifier entirely (spend-safety
	// beats recall for an optional refinement).
	if g.Cache == nil {
		return VerdictBorderline, nil
	}

	capKey := g.crawlCounterKey(ch)

	n, err := g.Cache.Incr(ctx, capKey)
	if err != nil {
		// Counter failure must not open an unbounded classifier spout.
		slog.Warn("relevance: rate-cap counter failed, skipping classifier", "key", capKey, "error", err)

		return VerdictBorderline, nil
	}
	if n == 1 {
		_ = g.Cache.Expire(ctx, capKey, crawlCounterTTL)
	}
	if cap := int64(g.BorderlineRateCap); cap <= 0 || n > cap {
		return VerdictBorderline, nil
	}

	onTopic, err := g.Classifier.Classify(ctx, item, topics)
	if err != nil {
		return VerdictBorderline, err
	}

	verdict := VerdictOffTopic
	if onTopic {
		verdict = VerdictOnTopic
	}

	if err := g.Cache.Set(ctx, cacheKey, string(verdict), VerdictCacheTTL); err != nil {
		slog.Warn("relevance: verdict cache write failed", "key", cacheKey, "error", err)
	}

	return verdict, nil
}

// verdictCacheKey builds jc:intel:relevance:verdict:{sha256(channel_id|title|url)}.
// The verdict is channel-scoped: the classifier answers "is this item
// on-topic for THIS channel's vocabulary", so the same title+url must not
// inherit another channel's verdict. When no channel row exists the
// worker seeds ch.ID with the source id — the scope degrades to
// per-source, never cross-source.
func verdictCacheKey(item ItemInput, ch sources.Channel) string {
	h := sha256.Sum256([]byte(ch.ID.String() + "|" + item.Title + "|" + item.URL))

	return "jc:intel:relevance:verdict:" + hex.EncodeToString(h[:])
}

// crawlCounterKey builds jc:intel:relevance:crawl:{channel_id}:{date} —
// the per-channel-per-day classifier call budget. The channel id doubles
// as the crawl scope; for channel-less (legacy) evaluations callers stamp
// the source id into ch.ID so the cap stays per-source.
func (g Gate) crawlCounterKey(ch sources.Channel) string {
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	scope := ch.ID.String()
	if scope == "" {
		scope = "no-channel"
	}

	return "jc:intel:relevance:crawl:" + scope + ":" + now().UTC().Format("2006-01-02")
}

// ---------------------------------------------------------------------------
// Deterministic scorer
// ---------------------------------------------------------------------------

// scoreItem returns the item's weighted topic-density in [0,1]:
//
//	score = Σ (per-topic matched phrase-token weight) / weighted token total
//
// where title tokens weigh 2× and summary/URL-path tokens weigh 1×, and
// each topic contributes its token length once per field it appears in
// (a topic hit in both title and summary counts twice). A topic that
// tokenizes to nothing is skipped. scorable=false means the item carried
// no usable text at all — the caller routes it to the borderline band
// rather than treating silence as off-topic.
func scoreItem(item ItemInput, topics []string) (score float64, scorable bool) {
	titleTok := tokenize(item.Title)
	sumTok := tokenize(item.Summary)
	urlTok := urlTokens(item.URL)

	total := 2*len(titleTok) + len(sumTok) + len(urlTok)
	if total == 0 {
		return 0, false
	}

	var hits float64

	for _, topic := range topics {
		tt := tokenize(topic)
		if len(tt) == 0 {
			continue
		}

		n := float64(len(tt))
		if containsPhrase(titleTok, tt) {
			hits += 2 * n
		}
		if containsPhrase(sumTok, tt) {
			hits += n
		}
		if containsPhrase(urlTok, tt) {
			hits += n
		}
	}

	return hits / float64(total), true
}

// tokenize splits s into lowercase alphanumeric tokens (letters + digits
// survive, everything else is a separator) so phrase matching is
// word-boundary aware — topic "visa" does not match inside "advisory"
// tokens. Unicode letters are kept so accented vocabulary still matches.
func tokenize(s string) []string {
	var out []string

	var b strings.Builder
	b.Grow(len(s))

	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}

	return out
}

// containsPhrase reports whether needle appears as a contiguous token run
// inside haystack.
func containsPhrase(haystack, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}

	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}

	return false
}

// urlTokens extracts tokens from the URL path and query (spec: "url path
// tokens") — the host is excluded so a domain like nairaland.com does not
// count as signal. Unparseable input is tokenized verbatim as a
// best-effort fallback.
func urlTokens(rawURL string) []string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil {
		return tokenize(rawURL)
	}

	return tokenize(u.Path + " " + u.RawQuery)
}

// dedupTopics normalizes and dedupes a topic list preserving first-seen
// order (blank entries dropped).
func dedupTopics(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))

	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}

	return out
}

// ---------------------------------------------------------------------------
// URL pattern matcher (spec §4 step 1)
// ---------------------------------------------------------------------------
//
// doublestar is not a dependency, so patterns use a deliberately small
// glob dialect documented for admins:
//
//   - matching is case-insensitive and scheme-insensitive — the pattern
//     and the item URL are lowercased and a leading http(s):// is stripped
//     from both, so "nairaland.com/travel/*" matches
//     "https://www.nairaland.com/travel/1234-visa-guide";
//   - a leading "www." on the URL is tolerated (the pattern may write the
//     bare domain);
//   - '*' matches ANY run of characters including '/' (so
//     "*/travel/*" spans host + depth); '?' matches one character;
//   - a pattern with no wildcards is a substring test — "nairaland.com/
//     travel" whitelists every URL containing that path prefix.
var globChar = regexp.MustCompile(`[*?]`)

// matchURLPattern reports whether the item URL matches one
// relevance_url_patterns entry.
func matchURLPattern(pattern, rawURL string) bool {
	p := normalizePatternURL(pattern)
	u := normalizePatternURL(rawURL)
	if p == "" || u == "" {
		return false
	}

	targets := []string{u}
	if rest, ok := strings.CutPrefix(u, "www."); ok {
		targets = append(targets, rest)
	} else {
		targets = append(targets, "www."+u)
	}

	if !globChar.MatchString(p) {
		for _, t := range targets {
			if strings.Contains(t, p) {
				return true
			}
		}

		return false
	}

	re := globRegexp(p)
	for _, t := range targets {
		if re.MatchString(t) {
			return true
		}
	}

	return false
}

// normalizePatternURL lowercases and strips the scheme so patterns are
// written without the protocol.
func normalizePatternURL(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}

	return s
}

// globRegexp translates the small glob dialect to an anchored regexp.
func globRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteByte('^')

	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteByte('$')

	// Pattern text is admin-controlled config; a compile failure is
	// impossible by construction (everything else is QuoteMeta'd) but a
	// fallback never matches.
	re, err := regexp.Compile(b.String())
	if err != nil {
		return regexp.MustCompile(`^$`)
	}

	return re
}

// ---------------------------------------------------------------------------
// Valkey adapter
// ---------------------------------------------------------------------------

// Cmdable is the Valkey command subset ValkeyCache needs. Both
// valkeycompat.Cmdable and the worker's narrower redisCache interface
// satisfy it structurally.
type Cmdable interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *valkeycompat.StatusCmd
	Incr(ctx context.Context, key string) *valkeycompat.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *valkeycompat.BoolCmd
}

// ValkeyCache adapts a Valkey client (the worker's rdb) to the Cache
// port. valkey.Nil maps to ErrCacheMiss.
type ValkeyCache struct {
	Rdb Cmdable
}

// Get fetches a cached verdict.
func (c ValkeyCache) Get(ctx context.Context, key string) (string, error) {
	s, err := c.Rdb.Get(ctx, key).Result()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return "", ErrCacheMiss
		}

		return "", err
	}

	return s, nil
}

// Set stores a verdict with TTL.
func (c ValkeyCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.Rdb.Set(ctx, key, value, ttl).Err()
}

// Incr increments the per-crawl classifier counter.
func (c ValkeyCache) Incr(ctx context.Context, key string) (int64, error) {
	return c.Rdb.Incr(ctx, key).Result()
}

// Expire sets the counter TTL.
func (c ValkeyCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.Rdb.Expire(ctx, key, ttl).Err()
}
