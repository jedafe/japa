/**
 * Sync log store.
 * Persists a log of publish/update/reassign/tags/archive/failed attempts.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import * as db from '$lib/services/db';

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

	async function load(): Promise<void> {
		entries = await db.loadSyncLog();
	}

	async function save(): Promise<void> {
		await db.clearSyncLog();
		for (const entry of entries) {
			await db.appendSyncLog(entry);
		}
	}

	function uid(): string {
		return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
	}

	async function append(entry: Omit<SyncLogEntry, 'id' | 'createdAt'>): Promise<void> {
		const full: SyncLogEntry = {
			...entry,
			id: uid(),
			createdAt: new Date().toISOString()
		};
		entries = [full, ...entries].slice(0, 1000);
		await db.appendSyncLog(full);
	}

	async function clear(): Promise<void> {
		entries = [];
		await db.clearSyncLog();
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
