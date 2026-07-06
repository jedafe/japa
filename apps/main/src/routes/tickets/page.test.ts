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
	const defaultData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {},
		tickets: []
	};

	it('renders the My Tickets heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: /My Tickets/i })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Get help from our support team.')).toBeTruthy();
	});

	it('renders the New Ticket button', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('New Ticket')).toBeTruthy();
	});

	it('shows filter buttons', () => {
		render(Page, { props: { data: defaultData } });
		const filters = ['All', 'Open', 'In Progress', 'Resolved', 'Closed'];
		for (const label of filters) {
			expect(screen.getByText(label)).toBeTruthy();
		}
	});

	it('renders the page structure with main element', () => {
		const { container } = render(Page, { props: { data: defaultData } });
		expect(container.querySelector('main')).toBeTruthy();
	});
});
