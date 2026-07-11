import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/api/client';
import type { ApiKey } from '$lib/api/types';

export const load: PageServerLoad = async ({ locals }) => {
	const user = locals.user as { id: string } | null;
	if (!user) {
		throw redirect(302, '/login');
	}

	let keys: ApiKey[] = [];
	const result = await apiFetch<ApiKey[]>('/developer/keys');
	if (result.data) {
		keys = result.data;
	}

	return { keys };
};
