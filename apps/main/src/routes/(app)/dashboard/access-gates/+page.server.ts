// Ref: japaconnect-engineering/sections/123-ops-access-gates.md
// Super-admin-only reachability toggles for privileged surfaces (admin panel,
// control panel, Stalwart admin). The page's existence is not disclosed to
// non-super-admins — they get the same 404 as any unknown dashboard route.
import { error, fail, redirect, type RequestEvent } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { apiContext, apiGet, apiPost, ApiError } from '#lib/server/api.js';

export interface AccessGate {
	key: string;
	label: string;
	kind: string;
	domain?: string;
	/** on | off | transitioning | unknown */
	state: string;
	/** Raw Coolify resource status, e.g. "running:unknown". */
	status: string;
}

function requireSuperAdmin(event: RequestEvent) {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const isSuperAdmin = (event.locals.user.groups ?? []).some(
		(g) => g === 'super_admin' || g === 'Super Admin'
	);
	if (!isSuperAdmin) {
		throw error(404, { code: 'not_found', message: 'Unknown dashboard view' });
	}
}

export const load: PageServerLoad = async (event) => {
	requireSuperAdmin(event);

	try {
		const { gates } = await apiGet<{ gates: AccessGate[] }>(apiContext(event), '/admin/access-gates');
		return { gates, featureEnabled: true };
	} catch (err) {
		if (err instanceof ApiError) {
			// feature_disabled surfaces as an in-page banner, not a route error.
			if (err.code === 'feature_disabled') return { gates: [] as AccessGate[], featureEnabled: false };
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(500, { code: 'api_error', message: 'Failed to load access gates' });
	}
};

export const actions: Actions = {
	toggle: async (event) => {
		requireSuperAdmin(event);
		const form = await event.request.formData();
		const key = String(form.get('key') ?? '');
		try {
			const gate = await apiPost<AccessGate>(apiContext(event), `/admin/access-gates/${encodeURIComponent(key)}/toggle`);
			return { ok: true, gate };
		} catch (err) {
			if (err instanceof ApiError) {
				return fail(err.status, { toggleKey: key, code: err.code, message: err.message });
			}
			return fail(502, { toggleKey: key, code: 'gate_backend_error', message: 'Toggle request failed' });
		}
	}
};
