import { browser } from '$app/environment';

// SPA mode — adapter-static emits a fallback index.html shell, so no
// server-side rendering is needed. Every route is handled client-side.
export const ssr = false;

// Universal load replaces the former +layout.server.ts. In a static SPA
// there is no server runtime to populate `locals`, so the auth token is
// read directly from the cookie on the client.
export const load = () => {
	if (!browser) return { token: null, user: null };

	const token =
		document.cookie
			.split('; ')
			.find((c) => c.startsWith('admin_token='))
			?.split('=')[1] ?? null;

	return { token, user: null };
};
