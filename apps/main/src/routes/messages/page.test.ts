import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		listDMConversations: vi.fn().mockRejectedValue(new Error('Not connected'))
	}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Messages +page.svelte', () => {
	it('renders the Messages heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Messages/i })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Private conversations with other members.')).toBeTruthy();
	});

	it('renders the page structure with main element', () => {
		const { container } = render(Page);
		expect(container.querySelector('main')).toBeTruthy();
	});
});
