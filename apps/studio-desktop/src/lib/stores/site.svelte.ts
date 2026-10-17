/** Configured site name (general.site_name) of the connected instance —
 * fetched from its public settings endpoint. Never hardcode the brand. */
export const DEFAULT_SITE_NAME = 'Japa.to';

let name = $state(DEFAULT_SITE_NAME);
let fetchedFor = '';

export function siteName(): string {
	return name;
}

/** Fetch /site/settings/public from the configured instance base URL. */
export async function initSiteSettings(baseUrl: string): Promise<void> {
	if (!baseUrl || fetchedFor === baseUrl) return;
	fetchedFor = baseUrl;
	try {
		const res = await fetch(`${baseUrl.replace(/\/$/, '')}/api/v1/site/settings/public`);
		if (!res.ok) return;
		const body = await res.json();
		const s = (body?.data ?? body)?.['general.site_name'];
		if (typeof s === 'string' && s) name = s;
	} catch {
		// keep default on failure
	}
}
