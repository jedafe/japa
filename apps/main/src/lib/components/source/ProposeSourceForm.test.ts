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

const meta: SourceMeta = {
	categories: ['news', 'job', 'blog'],
	source_types: ['rss', 'video', 'api', 'static'],
	source_kinds: [
		{ value: 'publisher', label: 'Publisher' },
		{ value: 'institution', label: 'Institution' }
	]
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

describe('ProposeSourceForm', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		mockApi();
	});

	it('renders name, url, category chips, source_kind select and submit — no source_type', () => {
		const { container, getByRole, queryByLabelText } = render(ProposeSourceForm, { props: { meta } });
		expect(container.querySelector('#psf-name')).toBeTruthy();
		expect(container.querySelector('#psf-source-url')).toBeTruthy();
		expect(container.querySelector('#psf-source-kind')).toBeTruthy();
		expect(container.querySelector('#psf-source-type')).toBeNull();
		expect(queryByLabelText(/source type/i)).toBeNull();
		expect(getByRole('button', { name: 'News' })).toBeTruthy();
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
		await fireEvent.click(getByRole('button', { name: 'News' }));
		await fireEvent.click(getByRole('button', { name: 'Jobs' }));
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
		await fireEvent.click(getByRole('button', { name: 'News' }));
		await fireEvent.click(getByRole('button', { name: /propose source/i }));

		await waitFor(() => {
			const body = postedBody('/sources/propose');
			expect(body.probe_snapshot).toMatchObject({ final_url: 'https://example.com/blog' });
		});
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
		await fireEvent.click(getByRole('button', { name: 'News' }));
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
		await fireEvent.click(getByRole('button', { name: 'News' }));
		await fireEvent.click(getByRole('button', { name: /propose source/i }));

		const alert = await findByRole('alert');
		expect(alert.textContent).toContain('already exists');
	});
});
