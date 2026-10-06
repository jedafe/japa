// spa-server.mjs — static file server with SPA fallback for adapter-static
// apps (control panel). Bun-native; serves ./app on $PORT.
const root = `${import.meta.dir}/app`;
const port = Number(process.env.PORT ?? 3000);

Bun.serve({
	port,
	async fetch(req) {
		const pathname = decodeURIComponent(new URL(req.url).pathname);
		if (pathname.includes('..')) return new Response('forbidden', { status: 403 });
		const file = Bun.file(`${root}${pathname}`);
		if (await file.exists()) return new Response(file);
		return new Response(Bun.file(`${root}/index.html`));
	},
});

console.log(`spa-server listening on :${port}`);
