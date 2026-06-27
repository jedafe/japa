import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Users +page.svelte', () => {
	it('renders the users heading', () => {
		render(Page);
		expect(screen.getByText('Users')).toBeTruthy();
	});

	it('shows filter controls', () => {
		render(Page);
		expect(screen.getByText('Apply')).toBeTruthy();
	});

	it('has search input', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Search')).toBeTruthy();
	});
});
