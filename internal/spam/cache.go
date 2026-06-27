package spam

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/valkey"
)

// Cache key prefix for censored words. All JapaConnect Valkey keys use the
// "jc:" namespace as documented in internal/valkey/client.go.
const cacheKeyPrefix = "jc:spam:censored:"

// cacheTTL is the TTL for cached censored words. One hour balances freshness
// with database load — admin changes propagate within the hour without
// requiring manual cache invalidation.
const cacheTTL = 1 * time.Hour

// cachedWords is the JSON-serializable shape stored in Valkey.
type cachedWords struct {
	Words []CensoredWord `json:"words"`
}

// CacheCensoredWords stores censored words in Valkey with a 1-hour TTL.
// If Valkey is unreachable the error is logged but not returned — the
// caller should fall back to a direct database read.
func CacheCensoredWords(ctx context.Context, rdb *valkey.Client, tenantID pgtype.UUID, words []CensoredWord) error {
	if rdb == nil {
		return nil
	}

	key := cacheKeyFor(tenantID)
	data, err := json.Marshal(cachedWords{Words: words})
	if err != nil {
		return fmt.Errorf("marshaling cached words: %w", err)
	}

	if err := rdb.Redis().Set(ctx, key, data, cacheTTL).Err(); err != nil {
		slog.Warn("failed to cache censored words",
			"tenant_id", tenantID,
			"error", err,
		)
		return fmt.Errorf("caching censored words: %w", err)
	}

	return nil
}

// GetCachedCensoredWords retrieves censored words from Valkey cache. On a
// cache miss it reads from the database, populates the cache, and returns
// the result. If both cache and database fail, returns an empty slice and
// an error so the caller can degrade gracefully.
func GetCachedCensoredWords(
	ctx context.Context,
	rdb *valkey.Client,
	queries *db.DB,
	tenantID pgtype.UUID,
) ([]CensoredWord, error) {
	if rdb != nil {
		words, err := fetchFromCache(ctx, rdb, tenantID)
		if err == nil && words != nil {
			return words, nil
		}
		// Cache miss or error — fall through to database
	}

	words, err := fetchFromDB(ctx, queries, tenantID)
	if err != nil {
		return nil, err
	}

	// Best-effort cache population (fire and forget on error)
	if rdb != nil && len(words) > 0 {
		if cacheErr := CacheCensoredWords(ctx, rdb, tenantID, words); cacheErr != nil {
			slog.Warn("failed to populate censored words cache",
				"tenant_id", tenantID,
				"error", cacheErr,
			)
		}
	}

	return words, nil
}

// InvalidateCensoredWordsCache removes the cached censored words for the
// tenant. Called after admin create/delete operations.
func InvalidateCensoredWordsCache(ctx context.Context, rdb *valkey.Client, tenantID pgtype.UUID) error {
	if rdb == nil {
		return nil
	}

	key := cacheKeyFor(tenantID)
	if err := rdb.Redis().Del(ctx, key).Err(); err != nil {
		slog.Warn("failed to invalidate censored words cache",
			"tenant_id", tenantID,
			"error", err,
		)
		return fmt.Errorf("invalidating cache: %w", err)
	}

	return nil
}

// fetchFromCache attempts to read and deserialize cached censored words.
func fetchFromCache(ctx context.Context, rdb *valkey.Client, tenantID pgtype.UUID) ([]CensoredWord, error) {
	key := cacheKeyFor(tenantID)
	data, err := rdb.Redis().Get(ctx, key).Bytes()
	if err != nil {
		return nil, fmt.Errorf("reading cache: %w", err)
	}

	var cached cachedWords
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("unmarshaling cached words: %w", err)
	}

	return cached.Words, nil
}

// fetchFromDB reads all censored words for the tenant from the database
// and converts them to the CensoredWord type.
func fetchFromDB(ctx context.Context, database *db.DB, tenantID pgtype.UUID) ([]CensoredWord, error) {
	store := NewStoreWithReader(database)
	rows, err := store.GetCensoredWordsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reading censored words from db: %w", err)
	}

	words := make([]CensoredWord, 0, len(rows))
	for _, row := range rows {
		w := CensoredWord{
			ID:      row.ID.String(),
			Pattern: row.Pattern,
			Action:  row.Action,
		}
		if row.Replacement != nil {
			w.Replacement = *row.Replacement
		}
		words = append(words, w)
	}

	return words, nil
}

// cacheKeyFor builds the Valkey key for a tenant's censored words cache.
func cacheKeyFor(tenantID pgtype.UUID) string {
	return fmt.Sprintf("%s%s", cacheKeyPrefix, tenantID.String())
}
