import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Blog Series [slug] +page.svelte', () => {
	const defaultData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {},
		series: {
			id: '1',
			title: 'Test Series',
			slug: 'test-series',
			description: 'Series description',
			status: 'published',
			created_at: '2026-01-01T00:00:00Z'
		},
		posts: []
	};

	it('renders the series title', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Test Series' })).toBeTruthy();
	});

	it('renders the Back to series link', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText(/Back to series/)).toBeTruthy();
	});

	it('shows empty state when no posts in series', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('No posts in this series yet')).toBeTruthy();
	});

	it('renders series description', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Series description')).toBeTruthy();
	});

	it('renders Series label', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Series')).toBeTruthy();
	});

	it('renders numbered posts when posts exist', () => {
		const dataWithPosts = {
			...defaultData,
			posts: [
				{ series_id: '1', content_id: 'c1', position: 1, content_id_2: 'c1', slug: 'post-1', title: 'First Post', created_at: '2026-01-01T00:00:00Z', status: 'published' },
				{ series_id: '1', content_id: 'c2', position: 2, content_id_2: 'c2', slug: 'post-2', title: 'Second Post', created_at: '2026-01-02T00:00:00Z', status: 'published' }
			]
		};
		render(Page, { props: { data: dataWithPosts } });
		expect(screen.getByText('First Post')).toBeTruthy();
		expect(screen.getByText('Second Post')).toBeTruthy();
	});
});
