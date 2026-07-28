import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// Learning platform is SSR via adapter-node + Bun (port 3003).
		// Canonical host: learn.japaconnect.com (university.japaconnect.com host
		// renders the Japa University shell variant — ADR-063).
		adapter: adapter()
	}
};

export default config;
