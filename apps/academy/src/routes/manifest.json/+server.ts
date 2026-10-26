import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { buildWebManifest } from '@japaconnect/ui/pwa';

// §124 — dynamic web app manifest from public system_settings.
export const GET: RequestHandler = (event) => {
	const manifest = buildWebManifest(event.locals.siteSettings ?? {}, {
		nameSuffix: ' Academy',
		defaultShortName: 'Academy',
		defaultStartUrl: '/'
	});
	return json(manifest, {
		headers: {
			'Content-Type': 'application/manifest+json',
			'Cache-Control': 'public, max-age=300'
		}
	});
};
