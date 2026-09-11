import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Tenants +page.svelte', () => {
	it('renders the tenants heading', () => {
		render(Page);
		expect(screen.getByText('Tenants')).toBeTruthy();
	});

	it('shows the create tenant button', () => {
		render(Page);
		expect(screen.getByText(/Create Tenant/)).toBeTruthy();
	});
});
