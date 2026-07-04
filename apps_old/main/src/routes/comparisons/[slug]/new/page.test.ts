import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((fn) => {
			fn({ params: { slug: 'countries' } });
			return { unsubscribe: vi.fn() };
		})
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Create Comparison +page.svelte', () => {
	it('renders the Create a comparison heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Create a comparison/i })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Add items to compare side by side.')).toBeTruthy();
	});

	it('renders the back to comparisons link', () => {
		render(Page);
		expect(screen.getByText('Back to comparisons')).toBeTruthy();
	});

	it('renders the form fields', () => {
		render(Page);
		expect(screen.getByLabelText('Title')).toBeTruthy();
		expect(screen.getByText('Items to compare')).toBeTruthy();
	});

	it('renders the Create comparison button', () => {
		render(Page);
		expect(screen.getByText('Create comparison')).toBeTruthy();
	});
});
