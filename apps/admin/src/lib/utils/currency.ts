// Currency name/symbol helpers driven by the platform currency table
// (Section 58, ADR-065). loadCurrencies() fetches the active list once per
// session from the public GET /api/v1/currencies reference endpoint.
import { api } from '$lib/api';

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
 * Fetch of the active currency list, cached for the session.
 * Returns [] on failure — callers must degrade to the raw ISO code.
 */
export function loadCurrencies(): Promise<CurrencyInfo[]> {
	if (cached) return Promise.resolve(cached);
	if (!inflight) {
		inflight = api<CurrencyInfo[]>('/currencies')
			.then((r) => (cached = r.ok && Array.isArray(r.data) ? r.data : []))
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

/**
 * Display symbol for a code: platform symbol, then Intl's narrow symbol,
 * then the code itself. Always returns something printable.
 */
export function currencySymbol(code: string | null | undefined, currencies?: CurrencyInfo[] | null): string {
	if (!code) return '';
	const hit = currencyOf(code, currencies);
	if (hit?.symbol) return hit.symbol;
	try {
		const part = new Intl.NumberFormat(undefined, {
			style: 'currency',
			currency: code.trim().toUpperCase(),
			currencyDisplay: 'narrowSymbol'
		})
			.formatToParts(0)
			.find((p) => p.type === 'currency');
		return part?.value ?? code;
	} catch {
		return code;
	}
}

/** Display name for a code: platform name, then Intl's display name, then the code. */
export function currencyName(code: string | null | undefined, currencies?: CurrencyInfo[] | null): string {
	if (!code) return '';
	const hit = currencyOf(code, currencies);
	if (hit?.name) return hit.name;
	try {
		return (
			new Intl.DisplayNames(undefined, { type: 'currency' }).of(code.trim().toUpperCase()) ?? code
		);
	} catch {
		return code;
	}
}

/** Select-label form: "NGN — Nigerian Naira". */
export function currencyLabel(code: string, currencies?: CurrencyInfo[] | null): string {
	const name = currencyName(code, currencies);
	return name && name !== code ? `${code} — ${name}` : code;
}

/** Select options for FieldDef type:'select' fields. */
export function currencyOptions(currencies: CurrencyInfo[] | null | undefined): Array<{ value: string; label: string }> {
	return (currencies ?? []).map((c) => ({ value: c.code, label: currencyLabel(c.code, currencies) }));
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
	const num = major.toLocaleString(opts.locale, {
		minimumFractionDigits: digits,
		maximumFractionDigits: digits
	});
	if (!code) return num;
	const c = code.trim().toUpperCase();
	const sym = currencySymbol(c, currencies);
	const prefix = sym && sym !== c ? sym : `${c} `;
	return `${prefix}${num}${opts.withCode ? ` ${c}` : ''}`;
}
