// Package scheduler selects due intelligence sources/channels for crawling
// and auto-tunes their effective crawl interval based on observed content
// volatility (source idea §17, Phase 3 Task 20; intel-channels Phase 1 moved
// the scheduling granularity to channels).
// // The volatility auto-tuner stores a per-crawl-unit effective interval in
// Valkey under `jc:intel:effective_interval::{id}` (TTL 7d) — keyed by
// channel_id on the channel path, source_id on the legacy source path.
// After 5 consecutive unchanged crawls the interval is doubled (capped at
// MaxEffectiveIntervalMins); after any change the interval is halved (floored
// at the channel's/source's base crawl_frequency_mins). A separate counter key
// `jc:intel:effective_interval::{id}:unchanged_count`
// tracks the consecutive-unchanged streak so the value survives restarts.
// All keys are platform-scoped per ADR-112 Amendment 2.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// DueSourceLister is the subset of sources.Store the scheduler needs to
// select sources whose crawl interval has elapsed. Declaring it as an
// interface keeps the scheduler testable without a live *db.DB.
type DueSourceLister interface {
	ListDue(ctx context.Context, limit int32) ([]gen.IntelSource, error)
}

// DueChannelLister is the subset of sources.Store the scheduler needs to
// select channels whose crawl interval has elapsed (intel-channels Phase 1
// — scheduling granularity moved from source to channel). *sources.Store
// satisfies it via ListDueChannels, which joins intel_sources and already
// excludes manual/inbound channels and disabled/inactive parents.
type DueChannelLister interface {
	ListDueChannels(ctx context.Context, limit int32) ([]gen.IntelSourceChannel, error)
}

// MaxEffectiveIntervalMins caps how far the auto-tuner can stretch a source's
// effective interval. Once a source is this stable, further unchanged crawls
// no longer lengthen the interval.
const MaxEffectiveIntervalMins = 24 * 60 // 24 hours

// UnchangedThreshold is the number of consecutive unchanged crawls required
// before the effective interval is doubled.
const UnchangedThreshold = 5

// effectiveIntervalTTL is the TTL applied to the stored effective interval so
// stale tuning state eventually self-cleans if a source is deleted.
const effectiveIntervalTTL = 7 * 24 * time.Hour

// Scheduler picks due sources and records crawl outcomes so the volatility
// auto-tuner can adjust each source's effective interval over time.
type Scheduler struct {
	sourceStore  DueSourceLister
	channelStore DueChannelLister
	rdb          valkeycompat.Cmdable
}

// NewScheduler returns a Scheduler backed by the given source/channel stores
// and Valkey (go-redis-compatible) client. *sources.Store satisfies both
// lister interfaces, so the concrete store can be passed for both arguments.
// The ID scopes all Valkey keys per ADR-112 Amendment 2.
// // channelLister may be nil for source-only callers (SelectDueChannels
// then returns an error).
func NewScheduler(sourceStore DueSourceLister, channelLister DueChannelLister, rdb valkeycompat.Cmdable) *Scheduler {
	return &Scheduler{sourceStore: sourceStore, channelStore: channelLister, rdb: rdb}
}

// dueOverFetch multiplies the SQL batch when the volatility-tuner filter is
// active: the SQL due check tests the static crawl_frequency_mins, but the
// tuner may have widened a channel's effective interval in Valkey — those
// rows stay "due" in SQL and are filtered out in Go, so the query over-
// fetches to keep truly-due rows flowing past tuner-suppressed ones.
const dueOverFetch = 4

// SelectDue returns up to batchSize enabled sources whose effective crawl
// interval has elapsed (or that have never been crawled), oldest-first. The
// SQL query uses the static crawl_frequency_mins; the volatility-tuned
// effective interval (jc:intel:effective_interval::{source_id}) is applied
// as a post-filter — effective ≥ base always, so statically-due is a
// superset of actually-due.
func (s *Scheduler) SelectDue(ctx context.Context, batchSize int32) ([]sources.Source, error) {
	rows, err := s.sourceStore.ListDue(ctx, batchSize*dueOverFetch)
	if err != nil {
		return nil, fmt.Errorf("scheduler: select due: %w", err)
	}

	return filterTuned(s, ctx, rows, batchSize, func(src sources.Source) (string, pgtype.Timestamptz, int32) {
		return src.ID.String(), src.LastCrawledAt, src.CrawlFrequencyMins
	}), nil
}

