import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'blog.title': 'Blog',
		'blog.subtitle': 'Stories, guides, and insights from the community.',
		'blog.no_posts': 'No posts yet',
		'blog.no_posts_msg': 'Check back later.',
		'blog.featured': 'Featured',
		'blog.recent_posts': 'Recent Posts',
		'blog.not_available': 'Blog is not available',
		'blog.min_read': 'min read',
		'blog.series': 'Series',
		'blog.view_all_series': 'View all',
		'blog.stay_updated': 'Stay updated',
		'blog.subscribe_msg': 'Subscribe',
		'blog.subscribe_rss': 'RSS',
		'forum.feature_disabled': 'This feature is currently disabled.',
		'forum.go_home': 'Go home'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('$lib/utils/timezone', () => ({
	formatTimestamp: () => 'Jan 15, 2026'
}));

vi.mock('@formkit/auto-animate', () => ({
	default: () => {}
}));

describe('Blog +page.svelte', () => {
	const defaultData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {},
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
		expect(screen.getAllByText('RSS').length).toBeGreaterThanOrEqual(1);
	});

	it('renders Featured section when featured posts exist', () => {
		const dataWithFeatured = {
			...defaultData,
			featured: [{ id: '1', featured_order: 1, content_id: 'c1', featured_until: '2026-12-31T00:00:00Z', created_at: '2026-01-01T00:00:00Z' }]
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
