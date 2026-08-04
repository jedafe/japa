import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ComparisonListItem } from '$lib/types';

/**
 * Compare list at /compare — published comparisons (Section 30, Phase 16.01).
 * SSR-fetches GET /comparisons so the list renders immediately. The endpoint
 * is auth-gated on the backend, so guests degrade to an empty list with a
 * login CTA (no hardcoded data — Rule 16).
 *
 * Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let comparisons: ComparisonListItem[] = [];
	try {
		comparisons = (await apiList<ComparisonListItem>(ctx, `/comparisons?page_size=${PAGE_SIZE}`))
			.items;
	} catch {
		// 401 for guests or backend unavailable — render empty state.
		comparisons = [];
	}

	return { comparisons };
};
