// Package scheduler selects due intelligence sources for crawling and
// auto-tunes their effective crawl interval based on observed content
// volatility (source idea §17, Phase 3 Task 20).
//
// The volatility auto-tuner stores a per-source effective interval in Valkey
// under `jc:intel:effective_interval:{source_id}` (TTL 7d). After 5
// consecutive unchanged crawls the interval is doubled (capped at
// MaxEffectiveIntervalMins); after any change the interval is halved (floored
// at the source's base crawl_frequency_mins). A separate counter key
// `jc:intel:effective_interval:{source_id}:unchanged_count` tracks the
// consecutive-unchanged streak so the value survives restarts.
package scheduler

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// DueSourceLister is the subset of sources.Store the scheduler needs to
// select sources whose crawl interval has elapsed. Declaring it as an
// interface keeps the scheduler testable without a live *db.DB.
type DueSourceLister interface {
	ListDueIntelSources(ctx context.Context, limit int32) ([]gen.IntelSource, error)
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
	sourceStore DueSourceLister
	rdb         valkeycompat.Cmdable
}

// NewScheduler returns a Scheduler backed by the given source store and
// Valkey (go-redis-compatible) client. A *sources.Store satisfies
// DueSourceLister, so the concrete store can be passed directly.
func NewScheduler(sourceStore DueSourceLister, rdb valkeycompat.Cmdable) *Scheduler {
	return &Scheduler{sourceStore: sourceStore, rdb: rdb}
}

// SelectDue returns up to batchSize enabled sources whose crawl interval has
// elapsed (or that have never been crawled), oldest-first. It is a thin pass-
// through to the source store's ListDueIntelSources query.
func (s *Scheduler) SelectDue(ctx context.Context, batchSize int32) ([]sources.Source, error) {
	rows, err := s.sourceStore.ListDueIntelSources(ctx, batchSize)
	if err != nil {
		return nil, fmt.Errorf("scheduler: select due: %w", err)
	}
	return rows, nil
}

// RecordCrawlOutcome updates the volatility auto-tuner state for a source
// after a crawl completes. If changed is true the content hash differed from
// the previous crawl; otherwise the source was crawled but unchanged.
//
// Tuning rules:
//   - unchanged: increment the consecutive-unchanged counter. When it reaches
//     UnchangedThreshold, double the effective interval (capped at
//     MaxEffectiveIntervalMins) and reset the counter.
//   - changed: halve the effective interval, floored at the source's base
//     crawl_frequency_mins, and reset the counter.
//
// baseIntervalMins is the source's crawl_frequency_mins and acts as the
// floor for the effective interval.
func (s *Scheduler) RecordCrawlOutcome(ctx context.Context, sourceID string, baseIntervalMins int32, changed bool) error {
	if sourceID == "" {
		return fmt.Errorf("scheduler: record crawl outcome: empty source id")
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
	return int32(mins), nil // #nosec G109 -- bounded by math.MaxInt32 check above
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
	return int32(mins), nil // #nosec G109 -- bounded by math.MaxInt32 check above
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
	return fmt.Sprintf("jc:intel:effective_interval:%s", sourceID)
}

func unchangedCountKey(sourceID string) string {
	return fmt.Sprintf("jc:intel:effective_interval:%s:unchanged_count", sourceID)
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
