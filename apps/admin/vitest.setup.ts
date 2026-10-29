import { vi, afterEach } from 'vitest';
import { cleanup } from '@testing-library/svelte';
import en from './src/lib/locales/en.json';

// jsdom does not implement ResizeObserver, but @dnd-kit/svelte (sortable
// drag-and-drop) and floating-ui (bits-ui popovers) read it at import/run time.
// Provide a constructable stub before any test modules are loaded.
class ResizeObserverMock {
	constructor(_callback: ResizeObserverCallback) {}
	observe = vi.fn();
	unobserve = vi.fn();
	disconnect = vi.fn();
}
globalThis.ResizeObserver = ResizeObserverMock as unknown as typeof ResizeObserver;

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

// jsdom does not implement matchMedia. Components that use Svelte's
// MediaQuery rune (theme/responsive helpers, LayerChart motion) call it at
// evaluation time — polyfill before any component imports run.
if (typeof window !== 'undefined' && !window.matchMedia) {
	window.matchMedia = (query: string): MediaQueryList => ({
		matches: false,
		media: query,
		onchange: null,
		addEventListener: () => {},
		removeEventListener: () => {},
		addListener: () => {},
		removeListener: () => {},
		dispatchEvent: () => false
	});
}
