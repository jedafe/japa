import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import LoginForm from './LoginForm.svelte';

vi.mock('$app/navigation', () => ({
	invalidateAll: vi.fn().mockResolvedValue(undefined),
	goto: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$app/state', () => ({
	page: {
		url: new URL('http://localhost/login'),
		data: {}
	}
}));

describe('LoginForm', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		global.fetch = vi.fn();
	});

	it('renders email and password fields and submit button', () => {
		const { container, getByRole } = render(LoginForm);
		expect(container.querySelector('#login-email')).toBeTruthy();
		expect(container.querySelector('#login-password')).toBeTruthy();
		expect(getByRole('button', { name: /sign in/i })).toBeTruthy();
	});

	it('shows validation error for empty fields on submit', async () => {
		const { getByRole, container } = render(LoginForm);
		vi.mocked(global.fetch).mockClear(); // mount-time fetches (csrf/session) — only assert on submit
		await fireEvent.click(getByRole('button', { name: /sign in/i }));
		// Validation should prevent submission — fetch not called
		expect(global.fetch).not.toHaveBeenCalled();
	});

	it('calls /api/auth/login on valid submit', async () => {
		const mockFetch = vi.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({ user: { id: '1' }, redirect: '/' })
		});
		global.fetch = mockFetch;

		const { getByRole, container } = render(LoginForm);
		vi.mocked(global.fetch).mockClear(); // mount-time fetches (csrf/session) — only assert on submit
		const emailInput = container.querySelector('#login-email') as HTMLInputElement;
		const passwordInput = container.querySelector('#login-password') as HTMLInputElement;
		await fireEvent.input(emailInput, { target: { value: 'user@example.com' } });
		await fireEvent.input(passwordInput, { target: { value: 'Password123' } });
		await fireEvent.click(getByRole('button', { name: /sign in/i }));

		await waitFor(() => {
			expect(mockFetch).toHaveBeenCalledWith('/api/auth/login', expect.objectContaining({
				method: 'POST'
			}));
		});
	});

	it('shows error on API failure (invalid_credentials)', async () => {
		const mockFetch = vi.fn().mockResolvedValue({
			ok: false,
			status: 401,
			json: () => Promise.resolve({ error: { code: 'invalid_credentials' } })
		});
		global.fetch = mockFetch;

		const { getByRole, container, getByText } = render(LoginForm);
		const emailInput = container.querySelector('#login-email') as HTMLInputElement;
		const passwordInput = container.querySelector('#login-password') as HTMLInputElement;
		await fireEvent.input(emailInput, { target: { value: 'user@example.com' } });
		await fireEvent.input(passwordInput, { target: { value: 'wrongpass' } });
		await fireEvent.click(getByRole('button', { name: /sign in/i }));

		await waitFor(() => {
			expect(getByText(/invalid email or password/i)).toBeTruthy();
		});
	});
});
