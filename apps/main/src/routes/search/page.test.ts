import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		search: vi.fn().mockResolvedValue([])
	}
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

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'search.results': 'Search',
		'search.find': 'Find communities, posts, and people on JapaConnect.',
		'search.placeholder_short': 'Search for topics, people, spaces...',
		'search.no_results': 'No results found',
		'search.no_results_hint': 'Try a different search term.',
		'search.enter_term': 'Enter a search term to get started.'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/analytics', () => ({
	trackEvent: vi.fn(),
	AnalyticsEvents: { SEARCH: 'search' }
}));

vi.mock('@formkit/auto-animate', () => ({
	default: () => {}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

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
