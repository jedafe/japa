import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockResolvedValue({ ok: true, data: {} })
}));

vi.mock('$lib/stores/i18n.svelte', () => ({
	t: vi.fn((key: string, fallback?: string) => fallback ?? key)
}));

describe('Cache +page.svelte', () => {
	it('renders the cache management heading', () => {
		render(Page);
		expect(screen.getByText('cache.title')).toBeTruthy();
	});

	it('shows the flush all caches action', () => {
		render(Page);
		const matches = screen.getAllByText('cache.flush_all');
		expect(matches.length).toBeGreaterThanOrEqual(1);
		expect(screen.getByText('cache.flush_description')).toBeTruthy();
	});

	it('has a flush button', () => {
		render(Page);
		const flushButton = screen.getByText('cache.flush_all', { selector: 'button' });
		expect(flushButton).toBeTruthy();
	});
});
