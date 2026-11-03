import type { ComposerStatus, SaveErrorClass } from '$lib/types';
import type { CreateContentInput } from '$lib/api/create';

type Res<T = unknown> = { data: T | null; error: { message: string; status: number } | null; status: number };

export interface ComposerDeps {
	create: (input: CreateContentInput) => Promise<Res<{ id: string; slug: string; updated_at?: string }>>;
	patch: (id: string, patch: Partial<CreateContentInput>) => Promise<Res<{ updated_at?: string }>>;
	publish: (id: string) => Promise<Res>;
	schedule: (id: string, at: string) => Promise<Res>;
	attachTag: (id: string, tagId: string) => Promise<Res>;
	detachTag: (id: string, tagId: string) => Promise<Res>;
	linkSeries: (seriesId: string, contentId: string, position: number) => Promise<Res>;
	unlinkSeries: (seriesId: string, contentId: string) => Promise<Res>;
	classify: (status: number) => SaveErrorClass;
}

export interface ComposerOptions {
	getSnapshot: () => CreateContentInput;
	canPublish: () => boolean;
	getPublishAt: () => string | null;
	/** Gate for autosave — return false to suppress a save cycle (e.g. empty title). */
	canSave?: () => boolean;
	debounceMs?: number;
	maxRetries?: number;
}

export class ComposerState {
	status = $state<ComposerStatus>('pristine');
	contentId = $state<string | null>(null);
	/** Latest server-side updated_at — sent as expected_updated_at on each
	 *  PATCH so the U1 optimistic lock detects concurrent edits (BUG-CE-006).
	 *  Must advance after every successful save or the next PATCH 409s. */
	expectedUpdatedAt = $state<string | null>(null);
	lastError = $state<{ cls: SaveErrorClass; message: string } | null>(null);
	savedAt = $state<number | null>(null);
	desiredTags = $state<string[]>([]);
	desiredSeries = $state<string | null>(null);

	#deps: ComposerDeps;
	#opts: Required<ComposerOptions>;
	#rev = 0;
	#savedRev = 0;
	#inFlight = false;
	#flightDone: Promise<void> | null = null;
	#retries = 0;
	#timer: ReturnType<typeof setTimeout> | null = null;
	#committedTags: string[] = [];
	#committedSeries: string | null = null;

	constructor(deps: ComposerDeps, opts: ComposerOptions) {
		this.#deps = deps;
		this.#opts = { debounceMs: 2000, maxRetries: 3, canSave: () => true, ...opts };
	}

	/** Call on any body/field edit. Bumps the revision and schedules a debounced save. */
	markDirty(): void {
		this.#rev++;
		if (this.status !== 'creating' && this.status !== 'saving') this.status = 'dirty';
		this.#schedule(this.#opts.debounceMs);
	}

	/** Surface an out-of-band save failure (e.g. from space assignment). */
	setError(status: number, message = ''): void {
		this.#fail(status, message);
	}

	setTags(next: string[]): void {
		// Skip reconciliation when the tag list hasn't changed — onMount and
		// the $effect both call setTags with the same value, and without this
		// guard two concurrent #reconcileTags calls race to create the same
		// tag, producing a spurious 400 on the duplicate insert.
		if (this.desiredTags.length === next.length && this.desiredTags.every((t, i) => t === next[i])) return;
		this.desiredTags = next;
		if (this.contentId) {
			void (async () => {
				const res = await this.#reconcileTags();
				if (res?.error) this.#fail(res.status, res.error.message);
			})();
		}
	}

	setSeries(id: string | null): void {
		if (this.desiredSeries === id) return;
		this.desiredSeries = id;
		if (this.contentId) {
			void (async () => {
				const res = await this.#reconcileSeries();
				if (res?.error) this.#fail(res.status, res.error.message);
			})();
		}
	}

