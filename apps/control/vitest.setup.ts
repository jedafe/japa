import { afterEach, vi } from 'vitest';
import { cleanup } from '@testing-library/svelte';
import en from './src/lib/locales/en.json';

// SvelteKit 3's generated `$app/env/public` module reads
// `globalThis.__sveltekit_dev.env` at import time; vitest has no kit runtime,
// so provide the global before any test module touches it.
(globalThis as any).__sveltekit_dev ??= { env: {} };

// bits-ui's body-scroll-lock schedules a setTimeout cleanup that can fire
// after jsdom is torn down, causing "document is not defined" errors. Unmount
// all rendered components and wait for the ~24ms cleanup to finish before
// the next test/file runs.
afterEach(async () => {
	cleanup();
	await new Promise((resolve) => setTimeout(resolve, 50));
});

// jsdom doesn't implement matchMedia; third-party UI (LayerChart/Bits UI) calls
// it during render, so polyfill it to keep component tests from throwing.
if (typeof window !== 'undefined' && !window.matchMedia) {
	window.matchMedia = ((query: string) => ({
		matches: false,
		media: query,
		onchange: null,
		addListener: () => {},
		removeListener: () => {},
		addEventListener: () => {},
		removeEventListener: () => {},
		dispatchEvent: () => false
	})) as unknown as typeof window.matchMedia;
}

// Pages import from '#lib/stores/i18n.svelte.js', so the mock
// must target that exact specifier or it silently never applies and t() returns
// raw i18n keys, breaking every getByText assertion.
vi.mock('#lib/stores/i18n.svelte.js', async () => {
    const actual = await vi.importActual<typeof import('#lib/stores/i18n.svelte.js')>('#lib/stores/i18n.svelte.js');
    return {
        ...actual,
        t: (key: string, fallback?: string) => en[key] ?? fallback ?? key,
        loadTranslations: vi.fn().mockResolvedValue(en)
    };
});
