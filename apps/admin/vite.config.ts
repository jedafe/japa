import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';

type CspSource = NonNullable<
	NonNullable<NonNullable<NonNullable<Parameters<typeof sveltekit>[0]>['csp']>['directives']>['connect-src']
>[number];

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
].filter(Boolean) as CspSource[];

const umamiOrigin = envOrigin(process.env.PUBLIC_JC_UMAMI_URL) as CspSource;

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter(),
			// Nonce-based CSP in production; disabled in dev where Vite injects
			// unnonced runtime <style> tags. frame-ancestors 'self' only — a
			// privileged panel must never be embedded cross-origin (clickjacking).
			...(dev
				? {}
				: {
						csp: {
							mode: 'nonce' as const,
							directives: {
								'default-src': ['self'],
								// 'unsafe-hashes' + the sha256 allow exactly one inline
								// handler: `this.__e=event`, emitted by Svelte 5.56+ SSR as
								// onload/onerror on <img> for hydration event replay.
								'script-src': [
									'self',
									'unsafe-hashes',
									'sha256-7dQwUgLau1NFCCGjfn9FsYptB6ZtWxJin6VohGIu20I=',
									...(umamiOrigin ? [umamiOrigin] : [])
								],
								'style-src': ['self', 'unsafe-inline'],
								'img-src': ['self', 'data:', 'blob:', 'https:'],
								'font-src': ['self', 'data:'],
								'connect-src': ['self', ...connectOrigins],
								'media-src': ['self', 'https:', 'blob:', 'data:'],
								'worker-src': ['self'],
								'manifest-src': ['self'],
								'form-action': ['self'],
								'object-src': ['none'],
								'base-uri': ['self'],
								'frame-ancestors': ['self']
							}
						}
					})
		})
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
