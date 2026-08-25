/**
 * Sync log store.
 * Persists a log of publish/update/reassign/tags/archive/failed attempts.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export const SYNC_LOG_KEY = 'japa-studio:sync-log';

export interface SyncLogEntry {
	id: string;
	relativePath: string;
	action: 'create' | 'update' | 'reassign' | 'tags' | 'archive' | 'failed';
	contentId?: string;
	error?: string;
	createdAt: string;
}

function createSyncLogStore() {
	let entries = $state<SyncLogEntry[]>([]);

	function load(): void {
		if (typeof localStorage === 'undefined') return;
		try {
			const raw = localStorage.getItem(SYNC_LOG_KEY);
			if (!raw) return;
			const parsed = JSON.parse(raw) as unknown;
			if (Array.isArray(parsed)) {
				entries = parsed as SyncLogEntry[];
			}
		} catch {
			// ignore malformed state
		}
	}

	function save(): void {
		if (typeof localStorage === 'undefined') return;
		localStorage.setItem(SYNC_LOG_KEY, JSON.stringify(entries));
	}

	function uid(): string {
		return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
	}

	function append(entry: Omit<SyncLogEntry, 'id' | 'createdAt'>): void {
		const full: SyncLogEntry = {
			...entry,
			id: uid(),
			createdAt: new Date().toISOString()
		};
		entries = [full, ...entries].slice(0, 1000);
		save();
	}

	function clear(): void {
		entries = [];
		save();
	}

	return {
		get entries() {
			return entries;
		},
		load,
		save,
		append,
		clear
	};
}

export const syncLogStore = createSyncLogStore();
