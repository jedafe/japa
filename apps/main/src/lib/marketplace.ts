// Ref: japaconnect-engineering/sections/114-services-marketplace.md
// Shared display helpers for the services marketplace surfaces — category
// accent mapping, "From $X" card pricing, and coupon discount labels.
// Mirrors the backend DTO shapes (internal/servicedir Option, ListingCoupon,
// PackageTierDTO) so every surface reads the same contract.

import { t } from '$lib/i18n';
import { formatMoney, type CurrencyInfo } from '$lib/utils/currency';
import type { ListingCoupon, ServiceCategoryInfo, ServiceListing } from '$lib/types';

/**
 * The fixed accent palette admins pick from for service categories
 * (§114: "token-mapped palette list (admin picks from palette, not free
 * hex)"). Anything outside this list renders as slate.
 */
export type CategoryAccent =
	| 'violet'
	| 'green'
	| 'amber'
	| 'blue'
	| 'sky'
	| 'emerald'
	| 'rose'
	| 'orange'
	| 'slate'
	| 'indigo'
	| 'gray'
	| 'cyan';

// Class strings must stay literal — Tailwind's scanner picks them up from
// this file; dynamically concatenated names would be purged from the build.
const ACCENT_CLASSES: Record<CategoryAccent, { chip: string; icon: string }> = {
	violet: { chip: 'bg-violet-500/10 text-violet-700 border-violet-500/30', icon: 'text-violet-600' },
	green: { chip: 'bg-green-500/10 text-green-700 border-green-500/30', icon: 'text-green-600' },
	amber: { chip: 'bg-amber-500/10 text-amber-700 border-amber-500/30', icon: 'text-amber-600' },
	blue: { chip: 'bg-blue-500/10 text-blue-700 border-blue-500/30', icon: 'text-blue-600' },
	sky: { chip: 'bg-sky-500/10 text-sky-700 border-sky-500/30', icon: 'text-sky-600' },
	emerald: { chip: 'bg-emerald-500/10 text-emerald-700 border-emerald-500/30', icon: 'text-emerald-600' },
	rose: { chip: 'bg-rose-500/10 text-rose-700 border-rose-500/30', icon: 'text-rose-600' },
	orange: { chip: 'bg-orange-500/10 text-orange-700 border-orange-500/30', icon: 'text-orange-600' },
	slate: { chip: 'bg-slate-500/10 text-slate-700 border-slate-500/30', icon: 'text-slate-600' },
	indigo: { chip: 'bg-indigo-500/10 text-indigo-700 border-indigo-500/30', icon: 'text-indigo-600' },
	gray: { chip: 'bg-gray-500/10 text-gray-700 border-gray-500/30', icon: 'text-gray-600' },
	cyan: { chip: 'bg-cyan-500/10 text-cyan-700 border-cyan-500/30', icon: 'text-cyan-600' }
};

/**
 * Chip + icon classes for a category `accent_color` value. Unknown or
 * absent accents fall back to slate so admin-supplied values can never
 * break the palette constraint.
 */
export function accentClasses(accent: string | undefined): { chip: string; icon: string } {
	return ACCENT_CLASSES[accent as CategoryAccent] ?? ACCENT_CLASSES.slate;
}

/** Look up a category record by slug; undefined when absent or unlisted. */
export function serviceCategoryInfo(
	cats: ServiceCategoryInfo[],
	slug: string | undefined
): ServiceCategoryInfo | undefined {
	if (!slug) return undefined;
	return cats.find((c) => c.value === slug);
}

// price_per unit labels reuse the §111 package keys so the suffix stays
// translated without new strings.
const PRICE_PER_KEYS: Record<string, [string, string]> = {
	person: ['services.pkg_per_person', 'per person'],
	package: ['services.pkg_per_package', 'per package'],
	group: ['services.pkg_per_group', 'per group']
};

/**
 * The "From $X" price line for a listing card/detail hero: the lowest
 * active tier price (min_price) falling back to price_amount — both minor
 * units, matching the server-side fallback order. Appends the translated
 * per-unit suffix for package listings ("per person" etc.). Returns null
 * when the listing carries no price at all.
 */
export function priceLabel(listing: ServiceListing, currencies: CurrencyInfo[]): string | null {
	const amount = listing.min_price ?? listing.price_amount;
	const money = formatMoney(amount, listing.price_currency, currencies, { minor: true });
	if (!money) return null;
	const suffix = listing.price_per ? PRICE_PER_KEYS[listing.price_per] : undefined;
	return suffix ? `${money} ${t(suffix[0], suffix[1])}` : money;
}

/**
 * Short discount label for a coupon badge: "-10%" for percent coupons,
 * "-₦250" for fixed ones (discount_value is minor units in the coupon's
 * currency). Unknown types render empty so callers can skip the badge.
 */
export function discountLabel(c: ListingCoupon, currencies: CurrencyInfo[]): string {
	if (c.discount_type === 'percent') return `-${c.discount_value}%`;
	if (c.discount_type === 'fixed') {
		const money = formatMoney(c.discount_value, c.currency, currencies, { minor: true });
		return money ? `-${money}` : '';
	}
	return '';
}
