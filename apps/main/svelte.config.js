import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { loadEnv } from 'vite';

const dev = process.argv.includes('dev') || process.argv.includes('serve');

// PUBLIC_* vars live in .env files — Vite injects them into
// import.meta.env for client code, but they are NOT in process.env when
// this config file is evaluated. loadEnv reads .env/.env.local/
// .env.[mode] so the CSP connect-src origins stay in sync with what the
// bundle actually calls. Real environment variables (CI/deploy) win over
// file values.
const fileEnv = loadEnv(dev ? 'development' : 'production', process.cwd(), 'PUBLIC_');
const env = (key) => process.env[key] ?? fileEnv[key];

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
	envOrigin(env('PUBLIC_JC_API_URL')),
	envOrigin(env('PUBLIC_JC_WS_URL')),
	envOrigin(env('PUBLIC_JC_UMAMI_URL')),
	envOrigin(env('PUBLIC_JC_GLITCHTIP_DSN'))
].filter(Boolean);
const umamiOrigin = envOrigin(env('PUBLIC_JC_UMAMI_URL'));

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
							'media-src': ['self', 'https:', 'blob:', 'data:'],
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
