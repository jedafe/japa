import { describe, it, expect } from 'vitest';
import {
	validateLogin,
	validateSignup,
	validateForgotPassword,
	validateResetPassword,
	passwordStrength
} from './validation';

describe('validateLogin', () => {
	it('passes with valid email and password', () => {
		const r = validateLogin('user@example.com', 'password123');
		expect(r.valid).toBe(true);
		expect(r.errors).toEqual({});
	});

	it('fails with empty email', () => {
		const r = validateLogin('', 'password123');
		expect(r.valid).toBe(false);
		expect(r.errors.email).toBeTruthy();
	});

	it('fails with invalid email', () => {
		const r = validateLogin('notanemail', 'password123');
		expect(r.valid).toBe(false);
		expect(r.errors.email).toBeTruthy();
	});

	it('fails with empty password', () => {
		const r = validateLogin('user@example.com', '');
		expect(r.valid).toBe(false);
		expect(r.errors.password).toBeTruthy();
	});
});

describe('validateSignup', () => {
	it('passes with valid fields', () => {
		const r = validateSignup('user@example.com', 'validuser', 'Password123');
		expect(r.valid).toBe(true);
	});

	it('fails with short username', () => {
		const r = validateSignup('user@example.com', 'ab', 'Password123');
		expect(r.valid).toBe(false);
		expect(r.errors.username).toBeTruthy();
	});

	it('fails with username with special chars', () => {
		const r = validateSignup('user@example.com', 'user@name', 'Password123');
		expect(r.valid).toBe(false);
		expect(r.errors.username).toBeTruthy();
	});

	it('fails with weak password (no uppercase)', () => {
		const r = validateSignup('user@example.com', 'validuser', 'password123');
		expect(r.valid).toBe(false);
		expect(r.errors.password).toBeTruthy();
	});

	it('fails with weak password (too short)', () => {
		const r = validateSignup('user@example.com', 'validuser', 'Ab1');
		expect(r.valid).toBe(false);
		expect(r.errors.password).toBeTruthy();
	});
});

describe('validateForgotPassword', () => {
	it('passes with valid email', () => {
		const r = validateForgotPassword('user@example.com');
		expect(r.valid).toBe(true);
	});

	it('fails with empty email', () => {
		const r = validateForgotPassword('');
		expect(r.valid).toBe(false);
		expect(r.errors.email).toBeTruthy();
	});
});

describe('validateResetPassword', () => {
	it('passes with matching strong passwords', () => {
		const r = validateResetPassword('Password123', 'Password123');
		expect(r.valid).toBe(true);
	});

	it('fails with mismatched passwords', () => {
		const r = validateResetPassword('Password123', 'Password456');
		expect(r.valid).toBe(false);
		expect(r.errors.confirm).toBeTruthy();
	});

	it('fails with weak password', () => {
		const r = validateResetPassword('weak', 'weak');
		expect(r.valid).toBe(false);
		expect(r.errors.password).toBeTruthy();
	});
});

describe('passwordStrength', () => {
	it('returns 0 for empty', () => {
		expect(passwordStrength('')).toBe(0);
	});

	it('returns 1 for 8+ chars only', () => {
		expect(passwordStrength('aaaaaaaa')).toBe(1);
	});

	it('returns higher score for complex passwords', () => {
		const score = passwordStrength('Abcdefgh123!');
		expect(score).toBeGreaterThanOrEqual(3);
	});
});
