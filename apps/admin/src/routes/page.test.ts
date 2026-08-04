import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './(app)/+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected')),
	analyticsAdminApi: {
		getOverview: vi.fn().mockRejectedValue(new Error('not connected')),
		getUsers: vi.fn().mockRejectedValue(new Error('not connected')),
	}
}));

vi.mock('$app/state', () => ({
	page: {
		params: {},
		url: new URL('http://localhost/')
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('layerchart', () => ({
	LineChart: vi.fn().mockImplementation(() => null),
	AreaChart: vi.fn().mockImplementation(() => null)
}));

vi.mock('d3-shape', () => ({
	curveMonotoneX: vi.fn()
}));

describe('Admin +page.svelte', () => {
	it('renders the dashboard heading', () => {
		render(Page);
		expect(screen.getByText('Dashboard')).toBeTruthy();
	});

	it('shows key metric labels after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Total Users')).toBeTruthy();
		});
	});

	it('shows user growth section after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('User Growth')).toBeTruthy();
		});
	});

	it('shows content activity section after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Content Activity')).toBeTruthy();
		});
	});
});
