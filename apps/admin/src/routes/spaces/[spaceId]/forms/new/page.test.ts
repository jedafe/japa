import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('#lib/api.js', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	replaceState: vi.fn(),
}));

vi.mock('$app/state', () => ({
	page: { params: { spaceId: 'test-space' } }
}));

describe('Create Form +page.svelte', () => {
	it('renders the heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Create Form/ })).toBeTruthy();
	});

	it('renders form builder sections', () => {
		render(Page);
		expect(screen.getByText('Form Details')).toBeTruthy();
		expect(screen.getByText('Fields')).toBeTruthy();
	});

	it('shows Add Field button', () => {
		render(Page);
		expect(screen.getByText('+ Add Field')).toBeTruthy();
	});
});
