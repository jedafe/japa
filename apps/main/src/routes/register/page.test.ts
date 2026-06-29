import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Register +page.svelte', () => {
	it('renders the Create an account heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Create an account' })).toBeTruthy();
	});

	it('shows the join subtitle', () => {
		render(Page);
		expect(screen.getByText('Join the JapaConnect community')).toBeTruthy();
	});

	it('renders the username input', () => {
		render(Page);
		const usernameInput = screen.getByPlaceholderText('johndoe');
		expect(usernameInput).toBeTruthy();
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

	it('renders the Create account submit button', () => {
		render(Page);
		expect(screen.getByRole('button', { name: /Create account/ })).toBeTruthy();
	});

	it('shows the sign-in link', () => {
		render(Page);
		expect(screen.getByText('Already have an account?')).toBeTruthy();
		expect(screen.getByText('Sign in')).toBeTruthy();
	});
});
