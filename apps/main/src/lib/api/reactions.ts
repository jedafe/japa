import { api } from '$lib/api/client';

export interface ReactionType {
	key: string;
	emoji: string;
	label: string;
	enabled: boolean;
	position: number;
}

/**
 * Reaction-type catalog — global and admin-configured (Section 14). Every
 * reaction widget on a page needs the same list, so the result is cached and
 * concurrent callers share one in-flight request instead of firing one HTTP
 * call per component instance.
 */
const CACHE_TTL_MS = 60_000;
let cache: { at: number; types: ReactionType[] } | null = null;
let inFlight: Promise<ReactionType[]> | null = null;

export function getReactionTypes(): Promise<ReactionType[]> {
	if (cache && Date.now() - cache.at < CACHE_TTL_MS) {
		return Promise.resolve(cache.types);
	}
	if (inFlight) return inFlight;
	inFlight = (async () => {
		try {
			const res = await api.get<ReactionType[]>('/reaction-types');
			const types = (res.data ?? [])
				.filter((r) => r.enabled)
				.sort((a, b) => a.position - b.position);
			cache = { at: Date.now(), types };
			return types;
		} finally {
			inFlight = null;
		}
	})();
	return inFlight;
}
