import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
	loadSettings,
	saveSettings,
	loadFileStates,
	saveAllFileStates,
	saveFileState,
	loadSyncLog,
	appendSyncLog,
	clearSyncLog,
	loadEditorDraft,
	saveEditorDraft,
	type EditorDraft
} from './db';
import { defaultSettings, type StudioSettings } from './settings';
import type { FileState } from '$lib/types/queue';

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

beforeEach(() => {
	vi.stubGlobal('localStorage', createStorage());
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('db fallback (localStorage)', () => {
	it('round-trips settings', async () => {
		const settings: StudioSettings = { ...defaultSettings(), projectName: 'Test', baseUrl: 'https://x' };
		await saveSettings(settings);
		const loaded = await loadSettings();
		expect(loaded).toEqual(settings);
	});

	it('round-trips file states', async () => {
		const files: FileState[] = [
			{
				relativePath: 'jc-content/guide/a.md',
				contentType: 'guide',
				slug: 'a',
				title: 'A',
				lastHash: 'h1',
				lastPublishedHash: '',
				lastPublishedAt: '',
				syncState: 'new',
				lastError: ''
			}
		];
		await saveAllFileStates(files);
		const loaded = await loadFileStates();
		expect(loaded).toEqual(files);
	});

	it('updates a single file state', async () => {
		const file: FileState = {
			relativePath: 'jc-content/guide/a.md',
			contentType: 'guide',
			slug: 'a',
			title: 'A',
			lastHash: 'h1',
			lastPublishedHash: '',
			lastPublishedAt: '',
			syncState: 'new',
			lastError: ''
		};
		await saveFileState(file);
		await saveFileState({ ...file, syncState: 'published' });
		const loaded = await loadFileStates();
		expect(loaded).toHaveLength(1);
		expect(loaded[0]?.syncState).toBe('published');
	});

	it('round-trips sync log entries', async () => {
		await appendSyncLog({
			id: '1',
			relativePath: 'jc-content/guide/a.md',
			action: 'create',
			createdAt: '2026-01-01T00:00:00.000Z'
		});
		await appendSyncLog({
			id: '2',
			relativePath: 'jc-content/guide/b.md',
			action: 'failed',
			error: 'x',
			createdAt: '2026-01-02T00:00:00.000Z'
		});
		const loaded = await loadSyncLog();
		expect(loaded).toHaveLength(2);
		expect(loaded[0]?.action).toBe('failed');
		await clearSyncLog();
		expect(await loadSyncLog()).toHaveLength(0);
	});

	it('round-trips editor drafts', async () => {
		const draft: EditorDraft = {
			body: 'hello',
			frontmatter: '{"title":"x"}',
			isDirty: true,
			updatedAt: '2026-01-01T00:00:00.000Z'
		};
		await saveEditorDraft('jc-content/guide/a.md', draft);
		const loaded = await loadEditorDraft('jc-content/guide/a.md');
		expect(loaded).toEqual(draft);
		const missing = await loadEditorDraft('jc-content/guide/b.md');
		expect(missing).toBeNull();
	});
});
