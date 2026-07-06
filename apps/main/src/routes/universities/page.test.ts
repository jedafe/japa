import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

const baseData = {
	user: null,
	flags: {},
	notificationCount: 0,
	token: null,
	locale: 'en',
	translations: {}
};

describe('Universities +page.svelte', () => {
	it('renders the heading', () => {
		render(Page, { props: { data: { ...baseData, universities: [], filters: { page: '1', country: '', sort: 'world_ranking' } } } });
		const headings = screen.getAllByRole('heading', { name: /Universities/i });
		expect(headings.length).toBeGreaterThanOrEqual(1);
	});

	it('renders the subtitle text', () => {
		render(Page, { props: { data: { ...baseData, universities: [], filters: { page: '1', country: '', sort: 'world_ranking' } } } });
		expect(screen.getByText('Find the right university for your study abroad journey.')).toBeTruthy();
	});

	it('shows empty state when no universities', () => {
		render(Page, { props: { data: { ...baseData, universities: [], filters: { page: '1', country: '', sort: 'world_ranking' } } } });
		expect(screen.getByText('No universities found')).toBeTruthy();
	});
});
