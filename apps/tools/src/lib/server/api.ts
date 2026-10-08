import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import type { RequestEvent } from '@sveltejs/kit';
import type { ApiErr, ApiOk, ListMeta } from '$lib/types';

/** Server-only base URL for the Go backend (no /api/v1 suffix). */
export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
export const API_V1 = `${API_BASE}/api/v1`;

/**
 * Surface identifier sent on every backend call (X-JC-Surface). The backend
 * resolves feature flags and geo rules per surface — the valid values are
 * defined in internal/features/eval.go (ValidSurface).
 */
export const JC_SURFACE = 'tools';

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
	/** Hostname with the port stripped — forwarded to the backend for host-based routing. */
	host: string;
	/**
	 * The real client IP address (from event.getClientAddress()). Forwarded
	 * as X-Forwarded-For so the backend's GeoIP middleware can resolve the
	 * visitor's country for geo-aware feature rules (ADR-064).
	 */
	clientIP: string | null;
	/**
	 * Double-submit CSRF token (Ref: §5.2). Attached as X-CSRF-Token on every
	 * non-GET request so the backend's csrf.Middleware accepts the mutation.
	 * Null for guests (no mutations) or when the token could not be fetched.
	 */
	csrfToken: string | null;
}

/** Build an ApiContext from a SvelteKit request event. */
export function apiContext(event: RequestEvent): ApiContext {
	return {
		fetch: event.fetch,
		token: event.locals.token,
		host: event.locals.requestHost,
		clientIP: event.getClientAddress() ?? null,
		csrfToken: event.locals.csrfToken ?? null
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
		// Forward the host header to the backend for reverse-proxy correctness. X-Forwarded-Host is the
		// portable channel (the Fetch API silently ignores any `host` header set
		// here, so we only send x-forwarded-host).
		'x-forwarded-host': ctx.host,
		// Identify this surface for scoped feature-flag resolution.
		'x-jc-surface': JC_SURFACE
	};
	// Forward the real client IP so the backend's GeoIP middleware resolves
	// the visitor's country, not the app server's egress IP (ADR-064).
	if (ctx.clientIP) headers['x-forwarded-for'] = ctx.clientIP;
	if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;
	// Attach the double-submit CSRF token on every mutating request so the
	// backend's csrf.Middleware (Ref: §5.2) accepts it. GETs are exempt.
	if (method !== 'GET' && ctx.csrfToken) headers['x-csrf-token'] = ctx.csrfToken;

	const res = await ctx.fetch(buildApiUrl(path, API_BASE), {
		method,
		headers,
		body: body === undefined ? undefined : JSON.stringify(body)
	});

	let payload: unknown = null;
	try {
		payload = await res.json();
	} catch {
		payload = null;
	}

	const isErrorEnvelope = payload !== null && typeof payload === 'object' && 'error' in payload;

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
 * GET a list endpoint. The backend returns `{ data: T[] }` (bare array, no
 * wrapper object) and may include `meta`; a null/absent data becomes `[]`.
 * Some endpoints return pagination totals under a top-level `pagination` key
 * rather than under `meta` (e.g. GET /attestation). Map them so callers can
 * use `meta` uniformly.
 */
export async function apiList<T>(
	ctx: ApiContext,
	path: string
): Promise<{ items: T[]; meta?: ListMeta }> {
	const raw = (await request<T[] | null>(ctx, 'GET', path)) as ApiOk<T[] | null> & {
		pagination?: { total?: number; page?: number; limit?: number; page_size?: number };
	};
	const meta =
		raw.meta ??
		(raw.pagination?.total !== undefined
			? {
					total: raw.pagination.total,
					page: raw.pagination.page,
					limit: raw.pagination.limit ?? raw.pagination.page_size
				}
			: undefined);
	return { items: raw.data ?? [], meta };
}

export async function apiPost<T>(ctx: ApiContext, path: string, body?: unknown): Promise<T> {
	const { data } = await request<T>(ctx, 'POST', path, body);
	return data;
}

export async function apiPatch<T>(ctx: ApiContext, path: string, body?: unknown): Promise<T> {
	const { data } = await request<T>(ctx, 'PATCH', path, body);
	return data;
}

export async function apiDelete<T>(ctx: ApiContext, path: string): Promise<T> {
	const { data } = await request<T>(ctx, 'DELETE', path);
	return data;
}

/**
 * GET a single resource, returning null on 404 instead of throwing. Useful for
 * optional lookups in load() functions where a missing resource is a valid state.
 */
export async function apiGetOrNull<T>(ctx: ApiContext, path: string): Promise<T | null> {
	try {
		return await apiGet<T>(ctx, path);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) return null;
		throw err;
	}
}
