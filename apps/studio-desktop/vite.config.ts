import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';

const dev = process.argv.includes('dev');

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter({ fallback: 'index.html' }),
			prerender: { entries: [] }
		})
	],
	clearScreen: false,
	server: {
		port: 3010,
		strictPort: true,
		watch: {
			ignored: ['**/src-tauri/**']
		}
	}
});
