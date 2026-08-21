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

export default defineConfig({
	plugins: [tailwindcss(), authProxyPlugin(), sveltekit()],
	resolve: {
		conditions: ['browser']
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
