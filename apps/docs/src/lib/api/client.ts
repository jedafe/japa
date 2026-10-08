// Ref: Section 59 (Sidebar Management System) — sidebar widgets fetch their
// data client-side via onMount. This lightweight client wraps fetch against
// the public API origin, handling the { data } envelope and error shapes.
// Server-side data loading uses $lib/server/api.ts instead.

import { env } from '$env/dynamic/public';
import { buildApiUrl } from '@japaconnect/api-url';

const API_BASE = (env.PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');

/**
 * Surface identifier sent on every backend call (X-JC-Surface). The backend
 * resolves feature flags and geo rules per surface — the valid values are
 * defined in internal/features/eval.go (ValidSurface).
 */
const JC_SURFACE = 'docs';

function apiUrl(path: string): string {
	return path.startsWith('http') ? path : buildApiUrl(path, API_BASE);
}

export interface ApiListResponse<T> {
	data: T[];
	total?: number;
	page?: number;
	page_size?: number;
}

export async function apiGet<T>(path: string): Promise<T | null> {
	try {
		const url = apiUrl(path);
		const res = await fetch(url, {
			credentials: 'include',
			headers: { Accept: 'application/json', 'X-JC-Surface': JC_SURFACE }
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

export function getCookie(name: string): string | undefined {
	if (typeof document === 'undefined') return undefined;
	const match = document.cookie.match(new RegExp('(?:^|; )' + encodeURIComponent(name).replace(/[-/\\^$*+?.()|[\]{}]/g, '\\$&') + '=([^;]+)'));
	return match ? decodeURIComponent(match[1]) : undefined;
}

export async function apiPost<T = unknown>(path: string, body?: unknown): Promise<{ ok: true; data: T } | { ok: false; error: string }> {
	const token = getCookie('jc_token');
	const csrf = getCookie('csrf_token');
	const url = apiUrl(path);
	const headers: Record<string, string> = {
		Accept: 'application/json',
		'Content-Type': 'application/json',
		'X-JC-Surface': JC_SURFACE
	};
	if (token) headers['Authorization'] = `Bearer ${token}`;
	if (csrf) headers['X-CSRF-Token'] = csrf;
	try {
		const res = await fetch(url, {
			method: 'POST',
			credentials: 'include',
			headers,
			body: JSON.stringify(body ?? {})
		});
		if (res.ok) {
			const json = await res.json().catch(() => ({}));
			return { ok: true, data: (json?.data ?? json) as T };
		}
		const text = await res.text().catch(() => res.statusText);
		let msg = text || res.statusText;
		try {
			const json = JSON.parse(text);
			if (json?.error?.message) msg = json.error.message;
		} catch {}
		return { ok: false, error: msg };
	} catch (err) {
		return { ok: false, error: err instanceof Error ? err.message : 'Network error' };
	}
}
