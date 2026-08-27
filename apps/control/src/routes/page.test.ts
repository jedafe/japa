import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('layerchart', () => ({
	PieChart: {},
	BarChart: {},
	LineChart: {}
}));

describe('Control +page.svelte', () => {
	it('renders the dashboard heading', () => {
		render(Page);
		expect(screen.getByText('Dashboard')).toBeTruthy();
	});

	it('shows health groups', () => {
		render(Page);
		expect(screen.getByText('Infrastructure')).toBeTruthy();
		expect(screen.getByText('Application')).toBeTruthy();
		expect(screen.getByText('Background Processing')).toBeTruthy();
		expect(screen.getByText('Real-time')).toBeTruthy();
	});

	it('has a refresh button', () => {
		render(Page);
		expect(screen.getByText('Refresh')).toBeTruthy();
	});
});
