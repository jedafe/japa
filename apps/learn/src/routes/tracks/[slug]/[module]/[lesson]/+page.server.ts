import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { error, redirect } from '@sveltejs/kit';
import type { Lesson, TrackDetail } from '$lib/types';

/**
 * Lesson Player — SSR fetches the lesson content (ADR-060 reading exception).
 *
 * Access control: the backend returns 403 if the user is not enrolled and the
 * lesson is not a preview. We redirect to the track's enroll page in that case.
 *
 * SSR/CSR boundary: the lesson CONTENT (article HTML, video URL, quiz) is
 * server-rendered. The completed-lessons set and progress are CSR-only.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const { slug, module: moduleSlug, lesson: lessonSlug } = event.params;

	// Fetch the track detail (for the curriculum rail + progress context).
	let track: TrackDetail;
	try {
		track = await apiGet<TrackDetail>(ctx, `/learn/university/tracks/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'track_not_found', message: 'Track not found' });
		}
		throw err;
	}

	// Fetch the lesson by slug (public content, but 403 if not enrolled and not preview).
	let lesson: Lesson;
	try {
		lesson = await apiGet<Lesson>(
			ctx,
			`/learn/university/tracks/${slug}/modules/${moduleSlug}/lessons/${lessonSlug}`
		);
	} catch (err) {
		if (err instanceof ApiError) {
			if (err.status === 404) {
				throw error(404, { code: 'lesson_not_found', message: 'Lesson not found' });
			}
			if (err.status === 403) {
				// Not enrolled and not a preview — redirect to the track enroll page.
				throw redirect(302, `/tracks/${slug}`);
			}
		}
		throw err;
	}

	return { track, lesson };
};
