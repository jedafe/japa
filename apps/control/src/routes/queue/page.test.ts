import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockResolvedValue({ ok: true, data: {} })
}));

describe('Queue +page.svelte', () => {
	it('renders the queue monitor heading', () => {
		render(Page);
		expect(screen.getByText('Queue Monitor')).toBeTruthy();
	});

	it('shows the auto-refresh indicator', () => {
		render(Page);
		expect(screen.getByText('Auto-refresh every 30s')).toBeTruthy();
	});

	it('shows the refresh button', () => {
		render(Page);
		expect(screen.getByText('Refresh')).toBeTruthy();
	});
});
