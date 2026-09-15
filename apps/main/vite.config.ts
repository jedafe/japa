import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import type { Connect, Plugin } from 'vite';

// Dev-only backend target. In production the SSR server talks to the backend via
// PRIVATE_API_BASE_URL ($lib/server/api); this proxy only serves browser fetches.
const API_TARGET = 'http://localhost:8080';

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
					if (url.startsWith('/api/v1/') || url.startsWith('/ws')) {
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

export default defineConfig({
	plugins: [tailwindcss(), apiHeadersPlugin(), sveltekit(), svelteTesting()],
	server: {
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
