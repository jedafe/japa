import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ComposerState, type ComposerDeps, type ComposerOptions } from './composerState.svelte';

const ok = (data: unknown = {}, status = 200) => ({ data, error: null, status });
const fail = (status: number, message = 'x') => ({ data: null, error: { code: 'e', message, status }, status });

function makeDeps(over: Partial<ComposerDeps> = {}): ComposerDeps {
	return {
		create: vi.fn().mockResolvedValue(ok({ id: 'c1', slug: 's' }, 201)),
		patch: vi.fn().mockResolvedValue(ok({ id: 'c1' })),
		publish: vi.fn().mockResolvedValue(ok()),
		schedule: vi.fn().mockResolvedValue(ok()),
		attachTag: vi.fn().mockResolvedValue(ok()),
		detachTag: vi.fn().mockResolvedValue(ok()),
		linkSeries: vi.fn().mockResolvedValue(ok()),
		unlinkSeries: vi.fn().mockResolvedValue(ok()),
		classify: (s: number) => (s === 0 ? 'network' : s === 409 ? 'conflict' : s >= 500 ? 'server' : 'validation'),
		...over
	};
}

function makeState(deps: ComposerDeps, opts: Partial<ComposerOptions> = {}) {
	const options: ComposerOptions = {
		getSnapshot: () => ({ content_type: 'blog_post', title: 'Hi', body_html: '<p>x</p>' }),
		canPublish: () => true,
		getPublishAt: () => null,
		debounceMs: 1_000_000, // effectively disable auto-fire; tests call flush() directly
		maxRetries: 2,
		...opts
	};
	return new ComposerState(deps, options);
}

describe('ComposerState lifecycle', () => {
	let deps: ComposerDeps;
	beforeEach(() => (deps = makeDeps()));

	it('starts pristine', () => {
		expect(makeState(deps).status).toBe('pristine');
	});

	it('first flush creates content and lands in saved', async () => {
		const s = makeState(deps);
		s.markDirty();
		expect(s.status).toBe('dirty');
		await s.flush();
		expect(deps.create).toHaveBeenCalledTimes(1);
		expect(s.contentId).toBe('c1');
		expect(s.status).toBe('saved');
	});

	it('subsequent flush patches, never re-creates', async () => {
		const s = makeState(deps);
		s.markDirty();
		await s.flush();
		s.markDirty();
		await s.flush();
		expect(deps.create).toHaveBeenCalledTimes(1);
		expect(deps.patch).toHaveBeenCalledTimes(1);
	});

	it('single-flight: overlapping flushes create once', async () => {
		let resolve!: (v: unknown) => void;
		deps.create = vi.fn().mockReturnValue(new Promise((r) => (resolve = r)));
		const s = makeState(deps);
		s.markDirty();
		const a = s.flush();
		const b = s.flush(); // must not start a second create
		resolve(ok({ id: 'c1', slug: 's' }, 201));
		await Promise.all([a, b]);
		expect(deps.create).toHaveBeenCalledTimes(1);
	});
});

describe('ComposerState deferred linkage', () => {
	let deps: ComposerDeps;
	beforeEach(() => (deps = makeDeps()));

	it('holds tags chosen while pristine, flushes them after create', async () => {
		const s = makeState(deps);
		s.setTags(['t1', 't2']); // no content yet
		expect(deps.attachTag).not.toHaveBeenCalled();
		s.markDirty();
		await s.flush();
		expect(deps.attachTag).toHaveBeenCalledWith('c1', 't1');
		expect(deps.attachTag).toHaveBeenCalledWith('c1', 't2');
	});

	it('reconciles a series change after the draft exists (unlink old, link new)', async () => {
		const s = makeState(deps);
		s.markDirty();
		await s.flush();
		s.setSeries('sA');
		await Promise.resolve();
		s.setSeries('sB');
		await Promise.resolve();
		expect(deps.linkSeries).toHaveBeenCalledWith('sA', 'c1', 0);
		expect(deps.unlinkSeries).toHaveBeenCalledWith('sA', 'c1');
		expect(deps.linkSeries).toHaveBeenCalledWith('sB', 'c1', 0);
	});
});

describe('ComposerState publish gate + failure classes', () => {
	it('refuses to publish when canPublish() is false', async () => {
		const deps = makeDeps();
		const s = makeState(deps, { canPublish: () => false });
		s.markDirty();
		await s.flush();
		const okPublished = await s.publish();
		expect(okPublished).toBe(false);
		expect(deps.publish).not.toHaveBeenCalled();
	});

	it('publishes when valid; schedules when a publish_at is set', async () => {
		const deps = makeDeps();
		const s = makeState(deps, { getPublishAt: () => '2099-01-01T00:00:00Z' });
		s.markDirty();
		await s.flush();
		await s.publish();
		expect(deps.schedule).toHaveBeenCalledWith('c1', '2099-01-01T00:00:00Z');
		expect(deps.publish).not.toHaveBeenCalled();
		expect(s.status).toBe('scheduled');
	});

	it('a 422 create failure is validation → error, no retry', async () => {
		const deps = makeDeps({ create: vi.fn().mockResolvedValue(fail(422, 'bad title')) });
		const s = makeState(deps);
		s.markDirty();
		await s.flush();
		expect(s.status).toBe('error');
		expect(s.lastError?.cls).toBe('validation');
		expect(deps.create).toHaveBeenCalledTimes(1);
	});

	it('a 500 create failure is server → keeps buffer, schedules a retry', async () => {
		vi.useFakeTimers();
		const deps = makeDeps({ create: vi.fn().mockResolvedValue(fail(503)) });
		const s = makeState(deps, { debounceMs: 10 });
		s.markDirty();
		await s.flush();
		expect(s.lastError?.cls).toBe('server');
		expect(s.status).toBe('dirty'); // queued for retry, not error
		deps.create = vi.fn().mockResolvedValue(ok({ id: 'c1', slug: 's' }, 201));
		// #backoff() = 1000 * 2^retries; after 1st retry #retries=1 → 2000ms.
		await vi.advanceTimersByTimeAsync(2100);
		expect(s.contentId).toBe('c1');
		vi.useRealTimers();
	});
});
