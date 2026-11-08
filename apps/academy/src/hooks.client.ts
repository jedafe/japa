import type { HandleClientError } from '@sveltejs/kit/hooks';

// Unexpected client-side errors render the nearest +error.svelte with this
// message instead of a bare exception. Dependency-free (no Sentry here).
export const handleError: HandleClientError = ({ error }) => {
	console.error('[jc:client-error]', error);
	return { code: 'internal_error', message: 'Something went wrong. Please try again.' };
};
