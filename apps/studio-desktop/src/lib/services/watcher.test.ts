import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { startWatcher, stopWatcher, isWatching } from './watcher';
import { scanProject, type ScannedFile } from '$lib/services/file-scanner';
import { queueStore } from '$lib/stores/queue.svelte';
import { syncLogStore } from '$lib/stores/sync-log.svelte';
import { watch } from '@tauri-apps/plugin-fs';
import {
	SETTINGS_KEY,
	defaultSettings,
	setSettingsCache,
	type StudioSettings
} from './settings';

vi.mock('@tauri-apps/plugin-fs', async () => ({
	watch: vi.fn(),
	readTextFile: vi.fn(),
	readDir: vi.fn(),
	stat: vi.fn(),
	BaseDirectory: {}
}));

vi.mock('@tauri-apps/api/path', () => ({
	join: vi.fn((...parts: string[]) => parts.join('/'))
}));

vi.mock('$lib/services/file-scanner', () => ({
	scanProject: vi.fn()
}));

const mockEnqueue = vi.hoisted(() => vi.fn());

vi.mock('$lib/services/publisher', () => ({
	publishWorker: {
		enqueue: mockEnqueue,
		get current() {
			return null;
		},
		get pending() {
			return 0;
		}
	},
	shouldAutoPublish: (settings: StudioSettings, contentType: string) => {
		if (!settings.autoSync) return false;
		if (settings.autoSyncContentTypes.length === 0) return true;
		return settings.autoSyncContentTypes.includes(contentType);
	},
	isPublishing: { set: vi.fn(), subscribe: vi.fn() }
}));

function createStorage(): Storage {
	const store = new Map<string, string>();
	return {
		getItem: (key: string) => store.get(key) ?? null,
		setItem: (key: string, value: string) => store.set(key, value),
		removeItem: (key: string) => store.delete(key),
		clear: () => store.clear(),
		key: (index: number) => Array.from(store.keys())[index] ?? null,
		get length() {
			return store.size;
		}
	} as Storage;
}

function setSettings(partial: Partial<StudioSettings>): void {
	const settings = { ...defaultSettings(), ...partial };
	localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
	setSettingsCache(settings);
}

function flush() {
	return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(async () => {
	vi.stubGlobal('localStorage', createStorage());
	setSettingsCache(defaultSettings());
	await queueStore.setFiles([]);
	await syncLogStore.clear();
	vi.mocked(watch).mockReset();
	vi.mocked(scanProject).mockReset();
	mockEnqueue.mockReset();
});

afterEach(async () => {
	await stopWatcher();
	vi.unstubAllGlobals();
});

describe('watcher', () => {
	it('starts watching the jc-content directory and stops on demand', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', autoSync: true });
		vi.mocked(watch).mockResolvedValue(() => {});

		const handle = await startWatcher('/projects/sample');
		expect(handle.watching).toBe(true);
		expect(isWatching()).toBe(true);
		expect(vi.mocked(watch)).toHaveBeenCalledWith(
			'/projects/sample/jc-content',
			expect.any(Function),
			{ recursive: true }
		);

		handle.stop();
		expect(isWatching()).toBe(false);
	});

	it('returns the existing handle when asked to watch the same project again', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', autoSync: true });
		const unwatch = vi.fn();
		vi.mocked(watch).mockResolvedValue(unwatch);

		const first = await startWatcher('/projects/sample');
		const second = await startWatcher('/projects/sample');

		expect(second.watching).toBe(true);
		expect(vi.mocked(watch)).toHaveBeenCalledTimes(1);
		expect(unwatch).not.toHaveBeenCalled();

		first.stop();
	});

	it('debounces file events into a scan and enqueues auto-publish', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', autoSync: true });

		const scanned: ScannedFile[] = [
			{
				relativePath: 'jc-content/guide/my-post.md',
				contentType: 'guide',
				slug: 'my-post',
				title: 'My Post',
				hash: 'abc123'
			}
		];
		vi.mocked(scanProject).mockResolvedValue(scanned);

		let capturedHandler: (() => void) | undefined;
		vi.mocked(watch).mockImplementation(async (_path, handler) => {
			capturedHandler = handler as () => void;
			return () => {};
		});

		await startWatcher('/projects/sample');

		capturedHandler?.();
		await new Promise((resolve) => setTimeout(resolve, 700));
		await flush();

		expect(vi.mocked(scanProject)).toHaveBeenCalledWith('/projects/sample');
		expect(mockEnqueue).toHaveBeenCalledWith(
			['jc-content/guide/my-post.md'],
			'auto',
			'/projects/sample'
		);
	}, 5000);

	it('skips auto-publish when the content type is not in the auto-sync allowlist', async () => {
		setSettings({
			baseUrl: 'https://api.example.com',
			apiKey: 'key',
			autoSync: true,
			autoSyncContentTypes: ['guide']
		});

		const scanned: ScannedFile[] = [
			{
				relativePath: 'jc-content/blog/my-post.md',
				contentType: 'blog',
				slug: 'my-post',
				title: 'My Post',
				hash: 'abc123'
			}
		];
		vi.mocked(scanProject).mockResolvedValue(scanned);

		let capturedHandler: (() => void) | undefined;
		vi.mocked(watch).mockImplementation(async (_path, handler) => {
			capturedHandler = handler as () => void;
			return () => {};
		});

		await startWatcher('/projects/sample');

		capturedHandler?.();
		await new Promise((resolve) => setTimeout(resolve, 700));
		await flush();

		expect(vi.mocked(scanProject)).toHaveBeenCalled();
		expect(mockEnqueue).not.toHaveBeenCalled();
	}, 5000);

	it('coalesces rapid events into a single scan', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', autoSync: false });
		vi.mocked(scanProject).mockResolvedValue([]);

		let capturedHandler: (() => void) | undefined;
		vi.mocked(watch).mockImplementation(async (_path, handler) => {
			capturedHandler = handler as () => void;
			return () => {};
		});

		await startWatcher('/projects/sample');

		capturedHandler?.();
		capturedHandler?.();
		capturedHandler?.();
		await new Promise((resolve) => setTimeout(resolve, 700));
		await flush();

		expect(vi.mocked(scanProject)).toHaveBeenCalledTimes(1);
	}, 5000);
});
