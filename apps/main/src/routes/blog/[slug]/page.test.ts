import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Blog [slug] +page.svelte', () => {
	const defaultData = {
		post: {
			id: '1',
			title: 'Test Post Title',
			body_html: '<p>Test content</p>',
			published_at: '2026-01-15T10:00:00Z',
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
