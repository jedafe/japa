import type { HandleClientError } from '@sveltejs/kit/hooks';

// Unexpected client-side errors (render throws, hydration faults, failed
// client navigations) land here — SvelteKit renders the nearest
// +error.svelte with this message instead of a bare exception. Kept
// dependency-free: admin/control pipe to GlitchTip via @sentry/sveltekit,
// but this app carries no Sentry dep.
export const handleError: HandleClientError = ({ error }) => {
	console.error('[jc:client-error]', error);
	return { code: 'internal_error', message: 'Something went wrong. Please try again.' };
};
