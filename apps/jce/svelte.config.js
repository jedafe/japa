import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// JCE is SSR via adapter-node on port 3006.
		// Canonical host: compare.japaconnect.com / japaconnect.com routes.
		adapter: adapter()
	}
};

export default config;
