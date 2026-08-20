// Ref: Section 59 (Sidebar Management System) — sidebar widgets fetch their
// data client-side via onMount. This lightweight client wraps fetch against
// the public API origin, handling the { data } envelope and error shapes.
// Server-side data loading uses $lib/server/api.ts instead.

import { env } from '$env/dynamic/public';

const API_V1 = (() => {
	const base = (env.PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/$/, '');
	// PUBLIC_JC_API_URL may or may not include /api/v1; normalise so the
	// caller passes paths like /tags/trending (without the /api/v1 prefix).
	return base.endsWith('/api/v1') ? base : `${base}/api/v1`;
})();

export interface ApiListResponse<T> {
	data: T[];
	total?: number;
	page?: number;
	page_size?: number;
}

export async function apiGet<T>(path: string): Promise<T | null> {
	try {
		const url = path.startsWith('http') ? path : `${API_V1}${path.startsWith('/') ? '' : '/'}${path}`;
		const res = await fetch(url, {
			headers: { Accept: 'application/json' }
		});
		if (!res.ok) return null;
		const json = await res.json();
		// Backend envelopes: { data: T } for single items, { data: T[] } for lists.
		return (json?.data ?? json) as T;
	} catch {
		return null;
	}
}

export async function apiGetList<T>(path: string): Promise<T[]> {
	const result = await apiGet<T[] | ApiListResponse<T>>(path);
	if (result === null) return [];
	if (Array.isArray(result)) return result;
	if (result && typeof result === 'object' && 'data' in result && Array.isArray((result as ApiListResponse<T>).data)) {
		return (result as ApiListResponse<T>).data;
	}
	return [];
}
