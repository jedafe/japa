import { env } from '$env/dynamic/private';
import type { RequestEvent } from '@sveltejs/kit';
import type { ApiErr, ApiOk, ListMeta } from '$lib/types';

/** Server-only base URL for the Go backend (no /api/v1 suffix). */
export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
export const API_V1 = `${API_BASE}/api/v1`;

/**
 * Per-request timeout (ms) for backend calls originating from the SSR server.
 * Without an explicit timeout a slow/stuck backend hangs the page indefinitely
 * and the swallowing catch blocks in hooks.server.ts hide the failure. 5s is
 * long enough to absorb a cold DB connection without making a degraded backend
 * feel like a hang. Tuned for the SSR critical path, not background work.
 */
const REQUEST_TIMEOUT_MS = 5000;

/** Typed transport error; carries the backend's status + error code. */
export class ApiError extends Error {
	constructor(
		public readonly status: number,
		public readonly code: string,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

export interface ApiContext {
	fetch: typeof fetch;
	token: string | null;
	/** Hostname with the port stripped — resolves the tenant on the backend. */
	host: string;
	/**
	 * The real client IP address (from event.getClientAddress()). Forwarded
	 * as X-Forwarded-For so the backend's GeoIP middleware can resolve the
	 * visitor's country for geo-aware personalization (ADR-064).
	 */
	clientIP: string | null;
	/**
	 * Double-submit CSRF token (Ref: §5.2). Attached as X-CSRF-Token on every
	 * non-GET request so the backend's csrf.Middleware accepts the mutation.
	 * Null for guests (no mutations) or when the token could not be fetched.
	 */
	csrfToken: string | null;
	/**
	 * Raw `jc_refresh` cookie value forwarded to the backend so endpoints like
	 * GET /me/sessions can identify the current session (JEOS §50). Null when
	 * the cookie is absent (guests, or sessions logged out elsewhere).
	 */
	refreshCookie: string | null;
	/**
	 * Raw `jc_journey_token` cookie value forwarded to the backend so guest
	 * journey context is available on SSR calls (ADR-064a / 2026-08-21).
	 */
	journeyCookie: string | null;
}

/** Build an ApiContext from a SvelteKit request event. */
export function apiContext(event: RequestEvent): ApiContext {
	return {
		fetch: event.fetch,
		token: event.locals.token,
		host: event.locals.tenantHost,
		clientIP: event.getClientAddress() ?? null,
		csrfToken: event.locals.csrfToken ?? null,
		refreshCookie: event.cookies.get('jc_refresh') ?? null,
		journeyCookie: event.cookies.get('jc_journey_token') ?? null
	};
}

async function request<T>(
	ctx: ApiContext,
	method: string,
	path: string,
	body?: unknown
): Promise<ApiOk<T>> {
	const headers: Record<string, string> = {
		'content-type': 'application/json',
		// The backend resolves the tenant from the host. X-Forwarded-Host is the
		// portable channel; Host is set too for direct-fetch environments.
		host: ctx.host,
		'x-forwarded-host': ctx.host
	};
	// Forward the real client IP so the backend's GeoIP middleware can
	// resolve the visitor's country for geo-aware personalization (ADR-064).
	if (ctx.clientIP) headers['x-forwarded-for'] = ctx.clientIP;
	if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;
	// Attach the double-submit CSRF token on every mutating request so the
	// backend's csrf.Middleware (Ref: §5.2) accepts it. GETs are exempt.
	if (method !== 'GET' && ctx.csrfToken) headers['x-csrf-token'] = ctx.csrfToken;
	// Forward the jc_refresh cookie so the backend can identify the current
	// session (e.g. GET /me/sessions flags the current session per JEOS §50),
	// and the jc_journey_token cookie so guest journey context is available
	// on SSR (ADR-064a).
	const cookies: string[] = [];
	if (ctx.refreshCookie) cookies.push(`jc_refresh=${ctx.refreshCookie}`);
	if (ctx.journeyCookie) cookies.push(`jc_journey_token=${ctx.journeyCookie}`);
	if (cookies.length) headers.cookie = cookies.join('; ');

	// Abort the fetch after REQUEST_TIMEOUT_MS so a slow/stuck backend cannot
	// hang the SSR critical path. The controller + timer are cleaned up via
	// the finally block below to avoid leaking a pending timer per request.
	const controller = new AbortController();
	const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
	let res: Response;
	try {
		res = await ctx.fetch(`${API_V1}${path}`, {
			method,
			headers,
			body: body === undefined ? undefined : JSON.stringify(body),
			signal: controller.signal
		});
	} finally {
		clearTimeout(timer);
	}

	let payload: unknown = null;
	try {
		payload = await res.json();
	} catch {
		payload = null;
	}

	const isErrorEnvelope =
		payload !== null && typeof payload === 'object' && 'error' in payload;

	if (!res.ok || isErrorEnvelope) {
		const err = (payload as ApiErr | null)?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}

	return (payload ?? { data: null }) as ApiOk<T>;
}

/** GET a single resource, unwrapping the `{ data }` envelope. */
export async function apiGet<T>(ctx: ApiContext, path: string): Promise<T> {
	const { data } = await request<T>(ctx, 'GET', path);
	return data;
}

/**
 * GET a single resource, returning the full response envelope (including
 * optional `json_ld` and `can_solve` fields). Use this when an endpoint
 * emits sidecar data beyond the standard `{ data }` wrapper.
 */
export async function apiGetEnvelope<T>(ctx: ApiContext, path: string): Promise<ApiOk<T>> {
	return request<T>(ctx, 'GET', path);
}

/**
 * GET a list endpoint. The backend returns `{ data: T[] }` (bare array, no
 * wrapper object) and may include `meta`; a null/absent data becomes `[]`.
 */
export async function apiList<T>(
	ctx: ApiContext,
	path: string
): Promise<{ items: T[]; meta?: ListMeta }> {
	const { data, meta } = await request<T[] | null>(ctx, 'GET', path);
	return { items: data ?? [], meta };
}

/**
 * GET a radar-style list endpoint. The radar feed endpoints
 * (GET /intel/radar, GET /intel/radar/followed) use a non-standard
 * envelope per spec §739: {items, next_cursor, has_more, counts_by_type,
 * counts_by_country, followed_space_ids} — the array is under `items`,
 * not `data`. This helper reads `items` from the raw payload and passes
 * through the sidecar fields as `meta` so callers can access counts and
 * pagination state.
 */
export async function apiRadarList<T>(
	ctx: ApiContext,
	path: string
): Promise<{ items: T[]; meta?: Record<string, unknown> }> {
	// request() casts the raw JSON as ApiOk<T> = {data: T}, but the radar
	// endpoint's raw payload IS the envelope (no `data` wrapper). Access
	// the fields directly from the returned object.
	const payload = await request<Record<string, unknown>>(ctx, 'GET', path);
	const raw = payload as unknown as Record<string, unknown>;
	const items = (raw['items'] as T[] | undefined) ?? [];
	const meta: Record<string, unknown> = {};
	for (const key of [
		'next_cursor',
		'has_more',
		'counts_by_type',
		'counts_by_country',
		'followed_space_ids'
	]) {
		if (key in raw) meta[key] = raw[key];
	}
	return { items, meta };
}

export async function apiPost<T>(ctx: ApiContext, path: string, body?: unknown): Promise<T> {
	const { data } = await request<T>(ctx, 'POST', path, body);
	return data;
}

export async function apiPatch<T>(ctx: ApiContext, path: string, body?: unknown): Promise<T> {
	const { data } = await request<T>(ctx, 'PATCH', path, body);
	return data;
}

export async function apiPut<T>(ctx: ApiContext, path: string, body?: unknown): Promise<T> {
	const { data } = await request<T>(ctx, 'PUT', path, body);
	return data;
}

export async function apiDelete<T>(ctx: ApiContext, path: string): Promise<T> {
	const { data } = await request<T>(ctx, 'DELETE', path);
	return data;
}
