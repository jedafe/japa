/**
 * Queue state store.
 * Persists FileState[] to localStorage.
 */

import { type FileState } from '$lib/types/queue';

export const FILE_STATES_KEY = 'japa-studio:file-states';

function createQueueStore() {
	let files = $state<FileState[]>([]);

	function load(): void {
		if (typeof localStorage === 'undefined') return;
		try {
			const raw = localStorage.getItem(FILE_STATES_KEY);
			if (!raw) return;
			const parsed = JSON.parse(raw) as unknown;
			if (Array.isArray(parsed)) {
				files = parsed as FileState[];
			}
		} catch {
			// ignore malformed state
		}
	}

	function save(): void {
		if (typeof localStorage === 'undefined') return;
		localStorage.setItem(FILE_STATES_KEY, JSON.stringify(files));
	}

	function setFiles(states: FileState[]): void {
		files = states;
		save();
	}

	function updateFile(relativePath: string, partial: Partial<FileState>): void {
		files = files.map((file) =>
			file.relativePath === relativePath ? { ...file, ...partial } : file
		);
		save();
	}

	return {
		get files() {
			return files;
		},
		setFiles,
		updateFile,
		load,
		save
	};
}

export const queueStore = createQueueStore();
