import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Control +page.svelte', () => {
	it('renders the dashboard heading', () => {
		render(Page);
		expect(screen.getByText('Dashboard')).toBeTruthy();
	});

	it('shows metric cards', () => {
		render(Page);
		expect(screen.getByText('Throughput')).toBeTruthy();
		expect(screen.getByText('Latency p99')).toBeTruthy();
		expect(screen.getByText('Error Rate')).toBeTruthy();
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
