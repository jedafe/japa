import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

const dev = process.argv.includes('dev');

// Origins the browser legitimately connects to (API, realtime WS, Umami,
// GlitchTip). PUBLIC_* vars are build-time constants — the same values are
// compiled into the client bundle, so deriving CSP origins from them can
// never drift from what the app actually calls.
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
		// Public-facing community app is SSR via adapter-node + Bun (port 3000).
		adapter: adapter(),
		// Nonce-based CSP in production. Disabled in dev: Vite injects
		// unnonced runtime <style> tags and eval'd modules that a strict
		// policy would break. kit.csp is the only mechanism that nonces
		// SvelteKit's inline bootstrap script — a hand-set CSP header
		// cannot reach it and would break hydration.
		// event.locals.cspNonce is populated by SvelteKit here and is
		// consumed by the layout's staff-gated custom CSS/JS blocks.
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
							'frame-ancestors': ['self']
						}
					}
				})
	}
};

export default config;
