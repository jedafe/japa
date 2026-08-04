import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// Tools surface is SSR via adapter-node + Bun (port 3005).
		// Canonical host: tools.japaconnect.com (per-tool subdomains and custom
		// domains render the single-tool shell — ADR-086, §78).
		adapter: adapter()
	}
};

export default config;
