import { SITE_ORIGIN } from './links';
import type { Digest, FeedItem } from './types';

// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
// (Browser-Extension Digest). Same-origin API — WXT_JC_SITE_URL (see
// lib/links.ts) points dev/staging builds at a different origin.
const API_BASE = `${SITE_ORIGIN}/api/v1`;

export class ApiError extends Error {
	constructor(
		public status: number,
		public code: string,
		message: string,
	) {
		super(message);
	}
}

async function request<T>(path: string, apiKey: string | null): Promise<T> {
	const headers: Record<string, string> = { Accept: 'application/json' };
	if (apiKey) headers['X-API-Key'] = apiKey;

	const res = await fetch(`${API_BASE}${path}`, { headers });
	if (!res.ok) {
		let code = 'request_failed';
		let message = `Request failed (${res.status})`;
		try {
			const body = await res.json();
			if (body?.error?.code) code = body.error.code;
			if (body?.error?.message) message = body.error.message;
		} catch {
			// non-JSON error body — keep the generic message
		}
		throw new ApiError(res.status, code, message);
	}

	const body = await res.json();
	return body.data as T;
}

// GET /public/me/digest — one aggregate call per tab open.
export function getDigest(apiKey: string): Promise<Digest> {
	return request<Digest>('/public/me/digest', apiKey);
}

// GET /public/feeds/{name} — anonymous callers may fetch "popular" only
// (OptionalScope); keyed callers may fetch any explore name.
export function getFeed(name: 'popular' | 'latest' | 'rising', apiKey: string | null): Promise<FeedItem[]> {
	return request<FeedItem[]>(`/public/feeds/${name}`, apiKey);
}
