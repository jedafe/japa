import { vi, afterEach } from 'vitest';
import { cleanup } from '@testing-library/svelte';
import en from './src/lib/locales/en.json';

// Svelte 5 emits non-fatal `derived_inert` warnings from bits-ui / shared
// components when a derived is read outside a live effect during test teardown.
// The warning is not a test failure; filter it so the test output stays readable.
const originalWarn = console.warn;
console.warn = (...args: any[]) => {
	if (args.some((a) => typeof a === 'string' && a.includes('derived_inert'))) {
		return;
	}

	originalWarn(...args);
};

vi.mock('$lib/stores/i18n.svelte', () => ({
    t: (key: string, fallback?: string) => en[key] ?? fallback ?? key,
    locale: { current: 'en', set: vi.fn(), init: vi.fn() },
    getLocale: () => ({ current: 'en', set: vi.fn(), init: vi.fn() }),
    loadTranslations: vi.fn().mockResolvedValue(en)
}));

// bits-ui's body-scroll-lock schedules a setTimeout cleanup that can fire
// after jsdom is torn down, causing "document is not defined" errors. Unmount
// all rendered components and wait for the ~24ms cleanup to finish before
// the next test/file runs.
afterEach(async () => {
    cleanup();
    await new Promise((resolve) => setTimeout(resolve, 50));
});
