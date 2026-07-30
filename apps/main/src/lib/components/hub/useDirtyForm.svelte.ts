import type { SaveErrorClass } from '$lib/types';

type Res = { data: unknown; error: { message: string; status: number } | null; status: number };

function defaultClassify(status: number): SaveErrorClass {
	if (status === 0) return 'network';
	if (status === 409) return 'conflict';
	if (status >= 500) return 'server';
	return 'validation';
}

/** Structural deep-equal for plain JSON snapshots (settings values are JSON-serializable). */
function deepEqual(a: unknown, b: unknown): boolean {
	return JSON.stringify(a) === JSON.stringify(b);
}

/** Deep-clone via JSON — required because $state proxies are not structuredClone-able. */
function clone<T>(v: T): T {
	return JSON.parse(JSON.stringify(v)) as T;
}

/**
 * Dirty-tracked form rune. Holds reactive `values`, a private snapshot, and a
 * save promise. `dirty` is a $derived-style getter comparing values to the
 * snapshot via structural deep-equal (so nested-map edits — e.g. privacy
 * toggles — surface as dirty without manual diffing).
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Settings forms)
 */
export class DirtyForm<T extends Record<string, unknown>> {
	values = $state<T>({} as T);
	saving = $state(false);
	error = $state<{ cls: SaveErrorClass; message: string } | null>(null);

	#snapshot: T;
	#save: (v: T) => Promise<Res>;
	#classify: (s: number) => SaveErrorClass;

	constructor(initial: T, save: (v: T) => Promise<Res>, classify = defaultClassify) {
		this.values = clone(initial);
		this.#snapshot = clone(initial);
		this.#save = save;
		this.#classify = classify;
	}

	get dirty(): boolean {
		return !deepEqual(this.values, this.#snapshot);
	}

	set(patch: Partial<T>): void {
		this.values = { ...this.values, ...patch };
	}

	/**
	 * Set values AND advance the snapshot so the form is clean.
	 * Use this after an *already-persisted* direct API call (e.g. avatar/cover
	 * upload) to avoid a false dirty state.
	 */
	setSaved(patch: Partial<T>): void {
		this.values = { ...this.values, ...patch };
		this.#snapshot = clone(this.values);
	}

	async save(): Promise<boolean> {
		if (!this.dirty) return true;
		this.saving = true;
		this.error = null;
		const res = await this.#save(this.values);
		this.saving = false;
		if (res.error) {
			this.error = { cls: this.#classify(res.status), message: res.error.message };
			return false;
		}
		this.#snapshot = clone(this.values);
		return true;
	}

	reset(): void {
		this.values = clone(this.#snapshot);
		this.error = null;
	}
}
