import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import type { Plugin } from 'vite';

function authProxyPlugin(): Plugin {
	return {
		name: 'auth-proxy',
		configureServer(server) {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			server.middlewares.use('/api', (req: any, _res: any, next: () => void) => {
				const headers: Record<string, string> = req.headers ?? {};
				const cookie = headers.cookie || '';
				const match = cookie.match(/jc_token=([^;]+)/);
				if (match) {
					headers['authorization'] = `Bearer ${match[1]}`;
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
	ssr: {
		noExternal: ['lucide-svelte']
	},
	server: {
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
