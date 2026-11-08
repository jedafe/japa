import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import type { Connect, Plugin } from 'vite';

// Dev-only backend target. In production the SSR server talks to the backend via
// PRIVATE_API_BASE_URL ($lib/server/api); this proxy only serves browser fetches.
const API_TARGET = 'http://localhost:8080';

/**
 * For client-side fetches to /api/v1 and /ws: turn the jc_token cookie into a
 * Bearer header and forward the request host (port stripped) for reverse-proxy
 * correctness. Mirrors the pattern used by the main app.
 *
 * NOTE: only /api/v1/* is proxied — /api/auth/* are SvelteKit API routes
 * (+server.ts endpoints) and must NOT be forwarded to the Go backend.
 */
function apiHeadersPlugin(): Plugin {
	return {
		name: 'jc-learn-api-headers',
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

// Origins the browser legitimately connects to (API, realtime WS, Umami).
// PUBLIC_* vars are build-time constants — the same values are compiled
// into the client bundle, so CSP origins derived from them cannot drift.
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
			// Learning platform is SSR via adapter-node + Bun (port 3003).
			adapter: adapter()
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
