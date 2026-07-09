import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ params }) => {
	try {
		const [lessonResult, trackResult] = await Promise.all([
			apiFetch<unknown>(`/learn/tracks/${params.trackSlug}/modules/${params.moduleSlug}/lessons/${params.lessonSlug}`),
			apiFetch<{ id: string; slug: string; title: string; modules: { slug: string; title: string; lessons: { id: string; slug: string; title: string; duration_minutes: number }[] }[] }>(`/learn/tracks/${params.trackSlug}`)
		]);

		if (!lessonResult.ok || !lessonResult.data) {
			throw error(404, { code: 'NOT_FOUND', message: 'Lesson not found' });
		}
		if (!trackResult.ok || !trackResult.data) {
			throw error(404, { code: 'NOT_FOUND', message: 'Track not found' });
		}

		const track = trackResult.data;
		const lesson = lessonResult.data;
		const currentModule = track.modules.find((m) => m.slug === params.moduleSlug);
		if (!currentModule) throw error(404, { code: 'NOT_FOUND', message: 'Module not found' });

		const allLessons: { moduleSlug: string; lessonSlug: string; lessonId: string; title: string }[] = [];
		for (const mod of track.modules) {
			for (const l of mod.lessons) {
				allLessons.push({ moduleSlug: mod.slug, lessonSlug: l.slug, lessonId: l.id, title: l.title });
			}
		}

		const currentIndex = allLessons.findIndex(
			(l) => l.moduleSlug === params.moduleSlug && l.lessonSlug === params.lessonSlug
		);
		const prevLesson = currentIndex > 0 ? allLessons[currentIndex - 1] : null;
		const nextLesson = currentIndex < allLessons.length - 1 ? allLessons[currentIndex + 1] : null;

		return { lesson, track, currentModule, allLessons, prevLesson, nextLesson, currentIndex };
	} catch (e) {
		if (e && typeof e === 'object' && 'status' in e) throw e;
		throw error(500, { code: 'LOAD_ERROR', message: 'Failed to load lesson' });
	}
};
