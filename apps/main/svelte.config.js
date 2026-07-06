import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/**
 * SvelteKit configuration for the main community site.
 *
 * The production port is controlled by the `PORT` environment variable
 * (the adapter-node runtime reads it at startup), not by an adapter option.
 * The dev/preview ports are set in package.json scripts.
 *
 * @type {import('@sveltejs/kit').Config}
 */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter()
	}
};

export default config;