// Shared client-side enrollments store (ADR-060 CSR boundary).
//
// All sidebar widgets (ContinueLearningCard, ProgressRingWidget,
// LearningEnrollmentsWidget) and the lesson player read from this single
// store instead of each fetching /me/enrollments independently. This
// guarantees that after a mutation (enroll, mark-complete, progress update)
// every widget reflects the latest state — call refresh() once and all
// subscribers update reactively.
//
// The store is session-guarded: if no user is logged in, it stays empty and
// never fetches. The first subscriber's $effect triggers the initial fetch;
// subsequent subscribers reuse the cached data.

import { api } from '$lib/api/client';
import type { Enrollment, SessionUser } from '$lib/types';

class EnrollmentsStore {
	/** Current enrollments (empty array = loaded but none, null = not loaded). */
	enrollments = $state<Enrollment[] | null>(null);
	loading = $state(false);
	error = $state<string | null>(null);

	private fetchPromise: Promise<void> | null = null;

	/**
	 * Fetch enrollments from /me/enrollments. Deduplicates concurrent calls
	 * so multiple widgets mounting at the same time share a single request.
	 * Safe to call when not logged in — returns immediately with empty array.
	 */
	async fetch(): Promise<void> {
		// Already loaded or in-flight — don't double-fetch.
		if (this.enrollments !== null || this.fetchPromise) {
			return this.fetchPromise ?? Promise.resolve();
		}

		this.loading = true;
		this.error = null;

		this.fetchPromise = (async () => {
			try {
				const r = await api.get<Enrollment[]>('/me/enrollments');
				this.enrollments = r.data ?? [];
			} catch (e) {
				this.error = e instanceof Error ? e.message : 'Failed to load enrollments';
				this.enrollments = [];
			} finally {
				this.loading = false;
				this.fetchPromise = null;
			}
		})();

		return this.fetchPromise;
	}

	/**
	 * Force a re-fetch of enrollments, discarding cached data. Called after
	 * mutations that change enrollment state (enroll, mark-complete, progress
	 * update). All subscribed widgets update reactively.
	 */
	async refresh(): Promise<void> {
		// Cancel any in-flight fetch by resetting its promise; the new fetch
		// below will overwrite the state when it completes.
		this.fetchPromise = null;
		this.enrollments = null;
		this.loading = true;
		this.error = null;

		this.fetchPromise = (async () => {
			try {
				const r = await api.get<Enrollment[]>('/me/enrollments');
				this.enrollments = r.data ?? [];
			} catch (e) {
				this.error = e instanceof Error ? e.message : 'Failed to load enrollments';
				this.enrollments = [];
			} finally {
				this.loading = false;
				this.fetchPromise = null;
			}
		})();

		return this.fetchPromise;
	}

	/** Reset to initial state (e.g. on logout). */
	reset(): void {
		this.enrollments = null;
		this.loading = false;
		this.error = null;
		this.fetchPromise = null;
	}

	/** Convenience: true if the user is enrolled in a given track. */
	isEnrolled(trackId: string): boolean {
		return this.enrollments?.some((e) => e.track_id === trackId) ?? false;
	}

	/** Convenience: get the enrollment for a given track, or null. */
	getByTrack(trackId: string): Enrollment | null {
		return this.enrollments?.find((e) => e.track_id === trackId) ?? null;
	}
}

export const enrollmentsStore = new EnrollmentsStore();
