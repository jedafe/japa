/**
 * Convert a 2-letter country code to its regional-indicator emoji flag.
 * Handles common non-ISO and legacy aliases (e.g. "uk" -> "gb" for the United Kingdom).
 *
 * Mappings:
 * - EU reserved / legacy codes that have a current ISO equivalent
 * - Common dataset anomalies (e.g. EU data sometimes uses EL for Greece, TP for Timor-Leste)
 */
export const COUNTRY_CODE_ALIASES: Record<string, string> = {
	uk: 'gb', // United Kingdom ISO 3166-1 alpha-2 code is GB
	el: 'gr', // Greece in EU/vehicle registries
	tp: 'tl', // Timor-Leste (old code)
	bu: 'mm', // Myanmar / Burma (old code)
	fx: 'fr', // Metropolitan France
	su: 'ru', // USSR -> Russian Federation
	yd: 'ye', // Democratic Yemen -> Yemen
	zr: 'cd', // Zaire -> DR Congo
	dd: 'de', // East Germany -> Germany
	// XK (Kosovo) is intentionally not remapped: it is a user-assigned code and
	// some platforms render 🇽🇰 as the Kosovo flag while others fall back to the
	// two letters, which is the expected behaviour for an unassigned ISO pair.
};

export function countryCodeToEmoji(code: string | undefined): string | null {
	if (!code || !/^[a-zA-Z]{2}$/.test(code)) return null;
	const normalized = COUNTRY_CODE_ALIASES[code.toLowerCase()] ?? code;
	const points = [...normalized.toUpperCase()].map((c) => 127397 + c.charCodeAt(0));
	return String.fromCodePoint(...points);
}
