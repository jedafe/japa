import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getTicket: vi.fn().mockRejectedValue(new Error('Not connected')),
		addTicketReply: vi.fn().mockRejectedValue(new Error('Not connected'))
	}
}));

vi.mock('$app/state', () => ({
	page: {
		params: { id: 'ticket-123' }
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Ticket Detail +page.svelte', () => {
	it('renders the back to tickets link', () => {
		render(Page);
		expect(screen.getByText('Back to Tickets')).toBeTruthy();
	});

	it('shows loading skeleton initially', () => {
		const { container } = render(Page);
		expect(container.querySelector('.animate-pulse')).toBeTruthy();
	});

	it('shows error state after failed load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Not connected')).toBeTruthy();
		});
	});

	it('has a link back to tickets list', () => {
		render(Page);
		const backLink = screen.getByRole('link');
		expect(backLink.getAttribute('href')).toBe('/tickets');
	});
});
