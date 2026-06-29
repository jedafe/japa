import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockResolvedValue({ ok: true, data: {} })
}));

describe('Cache +page.svelte', () => {
	it('renders the cache management heading', () => {
		render(Page);
		expect(screen.getByText('Cache Management')).toBeTruthy();
	});

	it('shows the flush all caches action', () => {
		render(Page);
		const matches = screen.getAllByText('Flush All Caches');
		expect(matches.length).toBeGreaterThanOrEqual(1);
		expect(screen.getByText('Clear every cached layer including pages, feeds, i18n, and search results.')).toBeTruthy();
	});

	it('has a flush button', () => {
		render(Page);
		const flushButton = screen.getByText('Flush All Caches', { selector: 'button' });
		expect(flushButton).toBeTruthy();
	});
});
