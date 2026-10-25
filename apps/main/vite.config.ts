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
						const match = /jc_token=([^;]+)/.exec(cookie);
						if (match) req.headers.authorization = `Bearer ${match[1]}`;
						const host = (req.headers.host ?? '').split(':')[0];
						if (host) req.headers['x-forwarded-host'] = host;
					}
					next();
				}
			);
		}
	};
}

/**
 * One app CSS file instead of per-component splits — the guest landing page
 * linked 8 stylesheets, all render-blocking (~2s est. on mobile PageSpeed).
 * SvelteKit forces cssCodeSplit=true in its own config hook, so this plugin is
 * registered after sveltekit() to win the merge order.
 */
function mergedCssPlugin(): Plugin {
	return {
		name: 'jc-merged-css',
		config: () => ({ build: { cssCodeSplit: false } })
	};
}

export default defineConfig({
	plugins: [tailwindcss(), apiHeadersPlugin(), sveltekit(), svelteTesting(), mergedCssPlugin()],
	build: {
		rollupOptions: {
			output: {
				codeSplitting: {
					groups: [
						{
							// The tiptap/prosemirror composer stack (~250KB transfer)
							// was being merged into shared vendor chunks that the (app)
							// layout pulls — shipping it to guests who never render an
							// editor. Force it into its own chunk so only routes that
							// statically import the editor pay for it.
							name: 'editor',
							test: /node_modules[\\/](@tiptap|prosemirror)/,
							priority: 10
						}
					]
				}
			}
		}
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
