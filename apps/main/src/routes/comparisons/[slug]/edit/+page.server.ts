import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { error } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const result = await apiFetch<{ id: string; name: string; slug: string; description: string; is_public: boolean }>(
		`/comparisons/types/${params.slug}`,
		{ headers }
	);

	if (!result.ok || !result.data) {
		throw error(404, { code: 'not_found', message: 'Comparison type not found' });
	}

	return { type: result.data };
};
