import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { buildWebManifest } from '@japaconnect/ui/pwa';

// §124 — dynamic web app manifest composed from public system_settings
// (pwa.* / general.* / appearance.*), so Admin → Settings → Progressive Web
// App changes take effect without a redeploy.
export const GET: RequestHandler = (event) => {
	const manifest = buildWebManifest(event.locals.siteSettings ?? {}, {
		defaultStartUrl: '/',
		includeShareTarget: true
	});
	return json(manifest, {
		headers: {
			'Content-Type': 'application/manifest+json',
			'Cache-Control': 'public, max-age=300'
		}
	});
};
