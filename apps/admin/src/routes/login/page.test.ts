import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected')),
	setToken: vi.fn()
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Login +page.svelte', () => {
	it('renders the JapaConnect Admin heading', () => {
		render(Page);
		expect(screen.getByText('JapaConnect Admin')).toBeTruthy();
	});

	it('renders the sign-in subtitle', () => {
		render(Page);
		expect(screen.getByText('Content Moderation & Management')).toBeTruthy();
	});

	it('shows email and password inputs', () => {
		render(Page);
		expect(screen.getByLabelText('Email')).toBeTruthy();
		expect(screen.getByLabelText('Password')).toBeTruthy();
	});
});
