import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import type { Connect, Plugin } from 'vite';

type CspSource = NonNullable<
	NonNullable<NonNullable<NonNullable<Parameters<typeof sveltekit>[0]>['csp']>['directives']>['connect-src']
>[number];

// Dev-only backend target. In production the SSR server talks to the backend
// via PRIVATE_API_BASE_URL ($lib/server/api); this proxy only serves browser
// fetches (e.g. the API playground's real calls).
const API_TARGET = 'http://localhost:8080';

/**
 * For client-side fetches to /api/v1 and /ws: turn the jc_token cookie into a
 * Bearer header and forward the request host (port stripped) for reverse-proxy
 * correctness. Mirrors the pattern used by the main/academy apps.
 *
 * NOTE: only /api/v1/* is proxied — SvelteKit API routes (+server.ts endpoints)
 * must NOT be forwarded to the Go backend.
 */
function apiHeadersPlugin(): Plugin {
	return {
		name: 'jc-docs-api-headers',
		configureServer(server) {
			server.middlewares.use(
				(req: Connect.IncomingMessage, _res, next: Connect.NextFunction) => {
				const url = req.url ?? '';
				if (url.startsWith('/api/v1/') || url.startsWith('/ws') || url.startsWith('/seaweedfs')) {
					const cookie = req.headers.cookie ?? '';
					const match = (/jc_token=([^;]+)/).exec(cookie);
					if (match) req.headers.authorization = `Bearer ${match[1]}`;
					const host = (req.headers.host ?? '').split(':')[0];
					if (host) req.headers['x-forwarded-host'] = host;
				}
				next();
			});
		}
	};
}

const dev = process.argv.includes('dev');

// Origins the browser legitimately connects to (API, realtime WS, Umami,
// GlitchTip). PUBLIC_* vars are build-time constants — CSP origins derived
// from them cannot drift from what the client bundle actually calls.
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
	define: {
		// §124 — per-build/dev-start cache namespace for the service worker
		// (replaces $service-worker version; see src/service-worker.ts).
		__SW_VERSION__: JSON.stringify(String(Date.now()))
	},
	plugins: [
		tailwindcss(),
		apiHeadersPlugin(),
		sveltekit({
			preprocess: vitePreprocess(),
			// Developer + user documentation is SSR via adapter-node + Bun (port 3004).
			adapter: adapter(),
			// Nonce-based CSP in production; disabled in dev where Vite injects
			// unnonced runtime <style> tags.
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
								'frame-src': [
									'https://www.youtube.com',
									'https://www.youtube-nocookie.com',
									'https://player.vimeo.com'
								],
								'worker-src': ['self'],
								'manifest-src': ['self'],
								'form-action': ['self'],
								'object-src': ['none'],
								'base-uri': ['self'],
								'frame-ancestors': ['self', 'https://japa.to', 'https://*.japa.to']
							}
						}
					})
		})
	],
	resolve: {
		// The workspace packages (@japaconnect/ui, ai-panel, landing-blocks)
		// carry stale bun-installed svelte copies in their own node_modules.
		// Without dedupe, Vite bundles a second svelte/internal/client whose
		// init_operations() never runs — hydration then crashes with
		// "Cannot read properties of undefined (reading 'call')".
		dedupe: ['svelte']
	},
	server: {
		proxy: {
			'/api/v1/ws': { target: API_TARGET.replace(/^http/, 'ws'), ws: true, changeOrigin: true },
			'/api/v1': { target: API_TARGET, changeOrigin: true },
			'/ws': { target: API_TARGET.replace(/^http/, 'ws'), ws: true, changeOrigin: true },
			'/seaweedfs': { target: API_TARGET, changeOrigin: true }
		}
	},
	test: {
		environment: 'jsdom',
		globals: true,
		include: ['src/**/*.{test,spec}.{js,ts}']
	}
});
