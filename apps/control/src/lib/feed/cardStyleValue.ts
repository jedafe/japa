export interface ParsedCardStyle {
	default: string;
	byType: Record<string, string>;
}

const STYLES = ['reddit', 'compact', 'grid', 'magazine'];

/** Parse the stored feed.card_style JSON string (object, legacy scalar, or junk). */
export function parseCardStyleValue(raw: string): ParsedCardStyle {
	const fallback: ParsedCardStyle = { default: 'reddit', byType: {} };
	if (!raw || !raw.trim()) return fallback;

	let v: unknown;
	try {
		v = JSON.parse(raw);
	} catch {
		return fallback;
	}

	// Every settings value is stored as a JSON string, so an object saved from
	// this editor comes back double-encoded: a JSON string whose content is
	// object JSON. A bare style scalar stays a scalar; anything else gets one
	// more parse to reach the {default, by_type} object.
	if (typeof v === 'string') {
		if (STYLES.includes(v)) return { default: v, byType: {} };
		try {
			v = JSON.parse(v);
		} catch {
			return fallback;
		}
	}
	if (v && typeof v === 'object') {
		const o = v as { default?: unknown; by_type?: unknown };
		const def = typeof o.default === 'string' && STYLES.includes(o.default) ? o.default : 'reddit';
		const byType: Record<string, string> = {};
		if (o.by_type && typeof o.by_type === 'object') {
			for (const [k, val] of Object.entries(o.by_type as Record<string, unknown>)) {
				if (typeof val === 'string' && STYLES.includes(val)) byType[k] = val;
			}
		}
		return { default: def, byType };
	}
	return fallback;
}

/** Build the JSON string for storage: {default, by_type}, dropping invalid styles. */
export function buildCardStyleValue(def: string, byType: Record<string, string>): string {
	const cleaned: Record<string, string> = {};
	for (const [k, v] of Object.entries(byType)) {
		if (STYLES.includes(v)) cleaned[k] = v;
	}
	return JSON.stringify({ default: STYLES.includes(def) ? def : 'reddit', by_type: cleaned });
}
