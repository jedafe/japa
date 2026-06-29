import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$app/state', () => ({
	page: {
		url: {
			searchParams: {
				get: vi.fn().mockReturnValue(null)
			}
		}
	}
}));

describe('Search +page.svelte', () => {
	it('renders the Search heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Search' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Find communities, posts, and people on JapaConnect.')).toBeTruthy();
	});

	it('renders the search input', () => {
		render(Page);
		const input = screen.getByPlaceholderText('Search for topics, people, spaces...');
		expect(input).toBeTruthy();
	});

	it('shows the initial empty state prompt', () => {
		render(Page);
		expect(screen.getByText('Enter a search term to get started.')).toBeTruthy();
	});
});
