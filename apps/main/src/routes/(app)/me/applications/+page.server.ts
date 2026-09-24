import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';
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
