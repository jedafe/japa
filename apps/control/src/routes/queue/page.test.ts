import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockResolvedValue({ ok: true, data: {} })
}));

describe('Queue +page.svelte', () => {
	it('renders the queue monitor heading', () => {
		render(Page);
		// In test env, i18n returns the key since translations are not loaded
		expect(screen.getByText('queue.title')).toBeTruthy();
	});

	it('shows the auto-refresh indicator', () => {
		render(Page);
		expect(screen.getByText('queue.auto_refresh')).toBeTruthy();
	});

	it('shows the refresh button', () => {
		render(Page);
		expect(screen.getByText('common.refresh')).toBeTruthy();
	});
});
