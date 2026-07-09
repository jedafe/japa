import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { apiFetch } from '$lib/server/api';

interface ProgressData {
	track_id: string;
	progress: number;
	completed_lessons: string[];
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('learn_token') ?? cookies.get('jc_token');

	try {
		const result = await apiFetch<{ id: string; slug: string; title: string; description: string; difficulty: string; estimated_hours: number; module_count: number; lesson_count: number; modules: unknown[] }>(`/learn/tracks/${params.slug}`, {}, token);
		if (!result.ok || !result.data) {
			throw error(404, { code: 'NOT_FOUND', message: 'Track not found' });
		}

		let progress: ProgressData | null = null;
		try {
			const progressResult = await apiFetch<ProgressData>(`/learn/progress/${result.data.id}`, {}, token);
			if (progressResult.ok && progressResult.data) {
				progress = progressResult.data;
			}
		} catch {
			progress = null;
		}

		return { track: result.data, progress };
	} catch (e) {
		if (e && typeof e === 'object' && 'status' in e) throw e;
		throw error(500, { code: 'LOAD_ERROR', message: 'Failed to load track' });
	}
};
