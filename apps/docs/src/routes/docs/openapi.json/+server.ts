import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { API_BASE } from '$lib/server/api';

/**
 * Proxy the canonical OpenAPI 3.0.3 spec from the Go backend
 * (GET /docs/openapi.json on the backend root, §6.4 dedupe) so the
 * "Download OpenAPI Spec" link on /docs/api resolves to the real JSON
 * rather than the docs SSR shell. The spec is public (no auth needed).
 *
 * Ref: ADR-060 (Docs drift), §47 (API reference), §6.4 (single canonical spec).
 */
export const GET: RequestHandler = async ({ fetch }) => {
	try {
		const res = await fetch(`${API_BASE}/docs/openapi.json`);
		if (!res.ok) {
			return json(
				{ error: { code: 'spec_unavailable', message: 'OpenAPI spec could not be loaded' } },
				{ status: res.status }
			);
		}
		const spec = await res.json();
		return json(spec);
	} catch {
		return json(
			{ error: { code: 'spec_unavailable', message: 'OpenAPI spec could not be loaded' } },
			{ status: 502 }
		);
	}
};
