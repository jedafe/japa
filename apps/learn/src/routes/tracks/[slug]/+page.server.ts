import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { apiFetch } from '$lib/server/api';
import { PUBLIC_JC_API_URL } from '$env/static/public';

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

		let isEnrolled = false;
		if (token) {
			try {
				const enrollRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me/enrollments`, {
					headers: { Authorization: `Bearer ${token}` }
				});
				if (enrollRes.ok) {
					const { data } = await enrollRes.json();
					isEnrolled = (data ?? []).some((e: { track_id: string }) => e.track_id === result.data!.id);
				}
			} catch {
				isEnrolled = false;
			}
		}

		return { track: result.data, progress, isEnrolled };
	} catch (e) {
		if (e && typeof e === 'object' && 'status' in e) throw e;
		throw error(500, { code: 'LOAD_ERROR', message: 'Failed to load track' });
	}
};
