// Currency name/symbol helpers driven by the platform currency table
// (Section 58, ADR-065). The active list is loaded once from the public
// GET /api/v1/currencies endpoint — server pages receive it via the root
// layout (page.data.currencies), client components via loadCurrencies().
import { api } from '#lib/api/client.js';

export interface CurrencyInfo {
	code: string;
	name: string;
	symbol: string;
	credits_per_unit?: number;
	position?: number;
	is_active?: boolean;
}

let cached: CurrencyInfo[] | null = null;
let inflight: Promise<CurrencyInfo[]> | null = null;

/**
 * Client-side fetch of the active currency list, cached for the session.
 * Returns [] on failure — callers must degrade to the raw ISO code.
 */
export function loadCurrencies(): Promise<CurrencyInfo[]> {
	if (cached) return Promise.resolve(cached);
	if (!inflight) {
		inflight = api
			.get<CurrencyInfo[]>('/currencies')
			.then((r) => (cached = r.data ?? []))
			.catch(() => [] as CurrencyInfo[])
			.finally(() => {
				inflight = null;
			});
	}
	return inflight;
}

/** Look up a currency record by ISO code (case-insensitive). */
export function currencyOf(code: string | null | undefined, currencies: CurrencyInfo[] | null | undefined): CurrencyInfo | null {
	if (!code || !currencies) return null;
	const c = code.trim().toUpperCase();
	return currencies.find((x) => x.code.toUpperCase() === c) ?? null;
}

// Intl constructors are expensive (~0.5ms each under CPU throttle) and the
// guest landing formats dozens of amounts per card list — memoize instances
// and resolved symbols/names per ISO code.
const currencySymbolCache = new Map<string, string>();
const currencyNameCache = new Map<string, string>();
const amountFormatterCache = new Map<string, Intl.NumberFormat>();
let currencyDisplayNames: Intl.DisplayNames | null | undefined;

function amountFormatter(locale: string | undefined, digits: number): Intl.NumberFormat {
	const key = `${locale ?? ''}|${digits}`;
	let nf = amountFormatterCache.get(key);
	if (!nf) {
		nf = new Intl.NumberFormat(locale, { minimumFractionDigits: digits, maximumFractionDigits: digits });
		amountFormatterCache.set(key, nf);
	}
	return nf;
}

/**
 * Display symbol for a code: platform symbol, then Intl's narrow symbol,
 * then the code itself. Always returns something printable.
 */
export function currencySymbol(code: string | null | undefined, currencies?: CurrencyInfo[] | null): string {
	if (!code) return '';
	const hit = currencyOf(code, currencies);
	if (hit?.symbol) return hit.symbol;
	const c = code.trim().toUpperCase();
	const cachedSym = currencySymbolCache.get(c);
	if (cachedSym !== undefined) return cachedSym;
	let sym = c;
	try {
		const part = new Intl.NumberFormat(undefined, {
			style: 'currency',
			currency: c,
			currencyDisplay: 'narrowSymbol'
		})
			.formatToParts(0)
			.find((p) => p.type === 'currency');
		sym = part?.value ?? c;
	} catch {
		sym = c;
	}
	currencySymbolCache.set(c, sym);
	return sym;
}

/** Display name for a code: platform name, then Intl's display name, then the code. */
export function currencyName(code: string | null | undefined, currencies?: CurrencyInfo[] | null): string {
	if (!code) return '';
	const hit = currencyOf(code, currencies);
	if (hit?.name) return hit.name;
	const c = code.trim().toUpperCase();
	const cachedName = currencyNameCache.get(c);
	if (cachedName !== undefined) return cachedName;
	let name = c;
	try {
		currencyDisplayNames ??= new Intl.DisplayNames(undefined, { type: 'currency' });
		name = currencyDisplayNames.of(c) ?? c;
	} catch {
		name = c;
	}
	currencyNameCache.set(c, name);
	return name;
}

/** Select-label form: "NGN — Nigerian Naira". */
export function currencyLabel(code: string, currencies?: CurrencyInfo[] | null): string {
	const name = currencyName(code, currencies);
	return name && name !== code ? `${code} — ${name}` : code;
}

export interface FormatMoneyOptions {
	/** amount is in minor units (kobo/cents) — divide by 100 before display. */
	minor?: boolean;
	/** BCP-47 locale for grouping; defaults to the runtime locale. */
	locale?: string;
	/** Fraction digits (default 2, or 0 when the amount is integral). */
	fractionDigits?: number;
	/** Show the ISO code after the amount ("₦1,400 NGN") — useful where the
	 *  bare symbol is ambiguous. */
	withCode?: boolean;
}

/**
 * Format an amount with its currency symbol — e.g. 140000 NGN minor →
 * "₦1,400.00". Falls back to "NGN 1,400.00" when the symbol is unknown and
 * to the plain number when no code is given.
 */
export function formatMoney(
	amount: number | null | undefined,
	code: string | null | undefined,
	currencies?: CurrencyInfo[] | null,
	opts: FormatMoneyOptions = {}
): string {
	if (amount === null || amount === undefined || Number.isNaN(amount)) return '';
	const major = opts.minor ? amount / 100 : amount;
	const digits = opts.fractionDigits ?? (Number.isInteger(major) ? 0 : 2);
	const num = amountFormatter(opts.locale, digits).format(major);
	if (!code) return num;
	const c = code.trim().toUpperCase();
	const sym = currencySymbol(c, currencies);
	const prefix = sym && sym !== c ? sym : `${c} `;
	return `${prefix}${num}${opts.withCode ? ` ${c}` : ''}`;
}
