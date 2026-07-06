import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (fn: Function) => {
			fn({ params: { spaceId: 'test-space' } });
			return () => {};
		}
	}
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
