import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import type { Connect, Plugin } from 'vite';

// Dev-only backend target. In production the SSR server talks to the backend via
// PRIVATE_API_BASE_URL ($lib/server/api); this proxy only serves browser fetches.
const API_TARGET = 'http://localhost:8080';

// Branded-surface hosts (ADR-063 university, ADR-127 roadmap) must be listed in
// server.allowedHosts or the dev server 403s them before hooks.server.ts can
// detect the host. Canonical hosts plus the JC_*_HOSTS env lists — same sources
// isUniversitySurface/isRoadmapSurface consult.
const devAllowedHosts = [
	'academy.japa.to',
	'roadmap.japa.to',
	...(process.env.JC_ACADEMY_HOSTS ?? '').split(','),
	...(process.env.JC_ROADMAP_HOSTS ?? '').split(',')
]
	.map((h) => h.trim())
	.filter(Boolean);

/**
 * For client-side fetches to /api/v1 and /ws: turn the jc_token cookie into a
 * Bearer header and forward the request host (port stripped) for reverse-proxy
 * correctness. Mirrors the pattern used by the control app.
 *
 * NOTE: only /api/v1/* is proxied — /api/auth/* are SvelteKit API routes
 * (+server.ts endpoints) and must NOT be forwarded to the Go backend.
 */
function apiHeadersPlugin(): Plugin {
	return {
		name: 'jc-api-headers',
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

const dev = process.argv.includes('dev') || process.argv.includes('serve');

// PUBLIC_* vars live in .env files — Vite injects them into
// import.meta.env for client code, but they are NOT in process.env when
// this config file is evaluated. loadEnv reads .env/.env.local/
// .env.[mode] so the CSP connect-src origins stay in sync with what the
// bundle actually calls. Real environment variables (CI/deploy) win over
// file values.
const fileEnv = loadEnv(dev ? 'development' : 'production', process.cwd(), 'PUBLIC_');

const env = (key: string) => process.env[key] ?? fileEnv[key];

// Origins the browser legitimately connects to (API, realtime WS, Umami,
// GlitchTip). PUBLIC_* vars are build-time constants — the same values are
// compiled into the client bundle, so deriving CSP origins from them can
// never drift from what the app actually calls.
const envOrigin = (v: string | undefined) => {
	try {
		const u = new URL(v ?? '');

		return (/^https?:|^wss?:/).test(u.protocol) ? `${u.protocol}//${u.host}` : '';
	} catch {
		return '';
	}
};

const connectOrigins = [
	envOrigin(env('PUBLIC_JC_API_URL')),
	envOrigin(env('PUBLIC_JC_WS_URL')),
	envOrigin(env('PUBLIC_JC_UMAMI_URL')),
	envOrigin(env('PUBLIC_JC_GLITCHTIP_DSN'))
].filter(Boolean);

const umamiOrigin = envOrigin(env('PUBLIC_JC_UMAMI_URL'));

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
			// Public-facing community app is SSR via adapter-node + Bun (port 3000).
			adapter: adapter(),
			// Nonce-based CSP in production. Disabled in dev: Vite injects
			// unnonced runtime <style> tags and eval'd modules that a strict
			// policy would break. kit.csp is the only mechanism that nonces
			// SvelteKit's inline bootstrap script — a hand-set CSP header
			// cannot reach it and would break hydration.
			// event.locals.cspNonce is populated by SvelteKit here and is
			// consumed by the layout's staff-gated custom CSS/JS blocks.
			...(dev
				? {}
				: {
						csp: {
							mode: 'nonce' as const,
							directives: {
								'default-src': ['self'],
								// 'unsafe-hashes' + the sha256 below allow exactly one inline
								// handler: `this.__e=event`, which Svelte 5.56+ SSR emits as
								// onload/onerror on every <img> for hydration event replay.
								// Nonces can't authorize event-handler attributes, and without
								// this the handler is blocked → console errors (best-practices).
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
								'frame-ancestors': ['self']
							}
						}
					})
		}),
		svelteTesting()
	],
	resolve: {
		// The workspace packages (@japaconnect/ui, ai-panel, landing-blocks)
		// carry stale bun-installed svelte copies in their own node_modules
		// (svelte@5.56.8 vs the app's 5.57.2). Without dedupe, Vite bundles a
		// second svelte/internal/client whose init_operations() never runs —
		// hydration then crashes with "Cannot read properties of undefined
		// (reading 'call')" inside get_next_sibling.
		dedupe: ['svelte']
	},
	// $lib/appOrigins reads PUBLIC_* origins from import.meta.env, which Vite
	// only populates for envPrefix-matched keys (default: VITE_ only).
	envPrefix: ['VITE_', 'PUBLIC_'],
	server: {
		allowedHosts: devAllowedHosts,
		// Pre-transform the heaviest pages at server start so the first hit
		// doesn't pay multi-second on-demand compile.
		warmup: {
			clientFiles: ['./src/routes/(app)/comparisons/[slug]/+page.svelte', './src/routes/(app)/spaces/[slug]/+page.svelte', './src/routes/(app)/+page.svelte']
		},
		// Allow serving workspace-level .bun node_modules (e.g. @fontsource
		// fonts installed by Bun at the monorepo root). Without this, Vite
		// rejects requests like:
		//   /node_modules/.bun/@fontsource+inter@5.3.0/.../inter-latin-500-normal.woff2
		fs: {
			allow: ['..']
		},
		proxy: {
			// WebSocket upgrade must be matched before the plain /api/v1 entry
			// (which has no ws:true) so the dev proxy upgrades /api/v1/ws to the
			// backend's GET /api/v1/ws?ticket=... handler.
			'/api/v1/ws': { target: API_TARGET.replace(/^http/, 'ws'), ws: true, changeOrigin: true },
			'/api/v1': { target: API_TARGET, changeOrigin: true, timeout: 0, proxyTimeout: 0 },
			'/ws': { target: API_TARGET.replace(/^http/, 'ws'), ws: true, changeOrigin: true },
			// SeaweedFS upload fallback — dev-only presigned URL proxy.
			// When S3 is not configured, the backend returns /seaweedfs/upload/...
			// as the presigned URL; the client PUTs raw bytes there.
			'/seaweedfs': { target: API_TARGET, changeOrigin: true }
		}
	},
	test: {
		environment: 'jsdom',
		globals: true,
		include: ['src/**/*.{test,spec}.{js,ts}'],
		setupFiles: ['./vitest.setup.ts']
	}
});
