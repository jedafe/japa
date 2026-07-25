import { describe, it, expect } from 'vitest';
import { validatePassword } from './security';

describe('validatePassword', () => {
	it('requires the current password', () => {
		expect(validatePassword('', 'Abcd1234!', 'Abcd1234!').reason).toBe('settings.security.err_current');
	});
	it('enforces a minimum strength (>=8 with letter+number)', () => {
		expect(validatePassword('x', 'short', 'short').reason).toBe('settings.security.err_weak');
		expect(validatePassword('x', 'allletters', 'allletters').reason).toBe('settings.security.err_weak');
	});
	it('requires confirm to match', () => {
		expect(validatePassword('x', 'Abcd1234', 'Abcd9999').reason).toBe('settings.security.err_mismatch');
	});
	it('passes a strong matching pair', () => {
		expect(validatePassword('x', 'Abcd1234', 'Abcd1234').ok).toBe(true);
	});
});
