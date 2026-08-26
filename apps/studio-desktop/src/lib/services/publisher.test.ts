import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { publishContent } from './publisher';
import { queueStore } from '$lib/stores/queue.svelte';
import { syncLogStore } from '$lib/stores/sync-log.svelte';
import {
	SETTINGS_KEY,
	defaultSettings,
	setSettingsCache,
	type StudioSettings
} from './settings';

vi.mock('$lib/services/markdown', () => ({
	markdownToHtml: vi.fn((text: string) => `<p>${text}</p>`)
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

function okJson<T>(data: T): Response {
	return {
		ok: true,
		status: 200,
		statusText: 'OK',
		json: () => Promise.resolve({ data })
	} as unknown as Response;
}

function errJson(error: { code: string; message: string }, status = 400): Response {
	return {
		ok: false,
		status,
		statusText: 'Bad Request',
		json: () => Promise.resolve({ error })
	} as unknown as Response;
}

function defaultFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
	const url = new URL(String(input));
	const path = url.pathname;
	const method = init?.method ?? 'GET';

	if (path === '/api/v1/admin/personas' && method === 'GET') {
		return Promise.resolve(okJson([{ id: 'persona-1', user_id: 'user-1', profile_name: 'Ada' }]));
	}
	if (path === '/api/v1/content' && method === 'POST') {
		return Promise.resolve(
			okJson({ id: 'content-1', slug: 'my-post', status: 'in_review', visibility: 'public', content_type: 'guide', title: 'My Post' })
		);
	}
	if (path === '/api/v1/content/content-1' && method === 'PATCH') {
		return Promise.resolve(
			okJson({ id: 'content-1', slug: 'my-post', status: 'in_review', visibility: 'public', content_type: 'guide', title: 'My Post' })
		);
	}
	if (path === '/api/v1/content/content-1/publish' && method === 'POST') {
		return Promise.resolve(
			okJson({ id: 'content-1', slug: 'my-post', status: 'published', visibility: 'public', content_type: 'guide', title: 'My Post' })
		);
	}
	if (path === '/api/v1/content/content-1/reassign-author' && method === 'POST') {
		return Promise.resolve(okJson(null));
	}
	if (path === '/api/v1/content/content-1/tags' && method === 'GET') {
		return Promise.resolve(okJson([]));
	}
	if (path === '/api/v1/content/content-1/tags' && method === 'POST') {
		return Promise.resolve(okJson({ added: 1 }));
	}
	if (path.startsWith('/api/v1/content/content-1/tags/') && method === 'DELETE') {
		return Promise.resolve(okJson(null));
	}

	return Promise.resolve(errJson({ code: 'unexpected', message: `Unexpected ${method} ${path}` }));
}

beforeEach(async () => {
	vi.stubGlobal('localStorage', createStorage());
	globalThis.fetch = vi.fn(defaultFetch);
	setSettingsCache(defaultSettings());
	await queueStore.setFiles([]);
	await syncLogStore.clear();
});

afterEach(() => {
	vi.unstubAllGlobals();
});

function buildFrontmatter(overrides: Record<string, unknown> = {}): Record<string, unknown> {
	return {
		title: 'My Post',
		slug: 'my-post',
		content_type: 'guide',
		guide_category: 'relocation',
		status: 'in_review',
		visibility: 'public',
		space_id: 'space-1',
		persona_id: 'persona-1',
		tags: ['tag-a', 'tag-b'],
		...overrides
	};
}

const sampleBody = 'This is the body content with enough length for publishing.';
const sampleRaw = `---\ntitle: My Post\n---\n\n${sampleBody}`;

describe('publishContent', () => {
	it('fails when API settings are missing', async () => {
		setSettings({ baseUrl: '', apiKey: '' });
		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter(),
			body: sampleBody
		});
		expect(result.ok).toBe(false);
		expect(result.error).toContain('API settings are not configured');
	});

	it('fails when the file path content_type does not match frontmatter', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000 });
		const result = await publishContent({
			relativePath: 'jc-content/blog/my-post.md',
			frontmatter: buildFrontmatter(),
			body: sampleBody
		});
		expect(result.ok).toBe(false);
		expect(result.error).toContain('Content type mismatch');
	});

	it('fails and logs validation errors', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000 });
		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: { content_type: 'guide' },
			body: ''
		});
		expect(result.ok).toBe(false);
		expect(result.error).toContain('Title is required');
	});

	it('creates content and reassigns authorship in operator mode', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'operator' });
		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter(),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(true);
		expect(result.contentId).toBe('content-1');

		const file = queueStore.files.find((f) => f.relativePath === 'jc-content/guide/my-post.md');
		expect(file).toBeDefined();
		expect(file?.syncState).toBe('in_review');
		expect(file?.contentId).toBe('content-1');

		const actions = syncLogStore.entries.map((e) => e.action);
		expect(actions).toContain('create');
		expect(actions).toContain('reassign');
		expect(actions).toContain('tags');
	});

	it('publishes live content when requested status is published', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'operator' });
		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter({ status: 'published' }),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(true);
		expect(result.status).toBe('published');

		const file = queueStore.files.find((f) => f.relativePath === 'jc-content/guide/my-post.md');
		expect(file?.syncState).toBe('published');

		expect(globalThis.fetch).toHaveBeenCalledWith(
			expect.stringContaining('/api/v1/content/content-1/publish'),
			expect.objectContaining({ method: 'POST' })
		);
	});

	it('creates content as the authenticated member in member mode', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'member' });
		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter({ persona_id: undefined }),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(true);

		const reassignCalls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.filter((call) =>
			String(call[0]).includes('/reassign-author')
		);
		expect(reassignCalls).toHaveLength(0);
	});

	it('forces high-stakes content to in_review', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'operator' });
		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter({ guide_category: 'visa', status: 'published' }),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(true);
		expect(result.status).toBe('in_review');

		const publishCalls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.filter((call) =>
			String(call[0]).includes('/publish')
		);
		expect(publishCalls).toHaveLength(0);
	});

	it('accepts scheduled content with a future publish_at', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'operator' });
		const future = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString();
		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter({ status: 'scheduled', publish_at: future }),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(true);

		const createCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.find((call) =>
			String(call[0]) === 'https://api.example.com/api/v1/content' && call[1]?.method === 'POST'
		);
		expect(createCall).toBeDefined();
		const payload = JSON.parse(createCall![1].body);
		expect(payload.status).toBe('scheduled');
		expect(payload.publish_at).toBeTruthy();
	});

	it('updates existing content and diffs tags', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'operator' });
		await queueStore.setFiles([
			{
				relativePath: 'jc-content/guide/my-post.md',
				contentId: 'content-1',
				contentType: 'guide',
				slug: 'my-post',
				title: 'My Post',
				lastHash: 'old-hash',
				lastPublishedHash: 'old-hash',
				lastPublishedAt: '',
				syncState: 'published',
				lastError: ''
			}
		]);

		(globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(async (input, init) => {
			const url = new URL(String(input));
			const path = url.pathname;
			const method = init?.method ?? 'GET';
			if (path === '/api/v1/content/content-1/tags' && method === 'GET') {
				return okJson([{ id: 't1', slug: 'old-tag' }, { id: 't2', slug: 'keep-tag' }]);
			}
			if (path.startsWith('/api/v1/content/content-1/tags/old-tag') && method === 'DELETE') {
				return okJson(null);
			}
			return defaultFetch(input, init);
		});

		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter({ tags: ['keep-tag', 'new-tag'] }),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(true);

		const addTagCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.find(
			(call) => String(call[0]) === 'https://api.example.com/api/v1/content/content-1/tags' && call[1]?.method === 'POST'
		);
		expect(addTagCall).toBeDefined();
		const added = JSON.parse(addTagCall![1].body).tag_slugs;
		expect(added).toContain('new-tag');

		const deleteTagCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.find(
			(call) => String(call[0]).includes('/tags/old-tag') && call[1]?.method === 'DELETE'
		);
		expect(deleteTagCall).toBeDefined();
	});

	it('fails and records queue state when the API rejects creation', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'operator' });
		(globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(async (input, init) => {
			const url = new URL(String(input));
			const path = url.pathname;
			const method = init?.method ?? 'GET';
			if (path === '/api/v1/content' && method === 'POST') {
				return errJson({ code: 'server_error', message: 'Server exploded' });
			}
			return defaultFetch(input, init);
		});

		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter(),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(false);
		const file = queueStore.files.find((f) => f.relativePath === 'jc-content/guide/my-post.md');
		expect(file?.syncState).toBe('failed');
		expect(file?.lastError).toContain('Server exploded');
		expect(syncLogStore.entries.some((e) => e.action === 'failed')).toBe(true);
	});

	it('fails when the selected persona cannot be resolved', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'operator' });
		(globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(async (input, init) => {
			const url = new URL(String(input));
			const path = url.pathname;
			const method = init?.method ?? 'GET';
			if (path === '/api/v1/admin/personas' && method === 'GET') {
				return okJson([]);
			}
			if (path === '/api/v1/content' && method === 'POST') {
				return okJson({ id: 'content-1', slug: 'my-post', status: 'in_review', visibility: 'public', content_type: 'guide', title: 'My Post' });
			}
			return defaultFetch(input, init);
		});

		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter({ persona_id: 'missing-persona' }),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(false);
		expect(result.error).toContain('Could not resolve persona');
		const file = queueStore.files.find((f) => f.relativePath === 'jc-content/guide/my-post.md');
		expect(file?.syncState).toBe('failed');
	});

	it('fails when authorship reassignment is rejected', async () => {
		setSettings({ baseUrl: 'https://api.example.com', apiKey: 'key', accessToken: 'token', tokenExpiresAt: Date.now() + 60000, mode: 'operator' });
		(globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(async (input, init) => {
			const url = new URL(String(input));
			const path = url.pathname;
			const method = init?.method ?? 'GET';
			if (path === '/api/v1/content/content-1/reassign-author' && method === 'POST') {
				return errJson({ code: 'forbidden', message: 'Not allowed to reassign' });
			}
			return defaultFetch(input, init);
		});

		const result = await publishContent({
			relativePath: 'jc-content/guide/my-post.md',
			frontmatter: buildFrontmatter(),
			body: sampleBody,
			raw: sampleRaw
		});

		expect(result.ok).toBe(false);
		expect(result.error).toContain('Authorship reassignment failed');
	});
});
