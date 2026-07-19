import { vi } from 'vitest';
import en from './src/lib/locales/en.json';

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

// Pages import from '$lib/stores/i18n' (NOT the .svelte variant), so the mock
// must target that exact specifier or it silently never applies and t() returns
// raw i18n keys, breaking every getByText assertion.
vi.mock('$lib/stores/i18n', async () => {
    const actual = await vi.importActual<typeof import('$lib/stores/i18n')>('$lib/stores/i18n');
    return {
        ...actual,
        t: (key: string, fallback?: string) => en[key] ?? fallback ?? key,
        loadTranslations: vi.fn().mockResolvedValue(en)
    };
});
