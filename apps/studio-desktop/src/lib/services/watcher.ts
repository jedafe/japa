/**
 * Filesystem watcher and optional auto-sync for jc-content/.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { watch } from '@tauri-apps/plugin-fs';
import { scanProject } from '$lib/services/file-scanner';
import { publishWorker, shouldAutoPublish } from '$lib/services/publisher';
import { queueStore } from '$lib/stores/queue.svelte';
import { syncLogStore } from '$lib/stores/sync-log.svelte';
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

function shouldAutoPublishForRow(settings: StudioSettings, row: { contentType: string; syncState: string; lastError: string }): boolean {
	if (row.syncState !== 'new' && row.syncState !== 'modified') return false;
	if (row.lastError) return false;
	return shouldAutoPublish(settings, row.contentType);
}

export async function startWatcher(projectRoot: string, onChange?: () => void): Promise<WatcherHandle> {
	await ensureSettingsLoaded();
	await queueStore.load();
	if (currentUnwatch && currentContentDir === projectRoot) {
		return { stop: currentUnwatch, watching: true };
	}

	await stopWatcher();

	const { join } = await import('@tauri-apps/api/path');
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

						// Auto-publish eligible files through the shared worker queue.
						// High-stakes content is forced to in_review inside publishContent.
						const settings = loadSettings();
						const eligible = queueStore.files
							.filter((row) => shouldAutoPublishForRow(settings, row))
							.map((row) => row.relativePath);
						if (eligible.length > 0) {
							publishWorker.enqueue(eligible, 'auto', projectRoot);
						}
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
