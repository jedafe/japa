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

/** §58+§112 — the local→display-base rate context for "(base)"
 *  equivalents; rate = local units per 1 base unit. Absent = local only. */
export interface FxEquiv {
	base: string;
	rate: number;
}

/** "(…)" equivalent in the display base currency — '' when the field's
 *  code IS the base, the rate is unusable, or fx is absent. */
function equivSuffix(
	n: number,
	code: string,
	fx: FxEquiv | undefined,
	currencies?: CurrencyInfo[] | null
): string {
	if (!fx || !code || code === fx.base || !(fx.rate > 0)) return '';
	const sym = currencySymbol(fx.base, currencies);
	const amt = n / fx.rate;
	const inner = sym && sym !== fx.base ? `${sym}${fmtNum(amt)}` : `${fx.base} ${fmtNum(amt)}`;
	return ` (${inner})`;
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
	currencies?: CurrencyInfo[] | null,
	fx?: FxEquiv
): string {
	const p = priceRangeParts(v);
	if (!p) return '—';
	const sym = code ? currencySymbol(code, currencies) : '';
	const money = (n: number) =>
		(sym && sym !== code ? `${sym}${fmtNum(n)}` : code ? `${code} ${fmtNum(n)}` : fmtNum(n)) +
		equivSuffix(n, code ?? '', fx, currencies);
	if (p.min !== undefined && p.max !== undefined && p.min !== p.max) {
		return `${money(p.min)} – ${money(p.max)}`;
	}
	const single = p.avg ?? p.min ?? p.max;
	return single !== undefined ? money(single) : '—';
}

/** §80 Wiki Topic Pages — one column of a `price_parts` field's declared
 *  schema (options.parts[]): the part key stored in value_json.parts, its
 *  display label, and the formatting kind. */
export interface PricePartDef {
	key: string;
	label: string;
	kind?: string;
}

/** The declared part columns of a `price_parts` field — [] when the
 *  options payload doesn't carry the object schema. */
export function pricePartsSchema(options: unknown): PricePartDef[] {
	if (options === null || typeof options !== 'object' || Array.isArray(options)) return [];
	const parts = (options as Record<string, unknown>).parts;
	if (!Array.isArray(parts)) return [];
	return parts.filter(
		(p): p is PricePartDef =>
			p !== null && typeof p === 'object' && typeof (p as PricePartDef).key === 'string'
	);
}

/** The stored part values of a `price_parts` field — the `parts` member
 *  of the canonical {parts: {...}} shape. */
export function pricePartsValue(v: unknown): Record<string, unknown> {
	if (v === null || typeof v !== 'object' || Array.isArray(v)) return {};
	const parts = (v as Record<string, unknown>).parts;
	if (parts === null || typeof parts !== 'object' || Array.isArray(parts)) return {};
	return parts as Record<string, unknown>;
}

/** Format one part value by its declared kind — `money` resolves through
 *  the same currency chain as currency/price_range, `number` localizes,
 *  anything else renders as text. */
export function formatPricePart(
	raw: unknown,
	kind: string | undefined,
	code: string,
	currencies?: CurrencyInfo[] | null,
	fx?: FxEquiv
): string {
	if (raw === null || raw === undefined || raw === '') return '—';
	if (kind === 'money' || kind === 'number') {
		const n = typeof raw === 'number' ? raw : Number(raw);
		if (!Number.isFinite(n)) return '—';
		if (kind !== 'money') return fmtNum(n);
		const sym = code ? currencySymbol(code, currencies) : '';
		const primary = sym && sym !== code ? `${sym}${fmtNum(n)}` : code ? `${code} ${fmtNum(n)}` : fmtNum(n);
		return primary + equivSuffix(n, code, fx, currencies);
	}
	return String(raw);
}

/** Inline `price_parts` render for list/dl contexts — the primary part
 *  alone when the schema declares one, otherwise the parts joined. */
