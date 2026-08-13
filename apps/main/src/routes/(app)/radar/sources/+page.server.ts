import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';

/**
 * Public source directory at /radar/sources — lists the intelligence
 * crawl sources as a transparency surface (INTEL-024 fix). Public endpoint
 * GET /api/v1/intel/sources returns name, domain, trust_level, category,
 * last_verified_at. No auth required.
 *
 * Ref: japaconnect-engineering — Japa Intelligence System spec §7
 *      ADR-112 (public source directory)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let sources: Array<{
		name: string;
		domain: string;
		trust_level: number;
		category: string;
		last_verified_at?: string | null;
	}> = [];
	let intelUnavailable = false;

	try {
		const res = await apiList<{
			name: string;
			domain: string;
			trust_level: number;
			category: string;
			last_verified_at?: string | null;
		}>(ctx, '/intel/sources?page_size=100');
		sources = res.items;
	} catch (err) {
		if (err instanceof ApiError && err.status === 503) {
			intelUnavailable = true;
		}
	}

	return { sources, intelUnavailable };
};
