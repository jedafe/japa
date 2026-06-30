import type { PageLoad } from './$types';
import { error } from '@sveltejs/kit';
import { api } from '$lib/api';

export const load: PageLoad = async ({ params }) => {
	try {
		const [lesson, track] = await Promise.all([
			api.getLesson(params.trackSlug, params.moduleSlug, params.lessonSlug),
			api.getTrack(params.trackSlug)
		]);

		// Find current module
		const currentModule = track.modules.find((m) => m.slug === params.moduleSlug);
		if (!currentModule) throw new Error('Module not found');

		// Build flat lesson list for navigation
		const allLessons: { moduleSlug: string; lessonSlug: string; lessonId: string; title: string }[] = [];
		for (const mod of track.modules) {
			for (const l of mod.lessons) {
				allLessons.push({
					moduleSlug: mod.slug,
					lessonSlug: l.slug,
					lessonId: l.id,
					title: l.title
				});
			}
		}

		const currentIndex = allLessons.findIndex(
			(l) => l.moduleSlug === params.moduleSlug && l.lessonSlug === params.lessonSlug
		);
		const prevLesson = currentIndex > 0 ? allLessons[currentIndex - 1] : null;
		const nextLesson = currentIndex < allLessons.length - 1 ? allLessons[currentIndex + 1] : null;

		return {
			lesson,
			track,
			currentModule,
			allLessons,
			prevLesson,
			nextLesson,
			currentIndex
		};
	} catch (e) {
		if (e instanceof Error && (e.message.includes('404') || e.message.includes('not found'))) {
			throw error(404, { code: 'NOT_FOUND', message: 'Lesson not found' });
		}
		throw error(500, { code: 'LOAD_ERROR', message: 'Failed to load lesson' });
	}
};
