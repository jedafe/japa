/**
 * Filesystem watcher and optional auto-sync for jc-content/.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { watch, readTextFile } from '@tauri-apps/plugin-fs';
import { join } from '@tauri-apps/api/path';
import { scanProject, type ScannedFile } from '$lib/services/file-scanner';
import { parseFrontmatter } from '$lib/services/markdown';
import { publishContent } from '$lib/services/publisher';
import { queueStore } from '$lib/stores/queue.svelte';
import { type FileState } from '$lib/types/queue';
import { loadSettings, type StudioSettings } from '$lib/services/settings';

export interface WatcherHandle {
	stop: () => void;
	watching: boolean;
}

let currentUnwatch: (() => void) | null = null;
let currentContentDir: string | null = null;
let syncing = false;

function shouldAutoPublish(settings: StudioSettings, contentType: string): boolean {
	if (!settings.autoSync) return false;
	if (settings.autoSyncContentTypes.length === 0) return true;
	return settings.autoSyncContentTypes.includes(contentType);
}

async function autoPublish(rows: FileState[], projectRoot: string): Promise<void> {
	for (const row of rows) {
		if (row.syncState !== 'new' && row.syncState !== 'modified') continue;
		if (row.lastError) continue;
		const settings = loadSettings();
		if (!shouldAutoPublish(settings, row.contentType)) continue;

		const filePath = await join(projectRoot, row.relativePath);
		const text = await readTextFile(filePath);
		const { frontmatter, body } = parseFrontmatter(text);

		const result = await publishContent({
			relativePath: row.relativePath,
			frontmatter,
			body,
			raw: text
		});

		if (!result.ok) {
			queueStore.updateFile(row.relativePath, { syncState: 'failed', lastError: result.error ?? '' });
		}
	}
}

export async function startWatcher(projectRoot: string, onChange?: () => void): Promise<WatcherHandle> {
	if (currentUnwatch && currentContentDir === projectRoot) {
		return { stop: currentUnwatch, watching: true };
	}

	await stopWatcher();

	const contentDir = await join(projectRoot, 'jc-content');
	currentContentDir = projectRoot;

	try {
		currentUnwatch = await watch(
			contentDir,
			async () => {
				onChange?.();
				if (syncing) return;
				syncing = true;
				try {
					const scanned = await scanProject(projectRoot);
					queueStore.mergeScanned(scanned);
					await autoPublish(queueStore.files, projectRoot);
				} catch {
					// best-effort
				} finally {
					syncing = false;
				}
			},
			{ recursive: true }
		);

		return {
			stop: () => {
				if (currentUnwatch) {
					currentUnwatch();
					currentUnwatch = null;
					currentContentDir = null;
				}
			},
			watching: true
		};
	} catch (e) {
		currentContentDir = null;
		throw e;
	}
}

export async function stopWatcher(): Promise<void> {
	if (currentUnwatch) {
		currentUnwatch();
		currentUnwatch = null;
		currentContentDir = null;
	}
}

export function isWatching(): boolean {
	return currentUnwatch !== null;
}
