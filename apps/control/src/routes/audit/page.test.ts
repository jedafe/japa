import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Audit +page.svelte', () => {
	it('renders the audit log heading', () => {
		render(Page);
		expect(screen.getByText('Audit Log')).toBeTruthy();
	});

	it('shows filter inputs', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Actor ID')).toBeTruthy();
		expect(screen.getByPlaceholderText('Action type')).toBeTruthy();
		expect(screen.getByPlaceholderText('Target type')).toBeTruthy();
	});

	it('has an apply filters button', () => {
		render(Page);
		expect(screen.getByText('Apply Filters')).toBeTruthy();
	});
});
