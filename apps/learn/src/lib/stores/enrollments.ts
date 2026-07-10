import { writable, derived, get } from 'svelte/store';
import type { Enrollment } from '$lib/api/types';

export const enrollments = writable<Enrollment[]>([]);

export const enrolledTrackIds = derived(enrollments, ($enrollments) =>
	new Set($enrollments.map((e) => e.track_id))
);

export function setEnrollments(e: Enrollment[]) {
	enrollments.set(e);
}

export function isEnrolled(trackId: string): boolean {
	return get(enrolledTrackIds).has(trackId);
}

export function getProgress(trackId: string): number {
	const $enrollments = get(enrollments);
	const enrollment = $enrollments.find((e) => e.track_id === trackId);
	return enrollment?.progress_pct ?? 0;
}

export function markLessonComplete(lessonId: string) {
	enrollments.update(($enrollments) => {
		return $enrollments.map((e) => {
			if (e.last_lesson_id === lessonId) {
				return { ...e, progress_pct: Math.min(100, e.progress_pct + 1) };
			}
			return e;
		});
	});
}
