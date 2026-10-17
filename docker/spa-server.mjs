// spa-server.mjs — static file server with SPA fallback for adapter-static
// apps (control panel). Bun-native; serves ./app on $PORT.
const root = `${import.meta.dir}/app`;
const port = Number(process.env.PORT ?? 3000);

// Baseline hardening headers for every response. frame-ancestors must be a
// real header (it is ignored inside a <meta> CSP): a privileged ops panel
// must never render inside an off-site frame (clickjacking). The sibling
// *.japa.to allowance matches the other surfaces' frame-ancestors
// policy; the rest of the CSP ships as a hashed <meta> from kit.csp.
const SECURITY_HEADERS = {
	'X-Content-Type-Options': 'nosniff',
	'Referrer-Policy': 'strict-origin-when-cross-origin',
	'Permissions-Policy': 'camera=(), microphone=(), geolocation=(), payment=()',
	'Content-Security-Policy': "frame-ancestors 'self' https://japa.to https://*.japa.to",
};

const withHeaders = (res) => {
	const headers = new Headers(res.headers);
	for (const [k, v] of Object.entries(SECURITY_HEADERS)) headers.set(k, v);
	return new Response(res.body, { status: res.status, statusText: res.statusText, headers });
};

Bun.serve({
	port,
	async fetch(req) {
		const pathname = decodeURIComponent(new URL(req.url).pathname);
		if (pathname.includes('..')) return new Response('forbidden', { status: 403 });
		const file = Bun.file(`${root}${pathname}`);
		if (await file.exists()) return withHeaders(new Response(file));
		return withHeaders(new Response(Bun.file(`${root}/index.html`)));
	},
});

console.log(`spa-server listening on :${port}`);
