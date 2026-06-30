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
		subscribe: vi.fn((fn) => {
			fn({ params: { id: 'test-id' } });
			return () => {};
		})
	}
}));

describe('Edit Custom Field +page.svelte', () => {
	it('renders the Edit Custom Field heading', () => {
		render(Page);
		expect(screen.getByText('Edit Custom Field')).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Update field definition')).toBeTruthy();
	});

	it('shows loading state initially', () => {
		render(Page);
		expect(screen.getByText('Loading...')).toBeTruthy();
	});
});
