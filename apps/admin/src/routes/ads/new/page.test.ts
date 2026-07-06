import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Create Campaign +page.svelte', () => {
	it('renders the heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Create Campaign/ })).toBeTruthy();
	});

	it('renders form sections', () => {
		render(Page);
		expect(screen.getByText('Campaign')).toBeTruthy();
		expect(screen.getByText('Creative')).toBeTruthy();
		expect(screen.getByText('Budget & Bidding')).toBeTruthy();
		expect(screen.getByText('Targeting')).toBeTruthy();
		expect(screen.getByText('Schedule')).toBeTruthy();
	});

	it('renders the create button', () => {
		render(Page);
		expect(screen.getByText('Create Campaign', { selector: 'button' })).toBeTruthy();
	});
});
