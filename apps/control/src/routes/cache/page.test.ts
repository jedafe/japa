import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Cache +page.svelte', () => {
	it('renders the cache management heading', () => {
		render(Page);
		expect(screen.getByText('Cache Management')).toBeTruthy();
	});

	it('shows all cache target categories', () => {
		render(Page);
		expect(screen.getByText('Flush All Caches')).toBeTruthy();
		expect(screen.getByText('Page Cache')).toBeTruthy();
		expect(screen.getByText('Feed Cache')).toBeTruthy();
		expect(screen.getByText('i18n Cache')).toBeTruthy();
		expect(screen.getByText('Search Cache')).toBeTruthy();
	});

	it('has flush buttons for each target', () => {
		render(Page);
		const flushButtons = screen.getAllByText('Flush');
		expect(flushButtons.length).toBe(5);
	});
});