	/** Persist the current snapshot. Single-flight; reschedules if edits arrive mid-flight.
	 *  Callers that need the save to complete (publish) must await the returned
	 *  promise — it resolves after the in-flight request finishes (BUG-CE-007). */
	async flush(): Promise<void> {
		if (this.#inFlight) {
			await this.#flightDone;
			return;
		}
		if (this.contentId && this.#rev === this.#savedRev) return; // nothing new
		// Suppress the initial create when the form doesn't meet the save gate
		// (e.g. title still empty). Edits are kept in local state; the next
		// markDirty after the gate passes will trigger the save.
		if (!this.contentId && !this.#opts.canSave()) {
			this.status = 'pristine';
			return;
		}
		this.#inFlight = true;
		const done = (async () => {
			this.lastError = null;
			const rev = this.#rev;
			const body = this.#opts.getSnapshot();
			try {
				if (!this.contentId) {
					this.status = 'creating';
					const res = await this.#deps.create({ ...body, status: 'draft' });
					if (res.error || !res.data) return this.#fail(res.status, res.error?.message);
					this.contentId = res.data.id;
					if (res.data.updated_at) this.expectedUpdatedAt = res.data.updated_at;
					// Reconcile tags/series before marking saved so a linkage failure
					// leaves the composer in an error/dirty state instead of "saved".
					const tagRes = await this.#reconcileTags();
					if (tagRes?.error) return this.#fail(tagRes.status, tagRes.error.message);
					const seriesRes = await this.#reconcileSeries();
					if (seriesRes?.error) return this.#fail(seriesRes.status, seriesRes.error.message);
					this.#markSaved(rev);
				} else {
					this.status = 'saving';
					const res = await this.#deps.patch(this.contentId, {
						...body,
						expected_updated_at: this.expectedUpdatedAt
					});
					if (res.error) return this.#fail(res.status, res.error.message);
					if (res.data?.updated_at) this.expectedUpdatedAt = res.data.updated_at;
					this.#markSaved(rev);
				}
			} finally {
				this.#inFlight = false;
				if (this.status === 'saved' && this.#rev !== this.#savedRev) this.#schedule(0);
			}
		})();
		this.#flightDone = done;
		return done;
	}

	/** Validate, wait for any in-flight save, persist the latest snapshot,
	 *  then publish or schedule. Returns success (BUG-CE-007). */
	async publish(): Promise<boolean> {
		if (!this.#opts.canPublish()) return false;
		// Bounded: flush() awaits the in-flight save; edits that arrive
		// mid-flight bump #rev and force one more save cycle before we
		// fire /publish — never publish a stale body.
		for (let i = 0; i < 4 && (this.#inFlight || this.#rev !== this.#savedRev); i++) {
			await this.flush();
			if (this.status === 'error') break; // don't retry doomed payloads
		}
		if (!this.contentId || this.status === 'error') return false;
		const at = this.#opts.getPublishAt();
		const res = at
			? await this.#deps.schedule(this.contentId, at)
			: await this.#deps.publish(this.contentId);
		if (res.error) {
			this.#fail(res.status, res.error.message);
			return false;
		}
		this.status = at ? 'scheduled' : 'published';
		return true;
	}

	dispose(): void {
		if (this.#timer) clearTimeout(this.#timer);
	}

	#markSaved(rev: number): void {
		this.#savedRev = rev;
		this.status = 'saved';
		this.savedAt = Date.now();
		this.#retries = 0;
	}

	#fail(status: number, message = ''): void {
		const cls = this.#deps.classify(status);
		this.lastError = { cls, message };
		if ((cls === 'network' || cls === 'server') && this.#retries < this.#opts.maxRetries) {
			this.#retries++;
			this.status = 'dirty';
			this.#schedule(this.#backoff());
		} else {
			this.status = 'error';
		}
	}

	#backoff(): number {
		return Math.min(1000 * 2 ** this.#retries, 15000);
	}

	#schedule(ms: number): void {
		if (this.#timer) clearTimeout(this.#timer);
		this.#timer = setTimeout(() => void this.flush(), ms);
	}

	async #reconcileTags(): Promise<Res | null> {
		if (!this.contentId) return null;
		const add = this.desiredTags.filter((t) => !this.#committedTags.includes(t));
		const remove = this.#committedTags.filter((t) => !this.desiredTags.includes(t));
		for (const t of add) {
			const res = await this.#deps.attachTag(this.contentId, t);
			// 409 = already attached, which is the desired end state.
			if (res.error && res.status !== 409) return res;
		}
		for (const t of remove) {
			const res = await this.#deps.detachTag(this.contentId, t);
			// 404 = already detached, which is the desired end state.
			if (res.error && res.status !== 404) return res;
		}
		this.#committedTags = [...this.desiredTags];
		return null;
	}

	async #reconcileSeries(): Promise<Res | null> {
		if (!this.contentId || this.desiredSeries === this.#committedSeries) return null;
		if (this.#committedSeries) {
			const res = await this.#deps.unlinkSeries(this.#committedSeries, this.contentId);
			if (res.error && res.status !== 404) return res;
		}
		if (this.desiredSeries) {
			const res = await this.#deps.linkSeries(this.desiredSeries, this.contentId, 0);
			if (res.error && res.status !== 409) return res;
		}
		this.#committedSeries = this.desiredSeries;
		return null;
	}
}
