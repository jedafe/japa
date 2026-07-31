// Discount type enum values — defined by the CHECK constraint on
// discount_codes.discount_type in migration 000211 (JEOS §17).
// These are schema constants, not configurable data.
export const DISCOUNT_TYPES = [
	{ value: 'percent', label: 'Percentage' },
	{ value: 'fixed', label: 'Fixed Amount' }
] as const;

export type DiscountType = (typeof DISCOUNT_TYPES)[number]['value'];
