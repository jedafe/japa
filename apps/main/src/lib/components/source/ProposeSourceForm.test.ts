import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor, screen } from '@testing-library/svelte';
import ProposeSourceForm from './ProposeSourceForm.svelte';
import type { SourceMeta, ProbeReport } from '$lib/types/sources';

vi.mock('$app/state', () => ({
	page: {
		url: new URL('http://localhost/sources/submit'),
		data: {}
	}
}));

// $lib/auth/guest reads the page through $app/stores + get() — mock an
// authenticated viewer so requireAuthOrRedirect() passes in tests.
vi.mock('$app/stores', () => ({
	page: {
		subscribe: (fn: (v: unknown) => void) => {
			fn({ url: new URL('http://localhost/sources/submit'), data: { isAuthed: true } });
			return () => {};
		}
	}
}));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

const meta: SourceMeta = {
	categories: ['news', 'job', 'blog'],
	// The form's category picker prefers the submittable subset — only
	// domains with a deterministic (non-AI) content path are offered.
	submittable_categories: ['blog', 'job', 'news', 'video'],
	admin_categories: ['news', 'job', 'blog', 'grants', 'housing'],
	source_types: ['rss', 'video', 'api', 'static'],
	source_kinds: [
		{ value: 'publisher', label: 'Publication' },
		{ value: 'institution', label: 'Institution' }
	],
	// Propose form renders the submittable subset — entity-stub kinds are
	// excluded server-side.
	submittable_source_kinds: [{ value: 'publisher', label: 'Publication' }]
};

const probeReport: ProbeReport = {
	final_url: 'https://example.com/blog',
	detected_kinds: [],
	methods: [{ method: 'rss', confidence: 0.95, evidence: '<link rel="alternate">' }],
	channels: [
		{
			label: 'RSS feed',
			entry_url: 'https://example.com/feed.xml',
			crawl_method: 'rss',
			entity_types: ['news'],
			pipeline: 'curate',
			confidence: 0.95
		}
	],
	has_websub: false,
	requires_js: false,
	robots: { fetched: true, allowed: true },
	warnings: [],
	probed_at: '2026-09-21T00:00:00Z'
};

function jsonResponse(body: unknown, status = 200): Response {
	return {
		ok: status >= 200 && status < 300,
		status,
		statusText: 'OK',
		json: () => Promise.resolve(body),
		clone() {
			return this;
		}
	} as unknown as Response;
}

function mockApi(handler?: (url: string) => Response | undefined) {
	global.fetch = vi.fn((input: RequestInfo | URL) => {
		const url = String(input);
		const custom = handler?.(url);
		if (custom) return Promise.resolve(custom);
		if (url.includes('/sources/probe')) return Promise.resolve(jsonResponse({ data: probeReport }));
		if (url.includes('/sources/propose'))
			return Promise.resolve(
				jsonResponse({ data: { id: '1', slug: 's', status: 'proposed', admin_review_url: '' } }, 201)
			);
		return Promise.resolve(jsonResponse({ error: { code: 'not_found', message: 'not found' } }, 404));
	}) as unknown as typeof fetch;
}

function postedBody(urlPart: string): Record<string, unknown> {
	const calls = (global.fetch as ReturnType<typeof vi.fn>).mock.calls;
	const call = calls.find(([u]) => String(u).includes(urlPart));
	expect(call, `expected fetch to ${urlPart}`).toBeTruthy();
	return JSON.parse(String(call![1]?.body));
}

// pickCategory drives the multiselect autocomplete: focus the input, wait
// for the option list, mousedown the named option (options pick on
// mousedown so the input keeps focus).
async function pickCategory(container: HTMLElement, name: string | RegExp) {
	await fireEvent.focus(container.querySelector('#psf-categories')!);
	await fireEvent.mouseDown(await screen.findByRole('option', { name }));
}

