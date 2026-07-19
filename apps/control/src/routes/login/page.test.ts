import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected')),
	setToken: vi.fn()
}));

describe('Login +page.svelte', () => {
	it('renders the heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /JapaControl/i })).toBeTruthy();
	});

	it('shows sign-in subtitle', () => {
		render(Page);
		expect(screen.getByText('Sign in to the control panel')).toBeTruthy();
	});

	it('renders email and password inputs', () => {
		render(Page);
		expect(screen.getByLabelText('Email')).toBeTruthy();
		expect(screen.getByLabelText('Password')).toBeTruthy();
	});

	it('renders a sign-in button', () => {
		render(Page);
		expect(screen.getByRole('button', { name: /Sign In/i })).toBeTruthy();
	});
});
