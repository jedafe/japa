import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';

const dev = process.argv.includes('dev');

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter({ fallback: 'index.html' }),
			prerender: { entries: [] },
			// Hash-based CSP emitted as a <meta> tag — tightens script-src to
			// hashed inline scripts. Intersects with the Tauri config CSP header
			// (src-tauri/tauri.conf.json): the header still governs connect-src
			// and keeps 'unsafe-inline' so hashed bootstrap scripts satisfy both
			// policies, but arbitrary injected inline scripts fail the meta policy.
			...(dev
				? {}
				: {
						csp: {
							mode: 'hash' as const,
							directives: {
								'default-src': ['self'],
								'script-src': ['self'],
								'style-src': ['self', 'unsafe-inline'],
								'img-src': ['self', 'data:', 'blob:', 'https:'],
								'font-src': ['self', 'data:'],
								'connect-src': ['self', 'https:', 'http:', 'wss:', 'ws:'],
								'media-src': ['self', 'https:', 'blob:', 'data:'],
								'worker-src': ['self'],
								'object-src': ['none'],
								'base-uri': ['self']
							}
						}
					})
		})
	],
	clearScreen: false,
	server: {
		port: 3010,
		strictPort: true,
		watch: {
			ignored: ['**/src-tauri/**']
		}
	}
});
