import { writable, derived, get } from 'svelte/store';
import type { Enrollment } from '$lib/api/types';
import { api } from '$lib/api';

const COMPLETED_LESSONS_KEY = 'jc_completed_lessons';
const PROGRESS_KEY = 'jc_progress';

function loadCompletedLessons(): Record<string, string[]> {
	if (typeof window === 'undefined') return {};
	try {
		const stored = localStorage.getItem(COMPLETED_LESSONS_KEY);
		return stored ? JSON.parse(stored) : {};
	} catch {
		return {};
	}
}

function saveCompletedLessons(data: Record<string, string[]>): void {
	if (typeof window === 'undefined') return;
	try {
		localStorage.setItem(COMPLETED_LESSONS_KEY, JSON.stringify(data));
	} catch {
		// Silently fail if localStorage is unavailable
	}
}

function loadProgress(): Record<string, number> {
	if (typeof window === 'undefined') return {};
	try {
		const stored = localStorage.getItem(PROGRESS_KEY);
		return stored ? JSON.parse(stored) : {};
	} catch {
		return {};
	}
}

function saveProgress(data: Record<string, number>): void {
	if (typeof window === 'undefined') return;
	try {
		localStorage.setItem(PROGRESS_KEY, JSON.stringify(data));
	} catch {
		// Silently fail if localStorage is unavailable
	}
}

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

export function getCompletedLessons(trackId: string): string[] {
	const stored = loadCompletedLessons();
	return stored[trackId] ?? [];
}

export async function markLessonComplete(
	lessonId: string,
	trackId: string,
	totalLessons: number
): Promise<boolean> {
	const completedLessons = loadCompletedLessons();
	const trackCompleted = completedLessons[trackId] ?? [];

	// Deduplication: don't increment if lesson already completed
	if (trackCompleted.includes(lessonId)) {
		return false;
	}

	try {
		await api.completeLesson(lessonId);
		// Server call succeeded — continue with client-side tracking
	} catch {
		// Server call failed — continue with client-side tracking
	}

	// Update completed lessons list
	trackCompleted.push(lessonId);
	completedLessons[trackId] = trackCompleted;
	saveCompletedLessons(completedLessons);

	// Update progress percentage
	const progressPct = totalLessons > 0 ? Math.round((trackCompleted.length / totalLessons) * 100) : 0;
	const progressData = loadProgress();
	progressData[trackId] = progressPct;
	saveProgress(progressData);

	// Update local store with client-side calculation
	enrollments.update(($enrollments) => {
		return $enrollments.map((e) => {
			if (e.track_id === trackId) {
				return { ...e, progress_pct: progressPct };
			}
			return e;
		});
	});

	return true;
}
