import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/state', () => ({
	page: {
		params: { slug: 'test-thread' }
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getForumThread: vi.fn().mockResolvedValue(null),
		getForumReplies: vi.fn().mockResolvedValue({ replies: [] }),
		getForumPoll: vi.fn().mockResolvedValue(null),
		voteForum: vi.fn().mockResolvedValue({ score: 0 }),
		solveForumThread: vi.fn().mockResolvedValue({ solved: true }),
		createForumReply: vi.fn().mockResolvedValue({}),
		addForumBookmark: vi.fn().mockResolvedValue({}),
		removeForumBookmark: vi.fn().mockResolvedValue({}),
		voteForumPoll: vi.fn().mockResolvedValue({})
	}
}));

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
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

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Forum [slug] +page.svelte', () => {
	it('renders the Back to Forum link', () => {
		render(Page);
		expect(screen.getByText('Back to Forum')).toBeTruthy();
	});

	it('shows empty state when thread not found', async () => {
		render(Page);
		const emptyEl = await screen.findByText('Thread not found');
		expect(emptyEl).toBeTruthy();
	});

	it('shows thread not found message', async () => {
		render(Page);
		const msg = await screen.findByText('This thread may have been deleted or the link is invalid.');
		expect(msg).toBeTruthy();
	});
});
