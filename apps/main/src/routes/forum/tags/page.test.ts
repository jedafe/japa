import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getForumTags: vi.fn().mockResolvedValue([])
	}
}));

describe('Forum Tags +page.svelte', () => {
	it('renders the Forum Tags heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Forum Tags' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Browse discussions by topic.')).toBeTruthy();
	});

	it('renders the Back to Forum link', () => {
		render(Page);
		expect(screen.getByText('Back to Forum')).toBeTruthy();
	});

	it('shows empty state when no tags exist', async () => {
		render(Page);
		const emptyEl = await screen.findByText('No tags yet');
		expect(emptyEl).toBeTruthy();
	});

	it('shows tag count message', async () => {
		render(Page);
		const msg = await screen.findByText('Tags will appear as threads are created.');
		expect(msg).toBeTruthy();
	});
});
