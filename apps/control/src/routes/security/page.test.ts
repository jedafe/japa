import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Security +page.svelte', () => {
	it('renders the security heading', () => {
		render(Page);
		expect(screen.getByText('Security')).toBeTruthy();
	});

	it('shows scan findings tab', () => {
		render(Page);
		expect(screen.getByRole('tab', { name: 'Scan Findings' })).toBeTruthy();
	});

	it('shows IP blocks tab', () => {
		render(Page);
		expect(screen.getByRole('tab', { name: /IP Blocks/ })).toBeTruthy();
	});
});
