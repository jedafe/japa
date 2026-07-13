import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit(),
		SvelteKitPWA({
			registerType: 'autoUpdate',
			manifest: {
				name: 'JapaConnect',
				short_name: 'JapaConnect',
				description: 'Community platform for the Nigerian diaspora',
				theme_color: '#4f46e5',
				background_color: '#ffffff',
				display: 'standalone',
				icons: [
					{ src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
					{ src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' }
				]
			},
		workbox: {
			globPatterns: ['client/**/*.{js,css,html,ico,png,svg}'],
			runtimeCaching: [
				{
					// Cache API responses (GET only) with NetworkFirst strategy
					urlPattern: /\/api\/v1\/.+/i,
					handler: 'NetworkFirst',
					options: {
						cacheName: 'api-cache',
						expiration: {
							maxEntries: 50,
							maxAgeSeconds: 5 * 60 // 5 minutes
						},
						networkTimeoutSeconds: 3
					}
				},
				{
					// Cache images with CacheFirst strategy
					urlPattern: /\.(png|jpg|jpeg|gif|webp|avif|svg)$/i,
					handler: 'CacheFirst',
					options: {
						cacheName: 'image-cache',
						expiration: {
							maxEntries: 100,
							maxAgeSeconds: 30 * 24 * 60 * 60 // 30 days
						}
					}
				},
				{
					// Cache Google Fonts stylesheets with StaleWhileRevalidate
					urlPattern: /^https:\/\/fonts\.googleapis\.com\/.+/i,
					handler: 'StaleWhileRevalidate',
					options: {
						cacheName: 'google-fonts-stylesheets'
					}
				},
				{
					// Cache Google Fonts webfont files with CacheFirst
					urlPattern: /^https:\/\/fonts\.gstatic\.com\/.+/i,
					handler: 'CacheFirst',
					options: {
						cacheName: 'google-fonts-webfonts',
						expiration: {
							maxEntries: 30,
							maxAgeSeconds: 60 * 60 * 24 * 365 // 1 year
						}
					}
				},
				{
					// Cache imgproxy responses with CacheFirst
					urlPattern: /\/imgproxy\//i,
					handler: 'CacheFirst',
					options: {
						cacheName: 'imgproxy-cache',
						expiration: {
							maxEntries: 100,
							maxAgeSeconds: 7 * 24 * 60 * 60 // 7 days
						}
					}
				}
			]
		}
		})
	],
	resolve: {
		conditions: ['browser']
	},
	server: {
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				changeOrigin: true
			}
		}
	},
	test: {
		environment: 'jsdom',
		globals: true,
		include: ['src/**/*.{test,spec}.{js,ts}']
	}
});