import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPatch } from '$lib/server/api';
import { redirect, fail } from '@sveltejs/kit';
import type { JobApplication } from '$lib/types';

/**
 * My Applications at /me/applications (Section 63, ADR-079).
 * Auth-gated list of the current user's job applications with status.
 *
 * Ref: japaconnect-engineering/sections/63-jobs-board.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/me/applications');
	}

	const ctx = apiContext(event);

	const page = event.url.searchParams.get('page') ?? '1';
	let applications: JobApplication[] = [];
	let meta: { total?: number; page?: number; limit?: number; total_pages?: number } | undefined;
	try {
		const res = await apiList<JobApplication>(
			ctx,
			`/me/jobs/applications?page=${encodeURIComponent(page)}&limit=20`
		);
		applications = res.items;
		meta = res.meta;
	} catch {
		applications = [];
	}

	return { applications, meta, currentPage: Number(page) || 1 };
};

/**
 * Withdraw an application — PATCH /me/jobs/applications/{id} (owner-only,
 * only transitions to withdrawn).
 */
export const actions: Actions = {
	withdraw: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to withdraw' });
		}
		const form = await event.request.formData();
		const id = (form.get('id') as string | null)?.trim() ?? '';
		if (!id) {
			return fail(400, { error: 'missing_id', message: 'Application id is required' });
		}
		const ctx = apiContext(event);
		try {
			await apiPatch(ctx, `/me/jobs/applications/${encodeURIComponent(id)}`, {});
			return { ok: true, id };
		} catch (e) {
			const err = e as { code?: string; message?: string };
			return fail(400, { error: err?.code ?? 'withdraw_failed', message: err?.message ?? 'Could not withdraw' });
		}
	}
};
