/**
 * Filesystem watcher and optional auto-sync for jc-content/.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { watch, readTextFile } from '@tauri-apps/plugin-fs';
import { join } from '@tauri-apps/api/path';
import { scanProject } from '$lib/services/file-scanner';
import { parseFrontmatter } from '$lib/services/markdown';
import { publishContent } from '$lib/services/publisher';
import { queueStore } from '$lib/stores/queue.svelte';
import { syncLogStore } from '$lib/stores/sync-log.svelte';
import { type FileState } from '$lib/types/queue';
import { ensureSettingsLoaded, loadSettings, type StudioSettings } from '$lib/services/settings';

export interface WatcherHandle {
	stop: () => void;
	watching: boolean;
}

let currentUnwatch: (() => void) | null = null;
let currentContentDir: string | null = null;
let syncing = false;
let debounceTimer: ReturnType<typeof setTimeout> | null = null;

const DEBOUNCE_MS = 500;

function shouldAutoPublish(settings: StudioSettings, contentType: string): boolean {
	if (!settings.autoSync) return false;
	if (settings.autoSyncContentTypes.length === 0) return true;
	return settings.autoSyncContentTypes.includes(contentType);
}

async function autoPublish(rows: FileState[], projectRoot: string): Promise<void> {
	await ensureSettingsLoaded();
	for (const row of rows) {
		if (row.syncState !== 'new' && row.syncState !== 'modified') continue;
		if (row.lastError) continue;
		const settings = loadSettings();
		if (!shouldAutoPublish(settings, row.contentType)) continue;

		try {
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
				await queueStore.upsertFile(row.relativePath, { syncState: 'failed', lastError: result.error ?? '' });
				await syncLogStore.append({
					relativePath: row.relativePath,
					action: 'failed',
					error: result.error ?? 'Auto-publish failed.'
				});
			}
		} catch (e) {
			const msg = e instanceof Error ? e.message : String(e);
			await queueStore.upsertFile(row.relativePath, { syncState: 'failed', lastError: msg });
			await syncLogStore.append({
				relativePath: row.relativePath,
				action: 'failed',
				error: `Auto-publish error: ${msg}`
			});
		}
	}
}

export async function startWatcher(projectRoot: string, onChange?: () => void): Promise<WatcherHandle> {
	await ensureSettingsLoaded();
	await queueStore.load();
	if (currentUnwatch && currentContentDir === projectRoot) {
		return { stop: currentUnwatch, watching: true };
	}

	await stopWatcher();

	const contentDir = await join(projectRoot, 'jc-content');
	currentContentDir = projectRoot;

	try {
		currentUnwatch = await watch(
			contentDir,
			() => {
				onChange?.();
				if (debounceTimer) clearTimeout(debounceTimer);
				debounceTimer = setTimeout(async () => {
					if (syncing) return;
					syncing = true;
					try {
						const scanned = await scanProject(projectRoot);
						await queueStore.mergeScanned(scanned);
						await autoPublish(queueStore.files, projectRoot);
					} catch (e) {
						const msg = e instanceof Error ? e.message : String(e);
						await syncLogStore.append({
							relativePath: 'jc-content/',
							action: 'failed',
							error: `Watcher rescan failed: ${msg}`
						});
					} finally {
						syncing = false;
					}
				}, DEBOUNCE_MS);
			},
			{ recursive: true }
		);

		return {
			stop: () => {
				if (currentUnwatch) {
					currentUnwatch();
					currentUnwatch = null;
					currentContentDir = null;
					if (debounceTimer) {
						clearTimeout(debounceTimer);
						debounceTimer = null;
					}
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
		if (debounceTimer) {
			clearTimeout(debounceTimer);
			debounceTimer = null;
		}
	}
}

export function isWatching(): boolean {
	return currentUnwatch !== null;
}
