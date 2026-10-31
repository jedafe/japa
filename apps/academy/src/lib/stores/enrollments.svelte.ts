// Shared client-side academy_enrollments store (ADR-060 CSR boundary).
//
// All sidebar widgets (ContinueAcademyCard, ProgressRingWidget,
// AcademyEnrollmentsWidget) and the lesson player read from this single
// store instead of each fetching /me/enrollments independently. This
// guarantees that after a mutation (enroll, mark-complete, progress update)
// every widget reflects the latest state — call refresh() once and all
// subscribers update reactively.
//
// The store is session-guarded: if no user is logged in, it stays empty and
// never fetches. The first subscriber's $effect triggers the initial fetch;
// subsequent subscribers reuse the cached data.

import { api, clientHasSession } from '$lib/api/client';
import type { Enrollment } from '$lib/types';

class EnrollmentsStore {
	/** Current academy_enrollments (empty array = loaded but none, null = not loaded). */
	academy_enrollments = $state<Enrollment[] | null>(null);
	loading = $state(false);
	error = $state<string | null>(null);

	private fetchPromise: Promise<void> | null = null;
	/** Monotonically incremented request generation — used to discard stale
	 *  in-flight responses after a refresh() or concurrent mutation. */
	private requestGen = 0;

	/**
	 * Fetch academy_enrollments from /me/enrollments. Deduplicates concurrent calls
	 * so multiple widgets mounting at the same time share a single request.
	 * Safe to call when not logged in — returns immediately with empty array.
	 */
	async fetch(authed?: boolean): Promise<void> {
		// Guard: no session cookie means this is a guest — don't call the
		// authenticated endpoint and risk 401 noise in the console. Callers
		// that already know the session state (e.g. page `data.user`) pass it
		// explicitly — the csrf_token cookie is a fragile proxy (F7).
		const has = authed ?? clientHasSession();
		if (!has) {
			this.academy_enrollments = [];
			this.loading = false;
			return;
		}

		// Already loaded or in-flight — don't double-fetch.
		if (this.academy_enrollments !== null || this.fetchPromise) {
			return this.fetchPromise ?? Promise.resolve();
		}

		this.loading = true;
		this.error = null;

		this.fetchPromise = (async () => {
			try {
				const r = await api.get<Enrollment[]>('/me/enrollments');
				// api.get returns { data, error } — HTTP failures land in
				// r.error, they do not throw. Surface them on this.error so
				// subscribers can render a retry state.
				if (r.error) {
					this.error = r.error.message || 'Failed to load academy_enrollments';
					this.academy_enrollments = [];
				} else {
					this.academy_enrollments = r.data ?? [];
				}
			} catch (e) {
				this.error = e instanceof Error ? e.message : 'Failed to load academy_enrollments';
				this.academy_enrollments = [];
			} finally {
				this.loading = false;
				this.fetchPromise = null;
			}
		})();

		return this.fetchPromise;
	}

	/**
	 * Force a re-fetch of academy_enrollments, discarding cached data. Called after
	 * mutations that change enrollment state (enroll, mark-complete, progress
	 * update). All subscribed widgets update reactively.
	 *
	 * A generation counter ensures an older in-flight request (e.g. from a
	 * slow refresh started just before a mutation) cannot overwrite newer state.
	 */
	async refresh(): Promise<void> {
		if (!clientHasSession()) {
			this.academy_enrollments = [];
			this.loading = false;
			this.error = null;
			return;
		}

		this.requestGen += 1;
		const myGen = this.requestGen;
		// Drop the stale in-flight promise reference. Its handler will check
		// the generation before mutating state.
		this.fetchPromise = null;
		this.academy_enrollments = null;
		this.loading = true;
		this.error = null;

		this.fetchPromise = (async () => {
			try {
				const r = await api.get<Enrollment[]>('/me/enrollments');
				if (this.requestGen === myGen) {
					if (r.error) {
						this.error = r.error.message || 'Failed to load academy_enrollments';
						this.academy_enrollments = [];
					} else {
						this.academy_enrollments = r.data ?? [];
					}
				}
			} catch (e) {
				if (this.requestGen === myGen) {
					this.error = e instanceof Error ? e.message : 'Failed to load academy_enrollments';
					this.academy_enrollments = [];
				}
			} finally {
				if (this.requestGen === myGen) {
					this.loading = false;
				}
				this.fetchPromise = null;
			}
		})();

		return this.fetchPromise;
	}

	/** Reset to initial state (e.g. on logout). */
	reset(): void {
		this.academy_enrollments = null;
		this.loading = false;
		this.error = null;
		this.fetchPromise = null;
		this.requestGen += 1;
	}

	/** Convenience: true if the user is enrolled in a given track. */
	isEnrolled(trackId: string): boolean {
		return this.academy_enrollments?.some((e) => e.track_id === trackId) ?? false;
	}

	/** Convenience: get the enrollment for a given track, or null. */
	getByTrack(trackId: string): Enrollment | null {
		return this.academy_enrollments?.find((e) => e.track_id === trackId) ?? null;
	}
}

export const enrollmentsStore = new EnrollmentsStore();
