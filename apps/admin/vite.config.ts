import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';

const dev = process.argv.includes('dev');

// Origins the browser legitimately connects to (API, realtime WS, Umami,
// GlitchTip/Sentry). PUBLIC_* vars are build-time constants — CSP origins
// derived from them cannot drift from what the client bundle calls.
const envOrigin = (v: string | undefined) => {
	try {
		const u = new URL(v ?? '');

		return (/^https?:|^wss?:/).test(u.protocol) ? `${u.protocol}//${u.host}` : '';
	} catch {
		return '';
	}
};

const connectOrigins = [
	envOrigin(process.env.PUBLIC_JC_API_URL),
	envOrigin(process.env.PUBLIC_JC_WS_URL),
	envOrigin(process.env.PUBLIC_JC_UMAMI_URL),
	envOrigin(process.env.PUBLIC_JC_GLITCHTIP_DSN)
].filter(Boolean);

const umamiOrigin = envOrigin(process.env.PUBLIC_JC_UMAMI_URL);

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({ preprocess: vitePreprocess(), adapter: adapter() })
	],
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
