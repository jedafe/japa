/**
 * Simple in-memory TTL cache for server-side data fetching.
 * Avoids re-fetching stable data (feature flags, translations) on every SSR request.
 */

interface CacheEntry<T> {
	data: T;
	expires: number;
}

const cache = new Map<string, CacheEntry<unknown>>();

/**
 * Get or fetch cached data. Returns cached data if valid, otherwise fetches and caches.
 */
export async function getCached<T>(
	key: string,
	ttlMs: number,
	fetcher: () => Promise<T>
): Promise<T> {
	const entry = cache.get(key);
	if (entry && entry.expires > Date.now()) {
		return entry.data as T;
	}

	const data = await fetcher();
	cache.set(key, { data, expires: Date.now() + ttlMs });
	return data;
}

/**
 * Invalidate a specific cache key.
 */
export function invalidateCache(key: string): void {
	cache.delete(key);
}

/**
 * Invalidate all cache entries matching a prefix.
 */
export function invalidateCacheByPrefix(prefix: string): void {
	for (const key of cache.keys()) {
		if (key.startsWith(prefix)) {
			cache.delete(key);
		}
	}
}