// SelectDueChannels returns up to batchSize enabled channels whose effective
// crawl interval has elapsed (or that have never been crawled), oldest-first —
// the intel-channels Phase 1 scheduling unit. The underlying
// ListDueIntelChannels query joins intel_sources so channels on disabled or
// non-active parents are excluded, and skips manual/email/webhook methods.
// The tuned effective interval (jc:intel:effective_interval::{channel_id})
// is applied as a post-filter over an over-fetched static batch.
func (s *Scheduler) SelectDueChannels(ctx context.Context, batchSize int32) ([]sources.Channel, error) {
	if s.channelStore == nil {
		return nil, errors.New("scheduler: select due channels: channel lister not wired")
	}

	rows, err := s.channelStore.ListDueChannels(ctx, batchSize*dueOverFetch)
	if err != nil {
		return nil, fmt.Errorf("scheduler: select due channels: %w", err)
	}

	return filterTuned(s, ctx, rows, batchSize, func(ch sources.Channel) (string, pgtype.Timestamptz, int32) {
		return ch.ID.String(), ch.LastCrawledAt, ch.CrawlFrequencyMins
	}), nil
}

// filterTuned drops rows whose volatility-tuned effective interval has not
// elapsed since their last crawl, returning at most batchSize. Never-crawled
// rows are always due. When Valkey is absent (tests) or a lookup errors the
// row is kept — a cache outage must never pause all crawling (fail-open,
// matching the codebase's resilience convention for tuning state).
func filterTuned[T any](s *Scheduler, ctx context.Context, rows []T, batchSize int32, fields func(T) (id string, lastCrawled pgtype.Timestamptz, baseMins int32)) []T {
	if s.rdb == nil {
		if len(rows) > int(batchSize) {
			return rows[:batchSize]
		}

		return rows
	}

	due := make([]T, 0, batchSize)

	for _, row := range rows {
		if len(due) >= int(batchSize) {
			break
		}

		id, lastCrawled, baseMins := fields(row)

		if !lastCrawled.Valid {
			due = append(due, row) // never crawled — always due
			continue
		}

		eff, err := s.EffectiveIntervalMins(ctx, id, baseMins)
		if err != nil {
			slog.Warn("scheduler: effective interval lookup failed, keeping row", "id", id, "error", err)

			due = append(due, row)

			continue
		}

		if time.Since(lastCrawled.Time) >= time.Duration(eff)*time.Minute {
			due = append(due, row)
		}
	}

	return due
}

// RecordCrawlOutcome updates the volatility auto-tuner state for a source
// after a crawl completes. If changed is true the content hash differed from
// the previous crawl; otherwise the source was crawled but unchanged.
// // Tuning rules:
// - unchanged: increment the consecutive-unchanged counter. When it reaches
// UnchangedThreshold, double the effective interval (capped at
// MaxEffectiveIntervalMins) and reset the counter.
// - changed: halve the effective interval, floored at the source's base
// crawl_frequency_mins, and reset the counter.
// // baseIntervalMins is the source's crawl_frequency_mins and acts as the
// floor for the effective interval.
func (s *Scheduler) RecordCrawlOutcome(ctx context.Context, sourceID string, baseIntervalMins int32, changed bool) error {
	if sourceID == "" {
		return errors.New("scheduler: record crawl outcome: empty source id")
	}

	if baseIntervalMins <= 0 {
		return fmt.Errorf("scheduler: record crawl outcome: non-positive base interval %d", baseIntervalMins)
	}

	intervalKey := intervalKey(sourceID)
	countKey := unchangedCountKey(sourceID)

	if changed {
		// A change signals volatility: halve the effective interval, floored
		// at the base interval, and reset the unchanged streak.
		if err := s.resetUnchangedAndHalve(ctx, intervalKey, countKey, baseIntervalMins); err != nil {
			return fmt.Errorf("scheduler: record crawl outcome (changed): %w", err)
		}

		return nil
	}

	// Unchanged: bump the streak counter.
	count, err := s.rdb.Incr(ctx, countKey).Result()
	if err != nil {
		return fmt.Errorf("scheduler: record crawl outcome: incr unchanged count: %w", err)
	}
	// Keep the counter key from lingering forever if a source goes dormant.
	if count == 1 {
		s.rdb.Expire(ctx, countKey, effectiveIntervalTTL)
	}

	// Not yet at the threshold — leave the effective interval as-is.
	if count < int64(UnchangedThreshold) {
		return nil
	}

	// Threshold reached: double the effective interval, then reset the streak.
	if err := s.doubleInterval(ctx, intervalKey, countKey, baseIntervalMins); err != nil {
		return fmt.Errorf("scheduler: record crawl outcome (unchanged): %w", err)
	}

	return nil
}

