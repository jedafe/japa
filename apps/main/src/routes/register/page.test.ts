import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'auth.create_account': 'Create an account',
		'auth.join_community': 'Join the JapaConnect community',
		'auth.username_label': 'Username',
		'auth.email_label': 'Email',
		'auth.password_label': 'Password',
		'auth.or_sign_up_with': 'Or sign up with',
		'auth.sign_up_google': 'Sign up with Google',
		'auth.sign_up_github': 'Sign up with GitHub',
		'auth.has_account': 'Already have an account?',
		'auth.sign_in': 'Sign in'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/analytics', () => ({
	trackEvent: vi.fn(),
	AnalyticsEvents: { REGISTER: 'register' }
}));

vi.mock('$lib/api', () => ({
	api: {
		register: vi.fn().mockResolvedValue({})
	}
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
		expect(screen.getByRole('button', { name: /Create an account/ })).toBeTruthy();
	});

	it('shows the sign-in link', () => {
		render(Page);
		expect(screen.getByText('Already have an account?')).toBeTruthy();
		expect(screen.getByText('Sign in')).toBeTruthy();
	});
});
