import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		addReaction: vi.fn().mockResolvedValue({})
	}
}));

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('$lib/utils/timezone', () => ({
	formatTimestampLong: () => 'January 15, 2026'
}));

vi.mock('$lib/components/ReactionBar.svelte', () => ({
	default: () => null
}));

vi.mock('$lib/components/RevisionHistory.svelte', () => ({
	default: () => null
}));

vi.mock('$lib/components/PinButton.svelte', () => ({
	default: () => null
}));

describe('Blog [slug] +page.svelte', () => {
	const defaultData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {},
		post: {
			id: '1',
			title: 'Test Post Title',
			slug: 'test-post',
			body: 'Test content',
			body_html: '<p>Test content</p>',
			content_type: 'blog_post',
			status: 'published',
			published_at: '2026-01-15T10:00:00Z',
			created_at: '2026-01-15T10:00:00Z',
			author_id: 'author-1',
			field_values: '{}',
			read_time_minutes: 5,
			view_count: 42,
			comment_count: 3,
			share_count: 1
		}
	};

	it('renders the post title', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Test Post Title' })).toBeTruthy();
	});

	it('renders the Back to blog link', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Back to blog')).toBeTruthy();
	});

	it('renders Upvote and Downvote buttons', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Upvote')).toBeTruthy();
		expect(screen.getByText('Downvote')).toBeTruthy();
	});

	it('renders post body content', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Test content')).toBeTruthy();
	});

	it('displays view count', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('42 views')).toBeTruthy();
	});

	it('displays comment count', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('3 comments')).toBeTruthy();
	});

	it('displays share count', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('1 shares')).toBeTruthy();
	});
});
