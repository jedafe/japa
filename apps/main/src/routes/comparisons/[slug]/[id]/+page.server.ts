import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const [compResult, matrixResult] = await Promise.all([
		apiFetch<{ id: string; title: string; slug: string; type_id: string; item_count: number; vote_count: number; created_at: string }>(
			`/comparisons/${params.id}`,
			{ headers }
		),
		apiFetch<{ dimensions: Array<{ id: string; name: string }>; items: Array<{ id: string; name: string; values: Record<string, string> }> }>(
			`/comparisons/${params.id}/matrix`,
			{ headers }
		)
	]);

	return {
		comparison: compResult.ok ? compResult.data : null,
		matrix: matrixResult.ok ? matrixResult.data : null
	};
};
