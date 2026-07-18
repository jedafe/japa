import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import type { Connect, Plugin } from 'vite';

// Dev-only backend target. In production the SSR server talks to the backend via
// PRIVATE_API_BASE_URL ($lib/server/api); this proxy only serves browser fetches.
const API_TARGET = 'http://localhost:8080';

/**
 * For client-side fetches to /api and /ws: turn the jc_token cookie into a Bearer
 * header and forward the tenant host (port stripped) so the backend resolves the
 * tenant. Mirrors the pattern used by the control app.
 */
function apiHeadersPlugin(): Plugin {
	return {
		name: 'jc-api-headers',
		configureServer(server) {
			server.middlewares.use(
				(req: Connect.IncomingMessage, _res, next: Connect.NextFunction) => {
					const url = req.url ?? '';
					if (url.startsWith('/api/') || url.startsWith('/ws')) {
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
	plugins: [tailwindcss(), apiHeadersPlugin(), sveltekit()],
	server: {
		proxy: {
			'/api': { target: API_TARGET, changeOrigin: true },
			'/ws': { target: API_TARGET.replace(/^http/, 'ws'), ws: true, changeOrigin: true }
		}
	},
	test: {
		environment: 'jsdom',
		globals: true,
		include: ['src/**/*.{test,spec}.{js,ts}'],
		setupFiles: ['./vitest.setup.ts']
	}
});
