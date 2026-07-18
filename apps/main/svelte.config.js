import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// Public-facing community app is SSR via adapter-node + Bun (port 3000).
		adapter: adapter()
	}
};

export default config;
