// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
// Shared display formatting for community field values — resolves stored
// select/multiselect keys to their option labels, formats numbers in the
// request-scoped locale, and unwraps {label} objects. Used by
// EntityFieldsPanel (body) and WikiQuickFacts (sidebar) so both stay in sync.
import { i18n, t } from '$lib/i18n';
import type { WikiEntityField } from '$lib/api/wiki';
import { currencySymbol, formatMoney, type CurrencyInfo } from '$lib/utils/currency';

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

/** §58 — the ISO code a price_range/currency field was bound to via its
 *  options config ({currency_code: "USD"}). Empty when unset. */
export function fieldCurrencyCode(options: unknown): string {
	if (options !== null && typeof options === 'object' && !Array.isArray(options)) {
		const c = (options as Record<string, unknown>).currency_code;
		if (typeof c === 'string') return c;
	}
	return '';
}

/** Display string for a price_range value — "₦1,200" for a lone avg,
 *  "₦800 – ₦1,600" when bounds are set. Bare numbers when the field
 *  carries no currency_code. */
export function formatPriceRange(
	v: unknown,
	code?: string,
	currencies?: CurrencyInfo[] | null
): string {
	const p = priceRangeParts(v);
	if (!p) return '—';
	const sym = code ? currencySymbol(code, currencies) : '';
	const money = (n: number) => (sym && sym !== code ? `${sym}${fmtNum(n)}` : code ? `${code} ${fmtNum(n)}` : fmtNum(n));
	if (p.min !== undefined && p.max !== undefined && p.min !== p.max) {
		return `${money(p.min)} – ${money(p.max)}`;
	}
	const single = p.avg ?? p.min ?? p.max;
	return single !== undefined ? money(single) : '—';
}

/** §58 — currency display precedence for money-typed fields: the field's
 *  explicit {currency_code} option wins; otherwise the host space/entity's
 *  resolved country currency (space_currency_code / entity_currency_code);
 *  otherwise bare numbers. */
export function formatWikiFieldValue(
	f: WikiEntityField,
	currencies?: CurrencyInfo[] | null,
	contextCurrencyCode?: string
): string {
	const v = f.value;
	if (v === null || v === undefined || v === '') return '—';
	const code = fieldCurrencyCode(f.options) || contextCurrencyCode || '';
	if (f.field_type === 'price_range') return formatPriceRange(v, code, currencies);
	if (f.field_type === 'currency' && typeof v === 'number') return formatMoney(v, code || undefined, currencies);
	if (typeof v === 'boolean') return v ? t('common.yes', 'Yes') : t('common.no', 'No');
	if (typeof v === 'number') return v.toLocaleString(i18n.locale);
	const options = choiceOptions(f.options);
	if (Array.isArray(v)) return v.map((item) => optionLabel(options, item)).join(', ');
	if (options.length > 0) return optionLabel(options, v);
	return objectLabel(v);
}
