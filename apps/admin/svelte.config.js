import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/**
 * SvelteKit configuration for the admin panel.
 *
 * Uses adapter-node so that hooks.server.ts runs per-request in production,
 * enabling proper httpOnly cookie-based auth guards and server-side logout.
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
