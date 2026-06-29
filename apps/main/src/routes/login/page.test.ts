import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Login +page.svelte', () => {
	it('renders the Sign in heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Sign in' })).toBeTruthy();
	});

	it('shows the welcome subtitle', () => {
		render(Page);
		expect(screen.getByText('Welcome back to JapaConnect')).toBeTruthy();
	});

	it('renders the email input', () => {
		render(Page);
		const emailInput = screen.getByPlaceholderText('you@example.com');
		expect(emailInput).toBeTruthy();
	});

	it('renders the password input', () => {
		render(Page);
		const passwordInput = screen.getByPlaceholderText('••••••••');
		expect(passwordInput).toBeTruthy();
	});

	it('renders the Sign in submit button', () => {
		render(Page);
		expect(screen.getByRole('button', { name: /Sign in/ })).toBeTruthy();
	});

	it('shows the register link', () => {
		render(Page);
		expect(screen.getByText("Don't have an account?")).toBeTruthy();
		expect(screen.getByText('Create one')).toBeTruthy();
	});
});
