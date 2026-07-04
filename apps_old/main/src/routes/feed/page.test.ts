import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getFeedPrimary: vi.fn().mockResolvedValue({ items: [], hasMore: false })
	}
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'feed.title': 'Feed',
		'feed.subtitle': 'Stay updated with the latest from your community.',
		'feed.empty': 'Your feed is empty',
		'feed.empty_hint': 'Follow spaces and people to see their posts here.',
		'feed.refresh': 'Refresh',
		'feed.load_more': 'Load more',
		'feed.loading': 'Loading more...'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/utils/timezone', () => ({
	formatTimestamp: () => 'Jan 15, 2026',
	formatRelativeTime: () => '2 hours ago'
}));

vi.mock('@formkit/auto-animate', () => ({
	default: () => {}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

beforeEach(() => {
	vi.stubGlobal(
		'IntersectionObserver',
		class {
			observe() {}
			unobserve() {}
			disconnect() {}
		}
	);
});

const defaultData = {
	user: null,
	flags: {},
	notificationCount: 0,
	token: null,
	locale: 'en',
	translations: {},
	feed: [],
	hasMore: false
};

describe('Feed +page.svelte', () => {
	it('renders the Feed heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Feed' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Stay updated with the latest from your community.')).toBeTruthy();
	});

	it('shows empty state when feed is empty', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('Your feed is empty')).toBeTruthy();
		});
	});
});
