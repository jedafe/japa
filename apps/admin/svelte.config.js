import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/**
 * SvelteKit configuration for the admin panel.
 *
 * The admin app is a single-page application (SPA): adapter-static emits a
 * static bundle with an index.html fallback so client-side routing handles
 * every path. No server runtime is needed in production.
 *
 * @type {import('@sveltejs/kit').Config}
 */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			fallback: 'index.html'
		})
	}
};

export default config;
