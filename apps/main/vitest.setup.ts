import '@testing-library/svelte';

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
