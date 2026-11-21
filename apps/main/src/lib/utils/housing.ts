// §96 Housing Guide — shared view normalizers for the tool's JSONB
// fields (ADR-104). housing_guides.search_platforms stores
// [{name, url, description}] and rent_ranges stores
// {tier: {studio, 1bed, 2bed, currency}}; both pages render the same
// shapes, so the parsing lives here rather than being duplicated.

import { safeHttpUrl } from './safeUrl.js';

/** One search platform entry after normalization. */
export interface HousingPlatform {
	name: string;
	url?: string;
	description?: string;
}

/** One rent-range tier row after normalization. */
export interface HousingRentTier {
	/** Tier key — city_centre | inner | outer (or a community-added key). */
	tier: string;
	/** Free-form range strings per unit type, e.g. "€400–700". */
	studio?: string;
	oneBed?: string;
	twoBed?: string;
	/** ISO currency applied to every range in the row, e.g. "EUR". */
	currency?: string;
	/** Scalar fallback — a community value that isn't the tier object. */
	raw?: string;
}

/** Milliseconds in 90 days — the ADR-104 staleness threshold. */
const STALE_MS = 90 * 24 * 60 * 60 * 1000;

/**
 * ADR-104: data not re-verified within 90 days is stale and must render
 * with a warning. Returns false for a missing date — that case has its
 * own "not yet verified" treatment, not the stale warning.
 */
export function isStaleVerification(lastVerifiedAt?: string | null): boolean {
	if (!lastVerifiedAt) return false;
	const t = new Date(lastVerifiedAt).getTime();
	if (Number.isNaN(t)) return false;
	return Date.now() - t > STALE_MS;
}

function isRecord(v: unknown): v is Record<string, unknown> {
	return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function str(v: unknown): string {
	return typeof v === 'string' ? v.trim() : '';
}

/**
 * Normalize housing_guides.search_platforms into renderable entries.
 * The §96 shape is [{name, url, description}]; string items and legacy
 * key→url object maps are tolerated so a partially-shaped row still
 * renders rather than dropping the section.
 */
export function housingPlatforms(input: unknown): HousingPlatform[] {
	const out: HousingPlatform[] = [];

	const fromObject = (o: Record<string, unknown>) => {
		const name = str(o.name) || str(o.title) || str(o.label);
		// safeHttpUrl: platform rows are admin-entered JSONB with no
		// backend URL validation — a javascript:/data: value must never
		// reach an href (same sink rule as jobs application_url).
		const url = safeHttpUrl(str(o.url) || str(o.link) || str(o.href));
		const description = str(o.description) || str(o.notes) || str(o.note);
		if (name || url) out.push({ name: name || url, url: url || undefined, description: description || undefined });
	};

	if (Array.isArray(input)) {
		for (const item of input) {
			if (typeof item === 'string') {
				const s = item.trim();
				if (s) out.push({ name: s });
			} else if (isRecord(item)) {
				fromObject(item);
			}
		}
		return out;
	}

	if (isRecord(input)) {
		// Legacy {name: url} or {name: {url, description}} map.
		for (const [key, val] of Object.entries(input)) {
			if (typeof val === 'string') {
				out.push({ name: key, url: safeHttpUrl(val) || undefined });
			} else if (isRecord(val)) {
				const url = safeHttpUrl(str(val.url) || str(val.link));
				const description = str(val.description) || str(val.notes);
				out.push({ name: key, url: url || undefined, description: description || undefined });
			}
		}
	}

	return out;
}

/**
 * Normalize housing_guides.rent_ranges into tier rows. The §96 shape is
 * {tier: {studio, 1bed, 2bed, currency}}; a tier whose value is a scalar
 * (community field-value overlay) renders as a raw row instead of being
 * dropped.
 */
export function housingRentTiers(input: unknown): HousingRentTier[] {
	if (!isRecord(input)) return [];

	const out: HousingRentTier[] = [];
	for (const [tier, val] of Object.entries(input)) {
		if (isRecord(val)) {
			const row: HousingRentTier = {
				tier,
				studio: str(val.studio) || undefined,
				oneBed: str(val['1bed']) || str(val.one_bed) || str(val['1_bed']) || undefined,
				twoBed: str(val['2bed']) || str(val.two_bed) || str(val['2_bed']) || undefined,
				currency: str(val.currency) || undefined
			};
			if (row.studio || row.oneBed || row.twoBed || row.currency) {
				out.push(row);
				continue;
			}
		}
		// Scalar or unrecognized object — keep the value visible.
		if (typeof val === 'string' && val.trim()) {
			out.push({ tier, raw: val.trim() });
		} else if (typeof val === 'number') {
			out.push({ tier, raw: String(val) });
		}
	}
	return out;
}

const TIER_LABELS: Record<string, string> = {
	city_centre: 'City centre',
	inner: 'Inner districts',
	outer: 'Outer districts',
	center: 'City centre'
};

/** Human label for a rent-range tier key. */
export function rentTierLabel(tier: string): string {
	return TIER_LABELS[tier] ?? tier.replace(/_/g, ' ');
}

/**
 * §96 accommodation_type enum — single source for the tool's option lists
 * AND value→label mapping. `all` is an enum value ("this guide covers
 * every type"), NOT "no filter" — the filter's empty option is a separate
 * 'any_type' i18n key. Mirrors custom_fields.options for
 * guide_accommodation_fields and the admin page's list.
 */
export const HOUSING_ACCOMMODATION_TYPES = [
	{ value: 'private_rental', labelKey: 'housing_guide.type_private_rental', label: 'Private rental' },
	{ value: 'flatshare', labelKey: 'housing_guide.type_flatshare', label: 'Flatshare' },
	{ value: 'student', labelKey: 'housing_guide.type_student', label: 'Student housing' },
	{ value: 'employer_provided', labelKey: 'housing_guide.type_employer', label: 'Employer-provided' },
	{ value: 'all', labelKey: 'housing_guide.type_all', label: 'All types' }
] as const;

/**
 * Localized display label for a stored accommodation_type. Known values
 * resolve through i18n; unknown legacy values (e.g. 'apartment') fall back
 * to a humanized form rather than leaking the raw slug.
 */
export function housingTypeLabel(
	type: string | null | undefined,
	t: (key: string, fallback: string) => string
): string {
	if (!type) return '';
	const found = HOUSING_ACCOMMODATION_TYPES.find((a) => a.value === type);
	return found ? t(found.labelKey, found.label) : type.replace(/_/g, ' ');
}
