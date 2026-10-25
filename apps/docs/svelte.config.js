import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

const dev = process.argv.includes('dev');

// Origins the browser legitimately connects to (API, realtime WS, Umami,
// GlitchTip). PUBLIC_* vars are build-time constants — CSP origins derived
// from them cannot drift from what the client bundle actually calls.
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
		// Developer + user documentation is SSR via adapter-node + Bun (port 3004).
		adapter: adapter(),
		// Nonce-based CSP in production; disabled in dev where Vite injects
		// unnonced runtime <style> tags.
		...(dev
			? {}
			: {
					csp: {
						mode: 'nonce',
						directives: {
							'default-src': ['self'],
							// 'unsafe-hashes' + the sha256 allow exactly one inline
							// handler: `this.__e=event`, emitted by Svelte 5.56+ SSR as
							// onload/onerror on <img> for hydration event replay.
							'script-src': [
								'self',
								'unsafe-hashes',
								'sha256-7dQwUgLau1NFCCGjfn9FsYptB6ZtWxJin6VohGIu20I=',
								...(umamiOrigin ? [umamiOrigin] : [])
							],
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
							'frame-ancestors': ['self', 'https://japa.to', 'https://*.japa.to']
						}
					}
				})
	}
};

export default config;
