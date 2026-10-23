import { afterEach, vi } from 'vitest';
import en from './src/lib/locales/en.json';

// Bits UI's body scroll lock schedules a ~24ms setTimeout cleanup when a
// modal unmounts. If that timer outlives the test file's jsdom teardown it
// throws "document is not defined" as an uncaught exception and fails the
// run. Wait a tick after each test so pending timers fire while the
// environment still exists.
afterEach(async () => {
	await new Promise((r) => setTimeout(r, 30));
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

// Pages import from '$lib/stores/i18n.svelte', so the mock
// must target that exact specifier or it silently never applies and t() returns
// raw i18n keys, breaking every getByText assertion.
vi.mock('$lib/stores/i18n.svelte', async () => {
    const actual = await vi.importActual<typeof import('$lib/stores/i18n.svelte')>('$lib/stores/i18n.svelte');
    return {
        ...actual,
        t: (key: string, fallback?: string) => en[key] ?? fallback ?? key,
        loadTranslations: vi.fn().mockResolvedValue(en)
    };
});
