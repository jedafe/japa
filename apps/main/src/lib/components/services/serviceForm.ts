// Ref: japaconnect-engineering/sections/114-services-marketplace.md
// Shared form-shape helpers for the service create/edit surfaces — the
// pricing-tier editor and the coupon-attach selector serialize into the
// TierInput / coupon_ids contracts the servicedir backend expects.

/** Editable shape for one pricing tier in the service forms. Fields stay
 *  strings as typed; tiersToPayload converts to the API contract. */
export interface EditableTier {
	name: string;
	badge: string;
	description: string;
	/** Kebab-case Lucide name from the marketplaceIcon whitelist; '' = none. */
	icon: string;
	/** Major units as typed ('' = price unset); ×100 → minor on serialize. */
	priceAmount: string;
	/** ISO currency override; '' inherits the package's currency. */
	currency: string;
	/** One feature per line — serialized to features[]. */
	featuresText: string;
	isFeatured: boolean;
}

/**
 * Serialize the editor rows into the backend TierInput[] contract
 * (internal/servicedir TierInput): position follows array order, a blank
 * or unparseable price serializes to price_amount 0 (the backend requires
 * the field and treats 0 as a valid free tier), blank badge/description
 * stay blank, blank currency inherits the package currency server-side.
 */
export function tiersToPayload(tiers: EditableTier[]): Record<string, unknown>[] {
	return tiers.map((t) => {
		const amountStr = String(t.priceAmount ?? '').trim();
		const major = Number(amountStr);
		const minor = amountStr && !Number.isNaN(major) ? Math.max(0, Math.round(major * 100)) : 0;
		return {
			name: t.name.trim(),
			badge: t.badge.trim(),
			description: t.description.trim(),
			...(t.icon ? { icon: t.icon } : {}),
			price_amount: minor,
			...(t.currency ? { price_currency: t.currency } : {}),
			features: t.featuresText
				.split('\n')
				.map((f) => f.trim())
				.filter(Boolean),
			is_featured: t.isFeatured
		};
	});
}
