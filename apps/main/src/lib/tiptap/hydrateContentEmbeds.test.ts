import { describe, it, expect, vi, beforeEach } from 'vitest';
import { waitFor } from '@testing-library/svelte';
import { hydrateContentEmbeds } from './hydrateContentEmbeds';

// ContentEmbedCard → $lib/i18n reads page.data.locale via $app/state.
vi.mock('$app/state', () => ({
	page: { url: new URL('http://localhost/list/test'), data: {} }
}));

function makeContainer(inner: string): HTMLElement {
	const el = document.createElement('div');
	el.innerHTML = inner;
	document.body.appendChild(el);
	return el;
}

function okFetch(payload: unknown) {
	const body = { data: payload };
	return vi.fn().mockResolvedValue({
		ok: true,
		// apiFetch sniffs content-type then calls text() — the fake must
		// carry both.
		headers: new Headers({ 'content-type': 'application/json' }),
		json: () => Promise.resolve(body),
		text: () => Promise.resolve(JSON.stringify(body))
	}) as unknown as typeof fetch;
}

describe('hydrateContentEmbeds', () => {
	beforeEach(() => {
		document.body.innerHTML = '';
		vi.restoreAllMocks();
		global.fetch = vi.fn() as unknown as typeof fetch;
	});

	it('hydrates a content-embed placeholder into a live card', async () => {
		global.fetch = okFetch({
			slug: 'hello-world',
			title: 'Hello World',
			content_type: 'blog_post',
			excerpt: 'An excerpt'
		});
		const host = makeContainer(
			'<div data-type="content-embed" data-content-slug="hello-world"><a class="ce-card" href="/content/hello-world"><span class="ce-title">Snap</span></a></div>'
		);
		const action = hydrateContentEmbeds(host);

		const placeholder = host.querySelector('[data-type="content-embed"]')!;
		expect(placeholder.getAttribute('data-hydrated')).toBe('true');

		await waitFor(() => {
			expect(host.querySelector('a[href="/blog/hello-world"]')).toBeTruthy();
		});
		expect(host.textContent).toContain('Hello World');
		expect(host.textContent).toContain('An excerpt');
		// The static snapshot was replaced by the mounted card.
		expect(host.querySelectorAll('.ce-title').length).toBe(1);

		action.destroy();
	});

	it('keeps the serialized snapshot when the fetch fails', async () => {
		global.fetch = vi.fn().mockResolvedValue({
			ok: false,
			status: 404,
			headers: new Headers({ 'content-type': 'application/json' }),
			json: () => Promise.resolve({ error: { code: 'not_found' } }),
			text: () => Promise.resolve(JSON.stringify({ error: { code: 'not_found' } }))
		}) as unknown as typeof fetch;
		const host = makeContainer(
			'<div data-type="content-embed" data-content-slug="gone"><a class="ce-card" href="/content/gone"><img src="/x.png"><span class="ce-title">Snap Title</span><span class="ce-excerpt">Snap excerpt</span></a></div>'
		);
		const action = hydrateContentEmbeds(host);

		await waitFor(() => {
			expect(host.textContent).toContain('Snap Title');
		});
		expect(host.textContent).toContain('Snap excerpt');
		expect(host.querySelector('img[src="/x.png"]')).toBeTruthy();

		action.destroy();
	});

	it('skips placeholders without a slug and already-hydrated nodes', () => {
		global.fetch = okFetch({ slug: 'x', title: 'X', content_type: 'list' });
		const host = makeContainer(
			'<div data-type="content-embed"></div>' +
				'<div data-type="content-embed" data-content-slug="y" data-hydrated="true"></div>'
		);
		const action = hydrateContentEmbeds(host);

		expect(global.fetch).not.toHaveBeenCalled();
		expect(host.querySelectorAll('[data-hydrated]').length).toBe(1);

		action.destroy();
	});

	it('hydrates placeholders added after mount via MutationObserver', async () => {
		global.fetch = okFetch({ slug: 'later', title: 'Later Item', content_type: 'list' });
		const host = makeContainer('<p>intro</p>');
		const action = hydrateContentEmbeds(host);

		const added = document.createElement('div');
		added.setAttribute('data-type', 'content-embed');
		added.setAttribute('data-content-slug', 'later');
		host.appendChild(added);

		await waitFor(() => {
			expect(added.getAttribute('data-hydrated')).toBe('true');
		});
		await waitFor(() => {
			expect(host.textContent).toContain('Later Item');
		});

		action.destroy();
	});

	it('destroy() unmounts cards and disconnects the observer', async () => {
		global.fetch = okFetch({ slug: 'a', title: 'A', content_type: 'list' });
		const host = makeContainer(
			'<div data-type="content-embed" data-content-slug="a"></div>'
		);
		const action = hydrateContentEmbeds(host);

		await waitFor(() => {
			expect(host.querySelector('.content-embed-card')).toBeTruthy();
		});
		action.destroy();
		expect(host.querySelector('.content-embed-card')).toBeFalsy();

		// Observer disconnected: new placeholders are no longer hydrated.
		const added = document.createElement('div');
		added.setAttribute('data-type', 'content-embed');
		added.setAttribute('data-content-slug', 'b');
		host.appendChild(added);
		await Promise.resolve();
		expect(added.hasAttribute('data-hydrated')).toBe(false);
	});
});
