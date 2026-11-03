import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	resolve: {
		conditions: ['browser'],
		// Dedupe svelte: workspace packages carry stale bun-installed
		// svelte copies — a second svelte/internal/client in the bundle
		// crashes hydration (get_next_sibling 'call' of undefined).
		dedupe: ['svelte']
	},
	server: {
		// Fontsource packages are hoisted in the workspace root node_modules and
		// loaded through Vite's /@fs/... dev-server path. Allow that directory
		// so woff/woff2 font files are not 403'd in local dev.
		fs: {
			allow: ['../..']
		},
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				changeOrigin: true
			},
			'/ws': {
				target: 'ws://localhost:8080',
				ws: true
			}
		}
	},
	test: {
		environment: 'jsdom',
		globals: true,
		include: ['src/**/*.{test,spec}.{js,ts}'],
		setupFiles: ['./vitest.setup.ts']
	}
});