export function formatPricePartsInline(
	f: WikiEntityField,
	contextCurrencyCode: string,
	currencies?: CurrencyInfo[] | null,
	fx?: FxEquiv
): string {
	const parts = pricePartsValue(f.value);
	const schema = pricePartsSchema(f.options);
	const code = fieldCurrencyCode(f.options) || contextCurrencyCode;
	const opts = (f.options ?? {}) as { primary_part?: string };
	const fmt = (key: string, kind?: string) => formatPricePart(parts[key], kind, code, currencies, fx);
	if (opts.primary_part && parts[opts.primary_part] !== undefined) {
		const kind = schema.find((p) => p.key === opts.primary_part)?.kind;
		return fmt(opts.primary_part, kind);
	}
	if (schema.length > 0) {
		const joined = schema
			.filter((p) => parts[p.key] !== undefined)
			.map((p) => fmt(p.key, p.kind))
			.join(' · ');
		if (joined) return joined;
	}
	return '—';
}

/** §80 attractions — one resolved media item inside an
 *  image_single/image_multi/media field value (backend resolves stored
 *  media ids to {id, url, alt} before the DTO leaves the API). */
export interface MediaFieldItem {
	id: string;
	url: string;
	alt?: string;
}

/** True for the media-bearing field types whose values are rendered as
 *  images, not text. */
export function isMediaFieldType(fieldType: string): boolean {
	return fieldType === 'image_single' || fieldType === 'image_multi' || fieldType === 'media';
}

/** The resolved media items of a media field value — accepts the single
 *  object shape (image_single) and the array shape (image_multi/media);
 *  raw/unresolved values (bare ids, unresolved failures) yield [] so the
 *  caller falls back to the text renderer. */
export function mediaFieldItems(v: unknown): MediaFieldItem[] {
	const pick = (x: unknown): MediaFieldItem | null => {
		if (x === null || typeof x !== 'object' || Array.isArray(x)) return null;
		const o = x as Record<string, unknown>;
		if (typeof o.url !== 'string' || o.url === '') return null;
		return {
			id: typeof o.id === 'string' ? o.id : '',
			url: o.url,
			alt: typeof o.alt === 'string' && o.alt !== '' ? o.alt : undefined
		};
	};
	if (Array.isArray(v)) {
		return v.map(pick).filter((m): m is MediaFieldItem => m !== null);
	}
	const single = pick(v);
	return single ? [single] : [];
}

/** §80 — select/multiselect values written outside the ops path may carry
 *  a {"value": x} / {"values": […]} envelope; unwrap to the scalar/array so
 *  option-label resolution still matches. */
function unwrapChoiceValue(v: unknown): unknown {
	if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
		const o = v as Record<string, unknown>;
		if ('values' in o) return o.values;
		if ('value' in o) return o.value;
	}
	return v;
}

/** §58 — currency display precedence for money-typed fields: the field's
 *  explicit {currency_code} option wins; otherwise the host space/entity's
 *  resolved country currency (space_currency_code / entity_currency_code);
 *  otherwise bare numbers. */
export function formatWikiFieldValue(
	f: WikiEntityField,
	currencies?: CurrencyInfo[] | null,
	contextCurrencyCode?: string,
	fx?: FxEquiv
): string {
	let v = f.value;
	if (f.field_type === 'select' || f.field_type === 'multiselect') {
		v = unwrapChoiceValue(v);
	}
	if (v === null || v === undefined || v === '') return '—';
	const code = fieldCurrencyCode(f.options) || contextCurrencyCode || '';
	if (f.field_type === 'price_range') return formatPriceRange(v, code, currencies, fx);
	if (f.field_type === 'price_parts') return formatPricePartsInline(f, code, currencies, fx);
	if (f.field_type === 'currency' && typeof v === 'number')
		return formatMoney(v, code || undefined, currencies) + equivSuffix(v, code, fx, currencies);
	if (typeof v === 'boolean') return v ? t('common.yes', 'Yes') : t('common.no', 'No');
	if (typeof v === 'number') return v.toLocaleString(i18n.locale);
	const options = choiceOptions(f.options);
	if (Array.isArray(v)) return v.map((item) => optionLabel(options, item)).join(', ');
	if (options.length > 0) return optionLabel(options, v);
	return objectLabel(v);
}
