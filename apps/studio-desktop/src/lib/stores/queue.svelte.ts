/**
 * Queue state store.
 * Persists FileState[] to localStorage.
 */

import { type FileState } from '$lib/types/queue';
import type { ScannedFile } from '$lib/services/file-scanner';

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

	function mergeScanned(scanned: ScannedFile[]): void {
		files = scanned.map((sf) => {
			const prev = files.find((f) => f.relativePath === sf.relativePath);
			const lastPublishedHash = prev?.lastPublishedHash ?? '';
			const lastPublishedAt = prev?.lastPublishedAt ?? '';
			const contentId = prev?.contentId;
			const lastError = prev?.lastError ?? '';

			let syncState: FileState['syncState'] = 'new';
			if (prev) {
				if (sf.hash === prev.lastHash) {
					syncState = prev.syncState;
				} else if (sf.hash === lastPublishedHash) {
					syncState = 'published';
				} else {
					syncState = 'modified';
				}
			}

			return {
				relativePath: sf.relativePath,
				contentType: sf.contentType,
				slug: sf.slug,
				title: sf.title,
				lastHash: sf.hash,
				lastPublishedHash,
				lastPublishedAt,
				syncState,
				lastError,
				contentId
			};
		});
		save();
	}

	return {
		get files() {
			return files;
		},
		setFiles,
		updateFile,
		mergeScanned,
		load,
		save
	};
}

export const queueStore = createQueueStore();