// EffectiveIntervalMins returns the currently tuned effective interval for a
// source, in minutes. If no tuning state exists yet (first crawl, or expired
// TTL), the source's base interval is returned so callers can fall back to
// the configured crawl_frequency_mins.
func (s *Scheduler) EffectiveIntervalMins(ctx context.Context, sourceID string, baseIntervalMins int32) (int32, error) {
	val, err := s.rdb.Get(ctx, intervalKey(sourceID)).Result()
	if err != nil {
		// Missing key → no tuning yet; use the base interval.
		if isRedisNil(err) {
			return baseIntervalMins, nil
		}

		return 0, fmt.Errorf("scheduler: effective interval: %w", err)
	}

	mins, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("scheduler: effective interval: parse %q: %w", val, err)
	}

	if mins > math.MaxInt32 {
		mins = math.MaxInt32
	}

	return int32(mins), nil // #nosec G109,G115 -- bounded by math.MaxInt32 check above
}

// doubleInterval doubles the stored effective interval, seeding it from the
// base interval when no value exists yet, and caps it at
// MaxEffectiveIntervalMins. The unchanged streak counter is reset.
func (s *Scheduler) doubleInterval(ctx context.Context, intervalKey, countKey string, baseIntervalMins int32) error {
	current, err := s.storedOrBase(ctx, intervalKey, baseIntervalMins)
	if err != nil {
		return err
	}

	doubled := current * 2
	if doubled > MaxEffectiveIntervalMins {
		doubled = MaxEffectiveIntervalMins
	}

	if doubled < baseIntervalMins {
		doubled = baseIntervalMins
	}

	if err := s.setInterval(ctx, intervalKey, doubled); err != nil {
		return err
	}
	// Reset the streak once the interval has been widened.
	s.rdb.Del(ctx, countKey)

	return nil
}

// resetUnchangedAndHalve halves the effective interval (floored at the base
// interval) and clears the unchanged streak counter.
func (s *Scheduler) resetUnchangedAndHalve(ctx context.Context, intervalKey, countKey string, baseIntervalMins int32) error {
	current, err := s.storedOrBase(ctx, intervalKey, baseIntervalMins)
	if err != nil {
		return err
	}

	halved := current / 2
	if halved < baseIntervalMins {
		halved = baseIntervalMins
	}

	if err := s.setInterval(ctx, intervalKey, halved); err != nil {
		return err
	}

	s.rdb.Del(ctx, countKey)

	return nil
}

// storedOrBase reads the current effective interval, falling back to the
// base interval when no tuning state exists yet.
func (s *Scheduler) storedOrBase(ctx context.Context, intervalKey string, baseIntervalMins int32) (int32, error) {
	val, err := s.rdb.Get(ctx, intervalKey).Result()
	if err != nil {
		if isRedisNil(err) {
			return baseIntervalMins, nil
		}

		return 0, fmt.Errorf("get effective interval: %w", err)
	}

	mins, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("parse effective interval %q: %w", val, err)
	}

	if mins <= 0 {
		return baseIntervalMins, nil
	}

	if mins > math.MaxInt32 {
		mins = math.MaxInt32
	}

	return int32(mins), nil // #nosec G109,G115 -- bounded by math.MaxInt32 check above
}

// setInterval writes the effective interval (in minutes) with the standard
// TTL so tuning state self-cleans if a source is removed.
func (s *Scheduler) setInterval(ctx context.Context, intervalKey string, mins int32) error {
	if err := s.rdb.Set(ctx, intervalKey, strconv.Itoa(int(mins)), effectiveIntervalTTL).Err(); err != nil {
		return fmt.Errorf("set effective interval: %w", err)
	}

	return nil
}

func intervalKey(sourceID string) string {
	return "jc:intel:effective_interval::" + sourceID
}

func unchangedCountKey(sourceID string) string {
	return fmt.Sprintf("jc:intel:effective_interval::%s:unchanged_count", sourceID)
}

// isRedisNil reports whether err is a "key not found" error from the
// go-redis-compatible adapter. The valkeycompat adapter surfaces a nil
// response as a *ValkeyError whose Error() is "valkey nil message"; the
// classic go-redis sentinel is "redis: nil". Both are accepted so the
// scheduler works against either backend without importing either sentinel
// directly.
func isRedisNil(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	return msg == "valkey nil message" || msg == "redis: nil"
}
