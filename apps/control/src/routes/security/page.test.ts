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

	it('shows severity filter', () => {
		render(Page);
		expect(screen.getByText('Filter severity:')).toBeTruthy();
	});
});
