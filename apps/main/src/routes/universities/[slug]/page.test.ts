import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/state', () => ({
	page: {
		params: { slug: 'test-university' }
	}
}));

const mockData = {
	university: {
		id: '1',
		name: 'University of Lagos',
		description: 'A top university in Nigeria.',
		country: 'Nigeria',
		logo_url: null,
		slug: 'university-of-lagos'
	},
	reviewAggregate: { average_rating: 4.5, review_count: 120 },
	fields: []
};

describe('UniversityDetail +page.svelte', () => {
	it('renders the follow button', () => {
		render(Page, { props: { data: mockData } });
		expect(screen.getByText('Follow')).toBeTruthy();
	});

	it('renders the Follow/Unfollow toggle', () => {
		render(Page, { props: { data: mockData } });
		expect(screen.getByRole('button', { name: /Follow/i })).toBeTruthy();
	});

	it('renders the overview tab content', () => {
		render(Page, { props: { data: mockData } });
		expect(screen.getByText('A top university in Nigeria.')).toBeTruthy();
	});
});
