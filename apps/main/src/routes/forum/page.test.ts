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
