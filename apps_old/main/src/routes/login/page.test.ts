import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'auth.sign_in': 'Sign in',
		'auth.welcome_back': 'Welcome back to JapaConnect',
		'auth.email_label': 'Email',
		'auth.password_label': 'Password',
		'auth.forgot_password': 'Forgot password?',
		'auth.or_continue_with': 'Or continue with',
		'auth.continue_google': 'Continue with Google',
		'auth.continue_github': 'Continue with GitHub',
		'auth.no_account': "Don't have an account?",
		'auth.create_one': 'Create one'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/analytics', () => ({
	trackEvent: vi.fn(),
	AnalyticsEvents: { LOGIN: 'login' }
}));

vi.mock('$lib/api', () => ({
	api: {
		login: vi.fn().mockResolvedValue({ token: null })
	}
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
