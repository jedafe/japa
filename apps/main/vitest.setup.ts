import '@testing-library/svelte';
import { cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';

// bits-ui's body-scroll-lock schedules a setTimeout cleanup that can fire
// after jsdom is torn down, causing "document is not defined" errors. Unmount
// all rendered components and wait for the ~24ms cleanup to finish before
// the next test/file runs.
afterEach(async () => {
	cleanup();
	await new Promise((resolve) => setTimeout(resolve, 50));
});

// jsdom does not implement matchMedia. LayerChart's motion helpers use Svelte's
// MediaQuery rune, which calls window.matchMedia at module-evaluation time.
// Polyfill it before any component imports run.
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
