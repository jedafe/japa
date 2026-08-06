import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	replaceState: vi.fn(),
}));

describe('Create Bot +page.svelte', () => {
	it('renders the Create Bot heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /create bot/i })).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Manage chatbots, moderation bots, and automated agents')).toBeTruthy();
	});

	it('shows form fields and Create Bot button', () => {
		render(Page);
		expect(screen.getByText(/name/i)).toBeTruthy();
		expect(screen.getByText(/user-agent pattern/i)).toBeTruthy();
	});
});
