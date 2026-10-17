import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

const dev = process.argv.includes('dev');

// Origins the browser legitimately connects to (API, realtime WS, Umami).
// PUBLIC_* vars are build-time constants — the same values are compiled
// into the client bundle, so CSP origins derived from them cannot drift.
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

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// Learning platform is SSR via adapter-node + Bun (port 3003).
		adapter: adapter(),
		// Nonce-based CSP in production; disabled in dev where Vite injects
		// unnonced runtime <style> tags. frame-ancestors also allows sibling
		// japaconnect surfaces (hub embedding), blocks external framing.
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
							'frame-ancestors': ['self', 'https://japaconnect.com', 'https://*.japaconnect.com']
						}
					}
				})
	}
};

export default config;
