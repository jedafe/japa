// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Client-side validation for auth forms. The backend is the source of truth;
// these functions provide instant feedback before submission.

export interface ValidationResult {
	valid: boolean;
	errors: Record<string, string>;
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const USERNAME_RE = /^[a-zA-Z0-9_]{3,30}$/;

/** Validate a login form submission. */
export function validateLogin(email: string, password: string): ValidationResult {
	const errors: Record<string, string> = {};

	if (!email.trim()) {
		errors.email = 'auth.errors.missing_fields';
	} else if (!EMAIL_RE.test(email.trim())) {
		errors.email = 'auth.errors.invalid_email';
	}

	if (!password) {
		errors.password = 'auth.errors.missing_fields';
	}

	return { valid: Object.keys(errors).length === 0, errors };
}

/** Validate a signup form submission. */
export function validateSignup(email: string, username: string, password: string): ValidationResult {
	const errors: Record<string, string> = {};

	if (!email.trim()) {
		errors.email = 'auth.errors.missing_fields';
	} else if (!EMAIL_RE.test(email.trim())) {
		errors.email = 'auth.errors.invalid_email';
	}

	if (!username.trim()) {
		errors.username = 'auth.errors.missing_fields';
	} else if (!USERNAME_RE.test(username.trim())) {
		errors.username = 'auth.errors.invalid_username';
	}

	if (!password) {
		errors.password = 'auth.errors.missing_fields';
	} else if (password.length < 8) {
		errors.password = 'auth.errors.weak_password';
	} else if (!/[a-z]/.test(password) || !/[A-Z]/.test(password) || !/[0-9]/.test(password)) {
		errors.password = 'auth.errors.weak_password';
	}

	return { valid: Object.keys(errors).length === 0, errors };
}

/** Validate a forgot-password form submission. */
export function validateForgotPassword(email: string): ValidationResult {
	const errors: Record<string, string> = {};

	if (!email.trim()) {
		errors.email = 'auth.errors.missing_fields';
	} else if (!EMAIL_RE.test(email.trim())) {
		errors.email = 'auth.errors.invalid_email';
	}

	return { valid: Object.keys(errors).length === 0, errors };
}

/** Validate a reset-password form submission. */
export function validateResetPassword(password: string, confirm: string): ValidationResult {
	const errors: Record<string, string> = {};

	if (!password) {
		errors.password = 'auth.errors.missing_fields';
	} else if (password.length < 8) {
		errors.password = 'auth.errors.weak_password';
	} else if (!/[a-z]/.test(password) || !/[A-Z]/.test(password) || !/[0-9]/.test(password)) {
		errors.password = 'auth.errors.weak_password';
	}

	if (password !== confirm) {
		errors.confirm = 'auth.errors.password_mismatch';
	}

	return { valid: Object.keys(errors).length === 0, errors };
}

/** Password strength score (0-4) for the signup strength meter. */
export function passwordStrength(password: string): 0 | 1 | 2 | 3 | 4 {
	if (!password) return 0;
	let score = 0;
	if (password.length >= 8) score++;
	if (password.length >= 12) score++;
	if (/[a-z]/.test(password) && /[A-Z]/.test(password)) score++;
	if (/[0-9]/.test(password) && /[^a-zA-Z0-9]/.test(password)) score++;
	return Math.min(score, 4) as 0 | 1 | 2 | 3 | 4;
}
