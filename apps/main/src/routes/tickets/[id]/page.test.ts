import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
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
	const defaultData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {},
		ticket: null,
		comments: []
	};

	it('renders the back to tickets link', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Back to Tickets')).toBeTruthy();
	});

	it('shows empty state when ticket is null', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Ticket not found')).toBeTruthy();
	});

	it('shows empty state message when ticket is null', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('This ticket may have been deleted or the link is invalid.')).toBeTruthy();
	});

	it('has a link back to tickets list', () => {
		render(Page, { props: { data: defaultData } });
		const backLink = screen.getByRole('link');
		expect(backLink.getAttribute('href')).toBe('/tickets');
	});
});
