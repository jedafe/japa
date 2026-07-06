import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		createForumThread: vi.fn().mockResolvedValue({ slug: 'test-thread' }),
		listSpaces: vi.fn().mockResolvedValue([])
	}
}));

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

describe('Forum New Thread +page.svelte', () => {
	it('renders the New Thread heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'New Thread' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Ask a question or start a discussion.')).toBeTruthy();
	});

	it('renders the Back to Forum link', () => {
		render(Page);
		expect(screen.getByText('Back to Forum')).toBeTruthy();
	});

	it('renders the title input field', () => {
		render(Page);
		expect(screen.getByPlaceholderText("What's your question or topic?")).toBeTruthy();
	});

	it('renders the body textarea', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Describe your question or topic in detail...')).toBeTruthy();
	});

	it('renders the Post Thread submit button', () => {
		render(Page);
		expect(screen.getByText('Post Thread')).toBeTruthy();
	});

	it('renders the Post anonymously checkbox', () => {
		render(Page);
		expect(screen.getByText('Post anonymously')).toBeTruthy();
	});
});
