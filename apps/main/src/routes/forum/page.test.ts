import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getForumThreads: vi.fn().mockResolvedValue({ threads: [], total_pages: 0 }),
		getForumTags: vi.fn().mockResolvedValue([])
	}
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'forum.title': 'Forum',
		'forum.subtitle': 'Ask questions, share experiences, and help others.',
		'forum.new_thread': 'New Thread',
		'forum.all_threads': 'All Threads',
		'forum.open': 'Open',
		'forum.solved': 'Solved',
		'forum.no_threads': 'No threads yet',
		'forum.no_threads_msg': 'Be the first to start a discussion.',
		'forum.not_available': 'Forum is not available',
		'forum.feature_disabled': 'This feature is currently disabled.',
		'forum.go_home': 'Go home',
		'forum.votes': 'votes',
		'forum.replies': 'replies',
		'forum.page_of': 'Page {current} of {total}',
		'common.previous': 'Previous',
		'common.next': 'Next'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Forum +page.svelte', () => {
	it('renders the Forum heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Forum' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Ask questions, share experiences, and help others.')).toBeTruthy();
	});

	it('renders the New Thread button', () => {
		render(Page);
		expect(screen.getByText('New Thread')).toBeTruthy();
	});

	it('shows filter buttons', () => {
		render(Page);
		expect(screen.getByText('All Threads')).toBeTruthy();
		expect(screen.getByText('Open')).toBeTruthy();
		expect(screen.getByText('Solved')).toBeTruthy();
	});

	it('shows empty state when no threads exist', async () => {
		render(Page);
		const emptyEl = await screen.findByText('No threads yet');
		expect(emptyEl).toBeTruthy();
	});
});
