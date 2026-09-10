export interface ParsedCardStyle {
	default: string;
	byType: Record<string, string>;
}

const STYLES = ['reddit', 'compact', 'grid', 'magazine', 'daily'];

/**
 * Parse the stored feed.card_style value. Accepts:
 *  - undefined/null (no setting) → fallback
 *  - a JSON string (object, legacy scalar, or double-encoded object)
 *  - a plain object {default, by_type} (when the DB returns JSONB as an object)
 */
export function parseCardStyleValue(raw: unknown): ParsedCardStyle {
	const fallback: ParsedCardStyle = { default: 'reddit', byType: {} };
	if (raw == null) return fallback;

	// If the value is already an object (JSONB returned as map[string]any by
	// pgx and serialized as a JSON object in the API response), handle it
	// directly without string round-tripping.
	if (typeof raw === 'object') {
		return parseObject(raw as Record<string, unknown>);
	}

	if (typeof raw !== 'string' || !raw.trim()) return fallback;

	let v: unknown;
	try {
		v = JSON.parse(raw);
	} catch {
		return fallback;
	}

	// Every settings value may be stored as a JSON string, so an object saved
	// from this editor can come back double-encoded: a JSON string whose
	// content is itself object JSON. A bare style scalar stays a scalar;
	// anything else gets one more parse to reach the {default, by_type} object.
	if (typeof v === 'string') {
		if (STYLES.includes(v)) return { default: v, byType: {} };
		try {
			v = JSON.parse(v);
		} catch {
			return fallback;
		}
	}

	if (v && typeof v === 'object') {
		return parseObject(v as Record<string, unknown>);
	}
	return fallback;
}

function parseObject(o: Record<string, unknown>): ParsedCardStyle {
	const def =
		typeof o['default'] === 'string' && STYLES.includes(o['default'] as string)
			? (o['default'] as string)
			: 'reddit';
	const byType: Record<string, string> = {};
	const bt = o['by_type'];
	if (bt && typeof bt === 'object') {
		for (const [k, val] of Object.entries(bt as Record<string, unknown>)) {
			if (typeof val === 'string' && STYLES.includes(val)) byType[k] = val;
		}
	}
	return { default: def, byType };
}

/** Build the JSON string for storage: {default, by_type}, dropping invalid styles. */
export function buildCardStyleValue(def: string, byType: Record<string, string>): string {
	const cleaned: Record<string, string> = {};
	for (const [k, v] of Object.entries(byType)) {
		if (STYLES.includes(v)) cleaned[k] = v;
	}
	return JSON.stringify({ default: STYLES.includes(def) ? def : 'reddit', by_type: cleaned });
}

export const CARD_STYLE_VALUES = ['reddit', 'compact', 'grid', 'magazine', 'daily'] as const;

// Backward-compat: still exported, but labels are i18n keys — consumers should
// translate via t(labelKey, fallback). Prefer CARD_STYLE_VALUES + t() at call site.
export const CARD_STYLE_OPTIONS = [
	{ value: 'reddit', labelKey: 'content_types.style_reddit', label: 'Reddit' },
	{ value: 'compact', labelKey: 'content_types.style_compact', label: 'Compact' },
	{ value: 'grid', labelKey: 'content_types.style_grid', label: 'Grid' },
	{ value: 'magazine', labelKey: 'content_types.style_magazine', label: 'Magazine' },
	{ value: 'daily', labelKey: 'content_types.style_daily', label: 'Daily' }
];