describe('ProposeSourceForm', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		mockApi();
	});

	it('renders name, url, category picker, source_kind select and submit — no source_type', async () => {
		const { container, getByRole, queryByLabelText } = render(ProposeSourceForm, { props: { meta } });
		expect(container.querySelector('#psf-name')).toBeTruthy();
		expect(container.querySelector('#psf-source-url')).toBeTruthy();
		expect(container.querySelector('#psf-source-kind')).toBeTruthy();
		expect(container.querySelector('#psf-source-type')).toBeNull();
		expect(queryByLabelText(/source type/i)).toBeNull();
		expect(container.querySelector('#psf-categories')).toBeTruthy();
		// The picker lists submittable_categories only — 'grants'/'housing'
		// are valid taxonomy values but stay hidden until AI extraction is
		// configured.
		await fireEvent.focus(container.querySelector('#psf-categories')!);
		expect(await screen.findByRole('option', { name: 'News' })).toBeTruthy();
		expect(screen.queryByRole('option', { name: /grants/i })).toBeNull();
		expect(screen.queryByRole('option', { name: /housing/i })).toBeNull();
		expect(getByRole('button', { name: /propose source/i })).toBeTruthy();
	});

	it('requires at least one category before submitting', async () => {
		const { container, getByRole, findByRole } = render(ProposeSourceForm, { props: { meta } });
		await fireEvent.input(container.querySelector('#psf-name')!, { target: { value: 'Example' } });
		await fireEvent.input(container.querySelector('#psf-source-url')!, {
			target: { value: 'https://example.com' }
		});
		await fireEvent.click(getByRole('button', { name: /propose source/i }));
		expect(await findByRole('alert')).toBeTruthy();
		expect(global.fetch).not.toHaveBeenCalledWith(
			expect.stringContaining('/sources/propose'),
			expect.anything()
		);
	});

	it('submits categories[] + source_kind (no source_type) and shows success', async () => {
		const { container, getByRole, findByText } = render(ProposeSourceForm, { props: { meta } });
		await fireEvent.input(container.querySelector('#psf-name')!, { target: { value: 'Example' } });
		await fireEvent.input(container.querySelector('#psf-source-url')!, {
			target: { value: 'https://example.com' }
		});
		await pickCategory(container, 'News');
		await pickCategory(container, 'Jobs');
		await fireEvent.click(getByRole('button', { name: /propose source/i }));

		await waitFor(() => {
			const body = postedBody('/sources/propose');
			expect(body.categories).toEqual(['news', 'job']);
			expect(body.source_kind).toBe('publisher');
			expect(body).not.toHaveProperty('source_type');
			expect(body).not.toHaveProperty('category');
			expect(body).not.toHaveProperty('probe_snapshot');
		});
		expect(await findByText(/proposed successfully/i)).toBeTruthy();
	});

	it('probes the URL via Check URL, renders detected chips, and attaches probe_snapshot on submit', async () => {
		const { container, getByRole, findByText } = render(ProposeSourceForm, { props: { meta } });
		await fireEvent.input(container.querySelector('#psf-source-url')!, {
			target: { value: 'https://example.com/blog' }
		});
		await fireEvent.click(getByRole('button', { name: /check url/i }));

		expect(await findByText(/^detected$/i)).toBeTruthy();
		expect((await screen.findAllByText('RSS feed')).length).toBeGreaterThan(0);
		expect(await findByText(/proposed channels/i)).toBeTruthy();
		expect(postedBody('/sources/probe')).toMatchObject({ url: 'https://example.com/blog' });

		await fireEvent.input(container.querySelector('#psf-name')!, { target: { value: 'Example' } });
		await pickCategory(container, 'News');
		await fireEvent.click(getByRole('button', { name: /propose source/i }));

		await waitFor(() => {
			const body = postedBody('/sources/propose');
			expect(body.probe_snapshot).toMatchObject({ final_url: 'https://example.com/blog' });
		});
	});

	it('groups same-method proposed channels into one row with expandable endpoint URLs', async () => {
		const multiReport: ProbeReport = {
			...probeReport,
			channels: [
				{ label: 'RSS feed', entry_url: 'https://example.com/feed.xml', crawl_method: 'rss', pipeline: 'curate', confidence: 0.95 },
				{ label: 'RSS feed', entry_url: 'https://example.com/comments.xml', crawl_method: 'rss', pipeline: 'curate', confidence: 0.95 },
				{ label: 'Sitemap manifest', entry_url: 'https://example.com/a.xml', crawl_method: 'sitemap', pipeline: 'curate', confidence: 0.7 },
				{ label: 'Sitemap manifest', entry_url: 'https://example.com/b.xml', crawl_method: 'sitemap', pipeline: 'curate', confidence: 0.7 },
				{ label: 'Sitemap manifest', entry_url: 'https://example.com/c.xml', crawl_method: 'sitemap', pipeline: 'curate', confidence: 0.7 }
			]
		};
		mockApi((url) => {
			if (url.includes('/sources/probe')) return jsonResponse({ data: multiReport });
			return undefined;
		});

		const { container, getByRole, findByText } = render(ProposeSourceForm, { props: { meta } });
		await fireEvent.input(container.querySelector('#psf-source-url')!, {
			target: { value: 'https://example.com/blog' }
		});
		await fireEvent.click(getByRole('button', { name: /check url/i }));

		// Each method+label collapses to a single row labelled with an
		// endpoint count; the URLs themselves are inside the expandable.
		expect(await findByText(/^proposed channels$/i)).toBeTruthy();
		expect(screen.getByText('2 endpoints')).toBeTruthy();
		expect(screen.getByText('3 endpoints')).toBeTruthy();
		expect(screen.getAllByText('Sitemap manifest').length).toBe(1);
		expect(screen.getByText('https://example.com/a.xml')).toBeTruthy();
		expect(screen.getByText('https://example.com/c.xml')).toBeTruthy();
	});

	it('degrades gracefully when the probe fails — form still submits without a snapshot', async () => {
		mockApi((url) => {
			if (url.includes('/sources/probe'))
				return jsonResponse({ error: { code: 'probe_quota_exceeded', message: 'limit reached' } }, 429);
			return undefined;
		});

		const { container, getByRole, findByText } = render(ProposeSourceForm, { props: { meta } });
		await fireEvent.input(container.querySelector('#psf-source-url')!, {
			target: { value: 'https://example.com' }
		});
		await fireEvent.click(getByRole('button', { name: /check url/i }));
		expect(await findByText(/limit reached/i)).toBeTruthy();

		await fireEvent.input(container.querySelector('#psf-name')!, { target: { value: 'Example' } });
		await pickCategory(container, 'News');
		await fireEvent.click(getByRole('button', { name: /propose source/i }));

		await waitFor(() => {
			const body = postedBody('/sources/propose');
			expect(body).not.toHaveProperty('probe_snapshot');
		});
		expect(await findByText(/proposed successfully/i)).toBeTruthy();
	});

	it('maps duplicate_source (409) to a friendly error', async () => {
		mockApi((url) => {
			if (url.includes('/sources/propose'))
				return jsonResponse(
					{ error: { code: 'duplicate_source', message: 'A source with this URL already exists' } },
					409
				);
			return undefined;
		});

		const { container, getByRole, findByRole } = render(ProposeSourceForm, { props: { meta } });
		await fireEvent.input(container.querySelector('#psf-name')!, { target: { value: 'Example' } });
		await fireEvent.input(container.querySelector('#psf-source-url')!, {
			target: { value: 'https://example.com' }
		});
		await pickCategory(container, 'News');
		await fireEvent.click(getByRole('button', { name: /propose source/i }));

		const alert = await findByRole('alert');
		expect(alert.textContent).toContain('already exists');
	});
});
