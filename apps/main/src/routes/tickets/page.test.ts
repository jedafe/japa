import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		listTickets: vi.fn().mockRejectedValue(new Error('Not connected'))
	}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Tickets +page.svelte', () => {
	it('renders the My Tickets heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /My Tickets/i })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Get help from our support team.')).toBeTruthy();
	});

	it('renders the New Ticket button', () => {
		render(Page);
		expect(screen.getByText('New Ticket')).toBeTruthy();
	});

	it('shows filter buttons', () => {
		render(Page);
		const filters = ['All', 'Open', 'In Progress', 'Resolved', 'Closed'];
		for (const label of filters) {
			expect(screen.getByText(label)).toBeTruthy();
		}
	});

	it('renders the page structure with main element', () => {
		const { container } = render(Page);
		expect(container.querySelector('main')).toBeTruthy();
	});
});
