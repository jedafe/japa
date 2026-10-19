/**
 * Flag artwork helpers — ISO alpha-2 (geo_code) → bundled SVG flag from
 * `flag-icons` (emitted as a hashed Vite asset, no external CDN calls);
 * the regional-indicator emoji is the fallback for codes without a
 * bundled flag. Shared by the radar country gallery and the wiki
 * destination cards.
 */
const flagModules = import.meta.glob<string>('/node_modules/flag-icons/flags/4x3/*.svg', {
	eager: true,
	query: '?url&no-inline',
	import: 'default'
});

const flagUrls: Record<string, string> = {};
for (const [path, url] of Object.entries(flagModules)) {
	const code = path.split('/').pop()?.slice(0, -4) ?? '';
	if (code) flagUrls[code] = url;
}

export function flagUrl(code: string | null | undefined): string | null {
	const c = code?.trim().toLowerCase();
	return (c && flagUrls[c]) || null;
}

export function flagEmoji(code: string | null | undefined): string {
	const c = code?.trim().toUpperCase();
	return c && /^[A-Z]{2}$/.test(c)
		? String.fromCodePoint(...[...c].map((ch) => 0x1f1e6 + ch.charCodeAt(0) - 65))
		: '';
}
