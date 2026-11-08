import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import type { Plugin } from 'vite';

function authProxyPlugin(): Plugin {
	return {
		name: 'auth-proxy',
		configureServer(server) {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			server.middlewares.use((req: any, _res: any, next: () => void) => {
				const url = req.url || '';
				if (url.startsWith('/api/') || url.startsWith('/control')) {
					const headers: Record<string, string> = req.headers ?? {};
					const cookie = headers.cookie || '';
					const match = cookie.match(/jc_token=([^;]+)/);
					if (match) {
						headers['authorization'] = `Bearer ${match[1]}`;
					}
				}
				next();
			});
		}
	};
}

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
		authProxyPlugin(),
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter({ fallback: 'index.html' }),
			// Hash-based CSP for the static SPA — SvelteKit emits a
			// <meta http-equiv> policy with sha256 hashes for its inline
			// bootstrap script. frame-ancestors cannot go in a meta tag; the
			// spa-server sets it as a real header (docker/spa-server.mjs).
			...(dev
				? {}
				: {
						csp: {
							mode: 'hash' as const,
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
								// /analytics embeds the Umami dashboard — the parent-side
								// counterpart to Umami's frame-ancestors.
								'frame-src': ['self', ...(umamiOrigin ? [umamiOrigin] : [])],
								'media-src': ['self', 'https:', 'blob:', 'data:'],
								'worker-src': ['self'],
								'manifest-src': ['self'],
								'form-action': ['self'],
								'object-src': ['none'],
								'base-uri': ['self']
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
		// Vite's root is apps/control; @fontsource/inter is hoisted to the
		// workspace node_modules/.bun outside that root, so disable fs.strict
		// in dev to allow those static font assets to be served.
		fs: {
			strict: false
		},
		proxy: {
			'/api/': {
				target: 'http://localhost:8080',
				changeOrigin: true
			},
			'/ws': {
				target: 'ws://localhost:8080',
				ws: true
			},
			'/control': {
				target: 'http://localhost:8080',
				changeOrigin: true
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
