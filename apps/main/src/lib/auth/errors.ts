// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Maps backend ApiError codes to field-level i18n message keys.

import type { AuthErrorCode } from '$lib/types';

export interface FieldError {
	field: 'email' | 'username' | 'password' | 'form' | null;
	messageKey: string;
}

/** Map a backend error code to a field + i18n key for form display. */
export function mapAuthError(code: string, status?: number): FieldError {
	const map: Record<AuthErrorCode, FieldError> = {
		missing_fields: { field: 'form', messageKey: 'auth.errors.missing_fields' },
		invalid_credentials: { field: 'form', messageKey: 'auth.errors.invalid_credentials' },
		email_exists: { field: 'email', messageKey: 'auth.errors.email_exists' },
		weak_password: { field: 'password', messageKey: 'auth.errors.weak_password' },
		password_breached: { field: 'password', messageKey: 'auth.errors.password_breached' },
		invalid_body: { field: 'form', messageKey: 'auth.errors.invalid_body' },
		tenant_unavailable: { field: 'form', messageKey: 'auth.errors.tenant_unavailable' },
		unsupported_provider: { field: 'form', messageKey: 'auth.errors.oauth_unsupported_provider' },
		state_error: { field: 'form', messageKey: 'auth.errors.oauth_state_error' },
		invalid_state: { field: 'form', messageKey: 'auth.errors.oauth_state_error' },
		missing_email: { field: 'form', messageKey: 'auth.errors.oauth_missing_email' },
		create_failed: { field: 'form', messageKey: 'auth.errors.create_failed' },
		token_error: { field: 'form', messageKey: 'auth.errors.token_error' }
	};

	const mapped = map[code as AuthErrorCode];
	if (mapped) return mapped;

	if (status === 429) return { field: 'form', messageKey: 'auth.errors.rate_limited' };

	return { field: 'form', messageKey: 'auth.errors.generic' };
}
