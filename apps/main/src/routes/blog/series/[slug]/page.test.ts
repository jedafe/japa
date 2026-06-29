import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Blog Series [slug] +page.svelte', () => {
	const defaultData = {
		series: {
			id: '1',
			title: 'Test Series',
			slug: 'test-series',
			description: 'Series description',
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
				{ content_id: 'c1', slug: 'post-1', title: 'First Post', created_at: '2026-01-01T00:00:00Z', status: 'published' },
				{ content_id: 'c2', slug: 'post-2', title: 'Second Post', created_at: '2026-01-02T00:00:00Z', status: 'published' }
			]
		};
		render(Page, { props: { data: dataWithPosts } });
		expect(screen.getByText('First Post')).toBeTruthy();
		expect(screen.getByText('Second Post')).toBeTruthy();
	});
});
