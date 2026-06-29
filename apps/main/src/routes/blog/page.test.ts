import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Blog +page.svelte', () => {
	const defaultData = {
		featured: [],
		posts: [],
		series: []
	};

	it('renders the Blog heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Blog' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Stories, guides, and insights from the community.')).toBeTruthy();
	});

	it('shows empty state when no posts exist', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('No posts yet')).toBeTruthy();
	});

	it('renders RSS link', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('RSS')).toBeTruthy();
	});

	it('renders Featured section when featured posts exist', () => {
		const dataWithFeatured = {
			...defaultData,
			featured: [{ id: '1', featured_order: 1, content_id: 'c1' }]
		};
		render(Page, { props: { data: dataWithFeatured } });
		const featuredTexts = screen.getAllByText('Featured');
		expect(featuredTexts.length).toBeGreaterThanOrEqual(2);
	});

	it('renders Recent Posts section heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Recent Posts')).toBeTruthy();
	});
});
