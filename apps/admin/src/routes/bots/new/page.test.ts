import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Create Bot +page.svelte', () => {
	it('renders the Create Bot heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /create bot/i })).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Set up a new automated bot for the platform')).toBeTruthy();
	});

	it('shows form fields and Create Bot button', () => {
		render(Page);
		expect(screen.getByLabelText(/name/i)).toBeTruthy();
		expect(screen.getByLabelText('Avatar URL')).toBeTruthy();
		expect(screen.getByLabelText('Description')).toBeTruthy();
	});
});
