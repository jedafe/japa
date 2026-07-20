export type CardStyle = 'reddit' | 'compact' | 'grid' | 'magazine';

export interface CardStyleConfig {
	default: CardStyle;
	byType: Partial<Record<string, CardStyle>>;
}

const STYLES: CardStyle[] = ['reddit', 'compact', 'grid', 'magazine'];

function isStyle(v: unknown): v is CardStyle {
	return typeof v === 'string' && (STYLES as string[]).includes(v);
}

/** Normalize the /feed/card-style payload (object, legacy scalar, or junk) into a config. */
export function normalizeConfig(raw: unknown): CardStyleConfig {
	const fallback: CardStyleConfig = { default: 'reddit', byType: {} };
	if (!raw) return fallback;
	if (typeof raw === 'string') return { default: isStyle(raw) ? raw : 'reddit', byType: {} };
	if (typeof raw !== 'object') return fallback;

	const r = raw as { default?: unknown; by_type?: unknown };
	const byType: Partial<Record<string, CardStyle>> = {};
	if (r.by_type && typeof r.by_type === 'object') {
		for (const [k, v] of Object.entries(r.by_type as Record<string, unknown>)) {
			if (isStyle(v)) byType[k] = v;
		}
	}
	return { default: isStyle(r.default) ? r.default : 'reddit', byType };
}

/** The style for a given content type: its override, else the default. */
export function cardStyleFor(contentType: string, cfg: CardStyleConfig): CardStyle {
	return cfg.byType[contentType] ?? cfg.default;
}

/** True only when the page is non-empty and every content type resolves to grid. */
export function gridMode(contentTypes: string[], cfg: CardStyleConfig): boolean {
	return contentTypes.length > 0 && contentTypes.every((ct) => cardStyleFor(ct, cfg) === 'grid');
}
