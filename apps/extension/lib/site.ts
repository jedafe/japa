import { SITE_ORIGIN } from './links';

/** Default brand when general.site_name can't be fetched. */
export const DEFAULT_SITE_NAME = 'Japa.to';

let cached = '';
let inflight: Promise<string> | null = null;

/** Configured site name from GET /site/settings/public, cached per session. */
export function siteName(): Promise<string> {
	if (cached) return Promise.resolve(cached);
	inflight ??= fetch(`${SITE_ORIGIN}/api/v1/site/settings/public`)
		.then((r) => (r.ok ? r.json() : null))
		.then((b) => {
			const s = (b?.data ?? b)?.['general.site_name'];
			cached = typeof s === 'string' && s ? s : DEFAULT_SITE_NAME;
			return cached;
		})
		.catch(() => DEFAULT_SITE_NAME);
	return inflight;
}
