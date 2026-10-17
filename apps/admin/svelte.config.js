import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

const dev = process.argv.includes('dev');

// Origins the browser legitimately connects to (API, realtime WS, Umami,
// GlitchTip/Sentry). PUBLIC_* vars are build-time constants — CSP origins
// derived from them cannot drift from what the client bundle calls.
const envOrigin = (v) => {
	try {
		const u = new URL(v ?? '');
		return /^https?:|^wss?:/.test(u.protocol) ? `${u.protocol}//${u.host}` : '';
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

/**
 * SvelteKit configuration for the admin panel.
 *
 * Uses adapter-node so that hooks.server.ts runs per-request in production,
 * enabling proper httpOnly cookie-based auth guards and server-side logout.
 *
 * @type {import('@sveltejs/kit').Config}
 */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter(),
		// Nonce-based CSP in production; disabled in dev where Vite injects
		// unnonced runtime <style> tags. frame-ancestors 'self' only — a
		// privileged panel must never be embedded cross-origin (clickjacking).
		...(dev
			? {}
			: {
					csp: {
						mode: 'nonce',
						directives: {
							'default-src': ['self'],
							'script-src': ['self', ...(umamiOrigin ? [umamiOrigin] : [])],
							'style-src': ['self', 'unsafe-inline'],
							'img-src': ['self', 'data:', 'blob:', 'https:'],
							'font-src': ['self', 'data:'],
							'connect-src': ['self', ...connectOrigins],
							'media-src': ['self', 'https:', 'blob:'],
							'worker-src': ['self'],
							'manifest-src': ['self'],
							'form-action': ['self'],
							'object-src': ['none'],
							'base-uri': ['self'],
							'frame-ancestors': ['self']
						}
					}
				})
	}
};

export default config;
