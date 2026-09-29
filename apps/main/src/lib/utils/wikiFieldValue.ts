// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
// Shared display formatting for community field values — resolves stored
// select/multiselect keys to their option labels, formats numbers in the
// request-scoped locale, and unwraps {label} objects. Used by
// EntityFieldsPanel (body) and WikiQuickFacts (sidebar) so both stay in sync.
import { i18n, t } from '$lib/i18n';
import type { WikiEntityField } from '$lib/api/wiki';

type ChoiceOption = string | { value: string; label: string };

function choiceOptions(options: unknown): ChoiceOption[] {
	return Array.isArray(options) ? (options as ChoiceOption[]) : [];
}

function objectLabel(raw: unknown): string {
	if (raw !== null && typeof raw === 'object') {
		const label = (raw as { label?: unknown }).label;
		if (typeof label === 'string') return label;
		return JSON.stringify(raw);
	}
	return String(raw);
}

function optionLabel(options: ChoiceOption[], raw: unknown): string {
	const match = options.find((o) => (typeof o === 'string' ? o === raw : o.value === raw));
	if (typeof match === 'string') return match;
	if (match) return match.label;
	return objectLabel(raw);
}

/** §80 Itemized Pricing — the composite value shape stored by a
 *  `price_range` field: {avg?, min?, max?} in value_json. */
export interface PriceRangeValue {
	avg?: number;
	min?: number;
	max?: number;
}

export function priceRangeParts(v: unknown): PriceRangeValue | null {
	if (v === null || typeof v !== 'object' || Array.isArray(v)) return null;
	const o = v as Record<string, unknown>;
	const num = (x: unknown) => (typeof x === 'number' && Number.isFinite(x) ? x : undefined);
	const parts = { avg: num(o.avg), min: num(o.min), max: num(o.max) };
	return parts.avg !== undefined || parts.min !== undefined || parts.max !== undefined
		? parts
		: null;
}

function fmtNum(n: number): string {
	return n.toLocaleString(i18n.locale, { maximumFractionDigits: 2 });
}

/** Display string for a price_range value — "1,200" for a lone avg,
 *  "800 – 1,600" when bounds are set. */
export function formatPriceRange(v: unknown): string {
	const p = priceRangeParts(v);
	if (!p) return '—';
	if (p.min !== undefined && p.max !== undefined && p.min !== p.max) {
		return `${fmtNum(p.min)} – ${fmtNum(p.max)}`;
	}
	const single = p.avg ?? p.min ?? p.max;
	return single !== undefined ? fmtNum(single) : '—';
}

export function formatWikiFieldValue(f: WikiEntityField): string {
	const v = f.value;
	if (v === null || v === undefined || v === '') return '—';
	if (f.field_type === 'price_range') return formatPriceRange(v);
	if (typeof v === 'boolean') return v ? t('common.yes', 'Yes') : t('common.no', 'No');
	if (typeof v === 'number') return v.toLocaleString(i18n.locale);
	const options = choiceOptions(f.options);
	if (Array.isArray(v)) return v.map((item) => optionLabel(options, item)).join(', ');
	if (options.length > 0) return optionLabel(options, v);
	return objectLabel(v);
}
