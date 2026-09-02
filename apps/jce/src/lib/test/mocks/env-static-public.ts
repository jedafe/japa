// Vitest mock for the SvelteKit $env/static/public module.
// SvelteKit does not generate env modules during unit tests, so this file
// stands in for it when resolving the alias in vitest.config.ts.

export const PUBLIC_JC_API_URL = 'http://localhost:8080/api/v1';
