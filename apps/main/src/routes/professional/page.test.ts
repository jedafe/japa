import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Professional Profiles +page.svelte', () => {
	const defaultData = {
		profiles: [],
		filters: { search: '' },
		total: 0,
		totalPages: 1
	};

	it('renders the Professional Profiles heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Professional Profiles' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Discover professionals ready for their next chapter abroad.')).toBeTruthy();
	});

	it('renders the Create profile button', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Create profile')).toBeTruthy();
	});

	it('renders the search input', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByPlaceholderText('Search by name or skill...')).toBeTruthy();
	});

	it('shows empty state when no profiles exist', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('No professional profiles yet')).toBeTruthy();
	});
});
