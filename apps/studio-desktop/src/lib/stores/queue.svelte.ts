/**
 * Queue state store.
 * Persists FileState[] to the local state layer (SQLite in Tauri, localStorage in browser).
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { type FileState } from '$lib/types/queue';
import type { ScannedFile } from '$lib/services/file-scanner';
import * as db from '$lib/services/db';

// Keeps the queue UI's canonical fields (modifiedAt, lastPublishedAt, syncState,
// status) in sync with their display aliases.
function normalizeFileState(file: FileState): FileState {
	return {
		...file,
		lastModified: file.modifiedAt || file.lastModified,
		lastPublished: file.lastPublishedAt || file.lastPublished,
		japaStatus: file.status || file.japaStatus,
		localStatus: file.syncState
	};
}

function createQueueStore() {
	let files = $state<FileState[]>([]);

	async function load(): Promise<void> {
		files = (await db.loadFileStates()).map(normalizeFileState);
	}

	async function save(): Promise<void> {
		await db.saveAllFileStates(files);
	}

	async function setFiles(states: FileState[]): Promise<void> {
		files = states.map(normalizeFileState);
		await db.saveAllFileStates(files);
	}

	async function updateFile(relativePath: string, partial: Partial<FileState>): Promise<void> {
		// Only overwrite with defined values so an unset optional field does not
		// clear the canonical value it aliases (e.g. modifiedAt -> lastModified).
		const definedPartial: Partial<FileState> = {};
		for (const [key, value] of Object.entries(partial)) {
			if (value !== undefined) {
				(definedPartial as Record<string, unknown>)[key] = value;
			}
		}

		files = files.map((file) =>
			file.relativePath === relativePath ? normalizeFileState({ ...file, ...definedPartial }) : file
		);
		const updated = files.find((f) => f.relativePath === relativePath);
		if (updated) {
			await db.saveFileState(updated);
		}
	}

	async function upsertFile(relativePath: string, partial: Partial<FileState>): Promise<void> {
		const existing = files.find((f) => f.relativePath === relativePath);
		if (existing) {
			await updateFile(relativePath, partial);
			return;
		}
		const created: FileState = normalizeFileState({
			relativePath,
			contentType: partial.contentType ?? '',
			slug: partial.slug ?? '',
			title: partial.title ?? '',
			lastHash: partial.lastHash ?? '',
			lastPublishedHash: partial.lastPublishedHash ?? '',
			lastPublishedAt: partial.lastPublishedAt ?? '',
			syncState: partial.syncState ?? 'new',
			lastError: partial.lastError ?? '',
			contentId: partial.contentId,
			spaceId: partial.spaceId,
			personaId: partial.personaId,
			guideCategory: partial.guideCategory,
			status: partial.status,
			visibility: partial.visibility,
			modifiedAt: partial.modifiedAt
		});
		files = [...files, created];
		await db.saveFileState(created);
	}

	async function deleteFile(relativePath: string): Promise<void> {
		files = files.filter((f) => f.relativePath !== relativePath);
		await db.deleteFileState(relativePath);
	}

	async function mergeScanned(scanned: ScannedFile[]): Promise<void> {
		const scannedPaths = new Set(scanned.map((sf) => sf.relativePath));

		const keptFiles: FileState[] = files
			.filter((f) => !scannedPaths.has(f.relativePath))
			.map((f) => {
				// Files that disappeared from disk are archived if they were ever published,
				// otherwise they are dropped below.
				if (f.contentId && (f.syncState === 'published' || f.syncState === 'in_review')) {
					return { ...f, syncState: 'archived' as const };
				}
				return f;
			})
			.filter((f) => f.syncState === 'archived');

		const merged = scanned.map((sf) => {
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
				} else if (prev.syncState === 'failed') {
					// a previously failed file that changed on disk is still treated
					// as modified; auto-sync will skip until the user retries.
					syncState = 'modified';
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
				contentId,
				spaceId: sf.spaceId ?? prev?.spaceId,
				personaId: sf.personaId ?? prev?.personaId,
				guideCategory: sf.guideCategory ?? prev?.guideCategory,
				status: sf.status ?? prev?.status,
				visibility: sf.visibility ?? prev?.visibility,
				modifiedAt: sf.modifiedAt
			};
		});

		files = [...keptFiles, ...merged].map(normalizeFileState);
		await db.saveAllFileStates(files);
	}

	return {
		get files() {
			return files;
		},
		setFiles,
		updateFile,
		upsertFile,
		deleteFile,
		mergeScanned,
		load,
		save
	};
}

export const queueStore = createQueueStore();
