import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// Developer + user documentation is SSR via adapter-node + Bun (port 3004).
		// Canonical host: docs.japaconnect.com (ADR-060 — Docs drift on the
		// unified app shell: nav-tree-as-left-nav, TOC-as-right-sidebar).
		adapter: adapter()
	}
};

export default config;
