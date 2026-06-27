import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('DB Health +page.svelte', () => {
	it('renders the db health heading', () => {
		render(Page);
		expect(screen.getByText('DB Health')).toBeTruthy();
	});

	it('shows the vacuum trigger button', () => {
		render(Page);
		expect(screen.getByText('Trigger VACUUM')).toBeTruthy();
	});

	it('shows slow query threshold input', () => {
		render(Page);
		expect(screen.getByText('Slow query threshold (ms)')).toBeTruthy();
	});

	it('shows table bloat section', () => {
		render(Page);
		expect(screen.getByText('Table Bloat')).toBeTruthy();
	});

	it('shows long-running queries section', () => {
		render(Page);
		expect(screen.getByText('Long-Running Queries')).toBeTruthy();
	});
});
