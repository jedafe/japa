import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Custom Fields +page.svelte', () => {
	it('renders the Custom Fields heading', () => {
		render(Page);
		expect(screen.getByText('Custom Fields')).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Manage field definitions for entities')).toBeTruthy();
	});

	it('shows Create Field link', () => {
		render(Page);
		expect(screen.getByText('Create Field')).toBeTruthy();
	});

	it('shows empty state when no fields', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No custom fields defined')).toBeTruthy();
		});
	});
});
