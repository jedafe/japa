import adapter from '@sveltejs/adapter-static';
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

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			fallback: 'index.html'
		}),
		// Hash-based CSP for the static SPA — SvelteKit emits a
		// <meta http-equiv> policy with sha256 hashes for its inline
		// bootstrap script. frame-ancestors cannot go in a meta tag; the
		// spa-server sets it as a real header (docker/spa-server.mjs).
		...(dev
			? {}
			: {
					csp: {
						mode: 'hash',
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
							// /analytics embeds the Umami dashboard — the parent-side
							// counterpart to Umami's frame-ancestors.
							'frame-src': ['self', ...(umamiOrigin ? [umamiOrigin] : [])],
							'media-src': ['self', 'https:', 'blob:'],
							'worker-src': ['self'],
							'manifest-src': ['self'],
							'form-action': ['self'],
							'object-src': ['none'],
							'base-uri': ['self']
						}
					}
				})
	}
};

export default config;
