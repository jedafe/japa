/**
 * Shared source-category helpers — the directory ?category= filter and the
 * propose form's category picker must always agree on the option list and
 * its labels.
 */
import { t } from '$lib/i18n';
import type { SourceMeta } from '$lib/types/sources';

/**
 * The canonical source-category list: the union of the curated proposal
 * catalog (meta.categories — carries extras like video/university that are
 * not extraction domains) and the full extraction-domain taxonomy
 * (meta.admin_categories), deduped and sorted.
 */
export function sourceCategoryOptions(meta: SourceMeta | null | undefined): string[] {
	return [...new Set([...(meta?.categories ?? []), ...(meta?.admin_categories ?? [])])].sort();
}

/** Human label for a source category — i18n key with a capitalized fallback. */
export function sourceCategoryLabel(c: string): string {
	const fallback = c.replace(/_/g, ' ');
	return t(`sources.submit.category.${c}`, fallback.charAt(0).toUpperCase() + fallback.slice(1));
}

/**
 * Human label for a source trust level. The numeric level is never shown
 * publicly — 1 is the most trusted (official) and 5 is unverified, so a raw
 * "Trust 5" badge reads inverted to users.
 */
export function sourceTrustLabel(level: number | undefined | null): string {
	const labels: Record<number, [string, string]> = {
		1: ['sources.trust.official', 'Official'],
		2: ['sources.trust.trusted', 'Trusted'],
		3: ['sources.trust.aggregator', 'Aggregator'],
		4: ['sources.trust.community', 'Community'],
		5: ['sources.trust.unverified', 'Unverified']
	};
	const entry = labels[level ?? 0];
	if (!entry) return t('sources.trust.unrated', 'Unrated');
	return t(entry[0], entry[1]);
}
