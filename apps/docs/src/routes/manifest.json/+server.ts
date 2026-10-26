import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { buildWebManifest } from '@japaconnect/ui/pwa';

// §124 — dynamic web app manifest from public system_settings.
export const GET: RequestHandler = (event) => {
	const manifest = buildWebManifest(event.locals.siteSettings ?? {}, {
		nameSuffix: ' Docs',
		defaultShortName: 'JC Docs',
		defaultStartUrl: '/docs'
	});
	// The docs PWA always launches into the docs tree — pwa.start_url is the
	// main app's setting and must not redirect the docs launcher to '/'.
	manifest.start_url = '/docs';
	return json(manifest, {
		headers: {
			'Content-Type': 'application/manifest+json',
			'Cache-Control': 'public, max-age=300'
		}
	});
};
