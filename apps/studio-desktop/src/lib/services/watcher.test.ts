import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { startWatcher, stopWatcher, isWatching } from './watcher';
import { scanProject, type ScannedFile } from '$lib/services/file-scanner';
import { publishContent } from '$lib/services/publisher';
import { readTextFile, watch } from '@tauri-apps/plugin-fs';
import { queueStore } from '$lib/stores/queue.svelte';
import { syncLogStore } from '$lib/stores/sync-log.svelte';
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

vi.mock('$lib/services/publisher', () => ({
	publishContent: vi.fn()
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
	vi.mocked(readTextFile).mockReset();
	vi.mocked(scanProject).mockReset();
	vi.mocked(publishContent).mockReset();
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

	it('debounces file events into a scan and auto-publish', async () => {
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
		vi.mocked(readTextFile).mockResolvedValue('---\ntitle: My Post\n---\nbody');
		vi.mocked(publishContent).mockResolvedValue({ ok: true });

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
		expect(vi.mocked(readTextFile)).toHaveBeenCalledWith('/projects/sample/jc-content/guide/my-post.md');
		expect(vi.mocked(publishContent)).toHaveBeenCalledWith(
			expect.objectContaining({
				relativePath: 'jc-content/guide/my-post.md',
				frontmatter: expect.objectContaining({ title: 'My Post' }),
				body: 'body'
			})
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
		vi.mocked(readTextFile).mockResolvedValue('---\ntitle: My Post\n---\nbody');
		vi.mocked(publishContent).mockResolvedValue({ ok: true });

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
		expect(vi.mocked(publishContent)).not.toHaveBeenCalled();
	}, 5000);

	it('marks the queue entry as failed when auto-publish fails', async () => {
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
		vi.mocked(readTextFile).mockResolvedValue('---\ntitle: My Post\n---\nbody');
		vi.mocked(publishContent).mockResolvedValue({ ok: false, error: 'API is down' });

		let capturedHandler: (() => void) | undefined;
		vi.mocked(watch).mockImplementation(async (_path, handler) => {
			capturedHandler = handler as () => void;
			return () => {};
		});

		await startWatcher('/projects/sample');

		capturedHandler?.();
		await new Promise((resolve) => setTimeout(resolve, 700));
		await flush();

		const file = queueStore.files.find((f) => f.relativePath === 'jc-content/guide/my-post.md');
		expect(file?.syncState).toBe('failed');
		expect(file?.lastError).toBe('API is down');
		expect(syncLogStore.entries.some((e) => e.action === 'failed')).toBe(true);
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
