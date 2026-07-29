import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import SignupForm from './SignupForm.svelte';

vi.mock('$app/navigation', () => ({
	invalidateAll: vi.fn().mockResolvedValue(undefined),
	goto: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$app/state', () => ({
	page: {
		url: new URL('http://localhost/signup'),
		data: {}
	}
}));

describe('SignupForm', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		global.fetch = vi.fn();
	});

	it('renders email, username, password fields and submit button', () => {
		const { container, getByRole } = render(SignupForm);
		expect(container.querySelector('#signup-email')).toBeTruthy();
		expect(container.querySelector('#signup-username')).toBeTruthy();
		expect(container.querySelector('#signup-password')).toBeTruthy();
		expect(getByRole('button', { name: /create account/i })).toBeTruthy();
	});

	it('shows validation error for empty fields on submit', async () => {
		const { getByRole } = render(SignupForm);
		await fireEvent.click(getByRole('button', { name: /create account/i }));
		expect(global.fetch).not.toHaveBeenCalled();
	});

	it('calls /api/auth/signup on valid submit', async () => {
		const mockFetch = vi.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({ user: { id: '1' }, redirect: '/onboarding' })
		});
		global.fetch = mockFetch;

		const { getByRole, container } = render(SignupForm);
		const emailInput = container.querySelector('#signup-email') as HTMLInputElement;
		const usernameInput = container.querySelector('#signup-username') as HTMLInputElement;
		const passwordInput = container.querySelector('#signup-password') as HTMLInputElement;
		const termsInput = container.querySelector('#signup-terms') as HTMLInputElement;
		await fireEvent.input(emailInput, { target: { value: 'new@example.com' } });
		await fireEvent.input(usernameInput, { target: { value: 'newuser' } });
		await fireEvent.input(passwordInput, { target: { value: 'Password123' } });
		await fireEvent.click(termsInput); // accept terms — required client-side gate
		await fireEvent.click(getByRole('button', { name: /create account/i }));

		await waitFor(() => {
			expect(mockFetch).toHaveBeenCalledWith('/api/auth/signup', expect.objectContaining({
				method: 'POST'
			}));
		});
	});

	it('blocks submit until terms are accepted', async () => {
		const mockFetch = vi.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({ redirect: '/onboarding' })
		});
		global.fetch = mockFetch;

		const { getByRole, container, getByText } = render(SignupForm);
		const emailInput = container.querySelector('#signup-email') as HTMLInputElement;
		const usernameInput = container.querySelector('#signup-username') as HTMLInputElement;
		const passwordInput = container.querySelector('#signup-password') as HTMLInputElement;
		await fireEvent.input(emailInput, { target: { value: 'new@example.com' } });
		await fireEvent.input(usernameInput, { target: { value: 'newuser' } });
		await fireEvent.input(passwordInput, { target: { value: 'Password123' } });
		await fireEvent.click(getByRole('button', { name: /create account/i }));

		await waitFor(() => {
			expect(getByText(/please accept the terms/i)).toBeTruthy();
		});
		expect(mockFetch).not.toHaveBeenCalled();
	});

	it('shows email_exists error from API', async () => {
		const mockFetch = vi.fn().mockResolvedValue({
			ok: false,
			status: 409,
			json: () => Promise.resolve({ error: { code: 'email_exists' } })
		});
		global.fetch = mockFetch;

		const { getByRole, container, getByText } = render(SignupForm);
		const emailInput = container.querySelector('#signup-email') as HTMLInputElement;
		const usernameInput = container.querySelector('#signup-username') as HTMLInputElement;
		const passwordInput = container.querySelector('#signup-password') as HTMLInputElement;
		const termsInput = container.querySelector('#signup-terms') as HTMLInputElement;
		await fireEvent.input(emailInput, { target: { value: 'existing@example.com' } });
		await fireEvent.input(usernameInput, { target: { value: 'existinguser' } });
		await fireEvent.input(passwordInput, { target: { value: 'Password123' } });
		await fireEvent.click(termsInput); // accept terms — required client-side gate
		await fireEvent.click(getByRole('button', { name: /create account/i }));

		await waitFor(() => {
			expect(getByText(/already in use/i)).toBeTruthy();
		});
	});
});
