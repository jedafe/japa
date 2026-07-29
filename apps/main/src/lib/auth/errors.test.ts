import { describe, it, expect } from 'vitest';
import { mapAuthError } from './errors';

describe('mapAuthError', () => {
	it('maps missing_fields to form', () => {
		const r = mapAuthError('missing_fields');
		expect(r.field).toBe('form');
		expect(r.messageKey).toBe('auth.errors.missing_fields');
	});

	it('maps email_exists to email field', () => {
		const r = mapAuthError('email_exists');
		expect(r.field).toBe('email');
		expect(r.messageKey).toBe('auth.errors.email_exists');
	});

	it('maps weak_password to password field', () => {
		const r = mapAuthError('weak_password');
		expect(r.field).toBe('password');
		expect(r.messageKey).toBe('auth.errors.weak_password');
	});

	it('maps password_breached to password field', () => {
		const r = mapAuthError('password_breached');
		expect(r.field).toBe('password');
		expect(r.messageKey).toBe('auth.errors.password_breached');
	});

	it('maps invalid_credentials to form', () => {
		const r = mapAuthError('invalid_credentials');
		expect(r.field).toBe('form');
		expect(r.messageKey).toBe('auth.errors.invalid_credentials');
	});

	it('maps 429 status to rate_limited', () => {
		const r = mapAuthError('unknown', 429);
		expect(r.field).toBe('form');
		expect(r.messageKey).toBe('auth.errors.rate_limited');
	});

	it('falls back to generic for unknown codes', () => {
		const r = mapAuthError('some_unknown_code');
		expect(r.field).toBe('form');
		expect(r.messageKey).toBe('auth.errors.generic');
	});

	it('maps invalid_state and state_error to oauth_state_error', () => {
		expect(mapAuthError('invalid_state').messageKey).toBe('auth.errors.oauth_state_error');
		expect(mapAuthError('state_error').messageKey).toBe('auth.errors.oauth_state_error');
	});
});
