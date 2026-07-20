export const DISCOUNT_TYPES = [
	{ value: 'percentage', label: 'Percentage' },
	{ value: 'fixed', label: 'Fixed Amount' }
] as const;

export type DiscountType = (typeof DISCOUNT_TYPES)[number]['value'];
