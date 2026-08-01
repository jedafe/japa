import type { ComposerStatus, SaveErrorClass } from '$lib/types';
import type { CreateContentInput } from '$lib/api/create';

type Res<T = unknown> = { data: T | null; error: { message: string; status: number } | null; status: number };

export interface ComposerDeps {
	create: (input: CreateContentInput) => Promise<Res<{ id: string; slug: string }>>;
	patch: (id: string, patch: Partial<CreateContentInput>) => Promise<Res>;
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
	lastError = $state<{ cls: SaveErrorClass; message: string } | null>(null);
	savedAt = $state<number | null>(null);
	desiredTags = $state<string[]>([]);
	desiredSeries = $state<string | null>(null);

	#deps: ComposerDeps;
	#opts: Required<ComposerOptions>;
	#rev = 0;
	#savedRev = 0;
	#inFlight = false;
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

	setTags(next: string[]): void {
		// Skip reconciliation when the tag list hasn't changed — onMount and
		// the $effect both call setTags with the same value, and without this
		// guard two concurrent #reconcileTags calls race to create the same
		// tag, producing a spurious 400 on the duplicate insert.
		if (this.desiredTags.length === next.length && this.desiredTags.every((t, i) => t === next[i])) return;
		this.desiredTags = next;
		if (this.contentId) void this.#reconcileTags();
	}

	setSeries(id: string | null): void {
		if (this.desiredSeries === id) return;
		this.desiredSeries = id;
		if (this.contentId) void this.#reconcileSeries();
	}

	/** Persist the current snapshot. Single-flight; reschedules if edits arrive mid-flight. */
	async flush(): Promise<void> {
		if (this.#inFlight) return;
		if (this.contentId && this.#rev === this.#savedRev) return; // nothing new
		// Suppress the initial create when the form doesn't meet the save gate
		// (e.g. title still empty). Edits are kept in local state; the next
		// markDirty after the gate passes will trigger the save.
		if (!this.contentId && !this.#opts.canSave()) {
			this.status = 'pristine';
			return;
		}
		this.#inFlight = true;
		this.lastError = null;
		const rev = this.#rev;
		const body = this.#opts.getSnapshot();
		try {
			if (!this.contentId) {
				this.status = 'creating';
				const res = await this.#deps.create({ ...body, status: 'draft' });
				if (res.error || !res.data) return this.#fail(res.status, res.error?.message);
				this.contentId = res.data.id;
				this.#markSaved(rev);
				await this.#reconcileTags();
				await this.#reconcileSeries();
			} else {
				this.status = 'saving';
				const res = await this.#deps.patch(this.contentId, body);
				if (res.error) return this.#fail(res.status, res.error.message);
				this.#markSaved(rev);
			}
		} finally {
			this.#inFlight = false;
			if (this.status === 'saved' && this.#rev !== this.#savedRev) this.#schedule(0);
		}
	}

	/** Validate, flush the latest draft, then publish or schedule. Returns success. */
	async publish(): Promise<boolean> {
		if (!this.#opts.canPublish()) return false;
		await this.flush();
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

	async #reconcileTags(): Promise<void> {
		if (!this.contentId) return;
		const add = this.desiredTags.filter((t) => !this.#committedTags.includes(t));
		const remove = this.#committedTags.filter((t) => !this.desiredTags.includes(t));
		for (const t of add) await this.#deps.attachTag(this.contentId, t);
		for (const t of remove) await this.#deps.detachTag(this.contentId, t);
		this.#committedTags = [...this.desiredTags];
	}

	async #reconcileSeries(): Promise<void> {
		if (!this.contentId || this.desiredSeries === this.#committedSeries) return;
		if (this.#committedSeries) await this.#deps.unlinkSeries(this.#committedSeries, this.contentId);
		if (this.desiredSeries) await this.#deps.linkSeries(this.desiredSeries, this.contentId, 0);
		this.#committedSeries = this.desiredSeries;
	}
}
