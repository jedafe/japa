/**
 * Badge derivation for comparison cards (spec §4, v1 client-side rules):
 * position ≤2 in a popularity sort → popular; view_count over threshold →
 * hot; created within 14 days → trending. Priority: popular > hot > trending.
 */
import type { JCEComparisonSummary } from '#lib/types/jce.js';

export type CompareBadgeKind = 'popular' | 'hot' | 'trending' | '';

export const HOT_MIN_VIEWS = 5000;
const TRENDING_WINDOW_MS = 14 * 24 * 60 * 60 * 1000;

export function badgeFor(
	item: JCEComparisonSummary,
	opts: { popularityIndex?: number; hotMinViews?: number; now?: number } = {}
): CompareBadgeKind {
	if ((opts.popularityIndex ?? Infinity) <= 2) return 'popular';
	if ((item.view_count ?? 0) >= (opts.hotMinViews ?? HOT_MIN_VIEWS)) return 'hot';
	const created = Date.parse(item.created_at ?? '');
	const now = opts.now ?? Date.now();
	if (!Number.isNaN(created) && now - created < TRENDING_WINDOW_MS) return 'trending';
	return '';
}
