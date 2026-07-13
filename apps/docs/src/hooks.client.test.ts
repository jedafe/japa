import { describe, it, expect, vi } from 'vitest';

// @sentry/sveltekit's browser entry imports SvelteKit's `$app` internals,
// which vitest can't resolve from inside node_modules. Mock it so importing
// hooks.client (for its pure predicate) doesn't drag in that dependency.
vi.mock('@sentry/sveltekit', () => ({
	init: vi.fn(),
	handleErrorWithSentry: () => vi.fn()
}));

import { shouldInitErrorTracking } from './hooks.client';

describe('shouldInitErrorTracking', () => {
	it('is false when the DSN is empty', () => {
		expect(shouldInitErrorTracking('', false)).toBe(false);
	});

	it('is false when the DSN env var is not defined at all', () => {
		expect(shouldInitErrorTracking(undefined, false)).toBe(false);
	});

	it('is false in dev mode even with a DSN set', () => {
		expect(shouldInitErrorTracking('https://key@glitchtip.example/1', true)).toBe(false);
	});

	it('is true with a DSN set and not in dev mode', () => {
		expect(shouldInitErrorTracking('https://key@glitchtip.example/1', false)).toBe(true);
	});
});
