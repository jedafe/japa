/**
 * Shared state + payload builders for the deal (coupon) form.
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
 *
 * Consumed by DealForm.svelte for staff create/edit (admin coupon routes)
 * and organization deal submissions (POST /organizations/{id}/deals).
 * Kept free of Svelte/DOM imports so the payload contract is unit-testable.
 */
import type { Coupon, DealPartnerRef } from '$lib/types';

/**
 * Props contract for DealForm.svelte — consumed by the staff create/edit
 * pages and the organization submit page (§55 deals redesign).
 */
export interface DealFormProps {
	mode: 'create' | 'edit';
	audience: 'staff' | 'org';
	/** Edit-mode seed row. */
	coupon?: Coupon;
	/** Staff: provider select options (GET /deals/partners). */
	providers?: DealPartnerRef[];
	/** Staff: enables the "+ New provider" inline block. */
	canManageProviders?: boolean;
	/** Org audience: target organization id for POST /organizations/{id}/deals. */
	orgId?: string;
	/** Optional currency datalist options (page data); falls back to a plain input. */
	currencies?: { code: string; name: string; symbol?: string }[];
	onsaved?: (coupon: Coupon) => void;
	oncancel?: () => void;
}

/** Editable shape of coupons.details (JSONB) inside the form. */
export interface DealDetailsValues {
	summary: string;
	trial: { title: string; body: string; url: string };
	sections: { title: string; body: string }[];
	faq: { q: string; a: string }[];
}

/**
 * Flat form state for a coupon. Date fields hold `datetime-local` values
 * (YYYY-MM-DDTHH:mm); buildCouponPayload converts them to RFC3339.
 */
export interface DealFormValues {
	provider_id: string;
	title: string;
	slug: string;
	code: string;
	/** Staff lifecycle control — 'active' | 'suspended' (pending/expired are server-managed). */
	status: string;
	description: string;
	discount_type: string;
	/**
	 * Number input binding — Svelte 5 coerces `type="number"` inputs to
	 * `number` (or `null` when empty), so the field accepts both. Converted
	 * to a JS number on submit.
	 */
	discount_value: string | number | null;
	currency: string;
	terms_url: string;
	referral_url: string;
	is_verified: boolean;
	valid_from: string;
	/** Maps to CreateCouponInput.valid_until on the wire. */
	expires_at: string;
	details: DealDetailsValues;
}

/** Blank details editor state. */
export function emptyDetails(): DealDetailsValues {
	return {
		summary: '',
		trial: { title: '', body: '', url: '' },
		sections: [],
		faq: []
	};
}

/** Blank form state for create mode. */
export function emptyDealFormValues(): DealFormValues {
	return {
		provider_id: '',
		title: '',
		slug: '',
		code: '',
		status: 'active',
		description: '',
		discount_type: '',
		discount_value: '',
		currency: '',
		terms_url: '',
		referral_url: '',
		is_verified: false,
		valid_from: '',
		expires_at: '',
		details: emptyDetails()
	};
}

/** Maps a coupon's details JSONB back into the editor shape (edit mode). */
export function detailsFromCoupon(coupon: Coupon): DealDetailsValues {
	const d = coupon.details;
	if (!d) return emptyDetails();
	return {
		summary: d.summary ?? '',
		trial: {
			title: d.trial?.title ?? '',
			body: d.trial?.body ?? '',
			url: d.trial?.url ?? ''
		},
		sections: (d.sections ?? []).map((s) => ({ title: s.title ?? '', body: s.body ?? '' })),
		faq: (d.faq ?? []).map((f) => ({ q: f.q ?? '', a: f.a ?? '' }))
	};
}

/** RFC3339/ISO timestamp → `datetime-local` input value (YYYY-MM-DDTHH:mm). */
export function toDateTimeLocal(iso: string | null | undefined): string {
	if (!iso) return '';
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** `datetime-local` input value → RFC3339 ISO string; '' when empty/invalid. */
export function fromDateTimeLocal(v: string): string {
	if (!v) return '';
	const d = new Date(v);
	return Number.isNaN(d.getTime()) ? '' : d.toISOString();
}

/** Seeds edit-mode form values from an existing coupon row. */
export function couponToFormValues(coupon: Coupon): DealFormValues {
	const v = emptyDealFormValues();
	v.provider_id = coupon.provider_id ?? '';
	v.title = coupon.title ?? '';
	v.slug = coupon.slug ?? '';
	v.code = coupon.code ?? '';
	v.status = coupon.status ?? 'active';
	v.description = coupon.description ?? '';
	v.discount_type = coupon.discount_type ?? '';
	v.discount_value =
		coupon.discount_value === null || coupon.discount_value === undefined
			? ''
			: String(coupon.discount_value);
	v.currency = coupon.currency ?? '';
	v.terms_url = coupon.terms_url ?? '';
	v.referral_url = coupon.referral_url ?? '';
	v.is_verified = coupon.is_verified ?? false;
	v.valid_from = toDateTimeLocal(coupon.valid_from);
	v.expires_at = toDateTimeLocal(coupon.expires_at);
	v.details = detailsFromCoupon(coupon);
	return v;
}

/**
 * Builds the POST/PATCH body matching `CreateCouponInput` json tags
 * (internal/coupons/service.go). Empty optional strings are omitted so a
 * PATCH never blanks a column unintentionally at the Go layer; expires_at
 * (form) → valid_until (wire).
 *
 * Org submissions (POST /organizations/{id}/deals → SubmitDealInput) share the
 * same input struct but the server forces status='pending' and derives the
 * provider, so staff-only keys are stripped here.
 */
export function buildCouponPayload(
	v: DealFormValues,
	audience: 'staff' | 'org'
): Record<string, unknown> {
	const payload: Record<string, unknown> = {
		title: v.title.trim(),
		coupon_type: 'external'
	};

	const str = (key: string, value: string) => {
		const trimmed = value.trim();
		if (trimmed) payload[key] = trimmed;
	};

	str('slug', v.slug);
	str('code', v.code);
	str('description', v.description);
	str('discount_type', v.discount_type);
	str('currency', v.currency);
	str('terms_url', v.terms_url);
	str('referral_url', v.referral_url);

	// number inputs bind as number|null in Svelte 5 — coerce defensively so a
	// null after clearing the field can't throw.
	const dv = v.discount_value;
	if (dv !== null && dv !== undefined && String(dv).trim() !== '') {
		const n = Number(dv);
		if (Number.isFinite(n)) payload.discount_value = n;
	}

	const validFrom = fromDateTimeLocal(v.valid_from);
	if (validFrom) payload.valid_from = validFrom;
	const validUntil = fromDateTimeLocal(v.expires_at);
	if (validUntil) payload.valid_until = validUntil;

	const details = buildDetails(v.details);
	if (details) payload.details = details;

	// Staff-only controls — the org submit endpoint ignores/overrides these.
	if (audience === 'staff') {
		if (v.provider_id.trim()) payload.provider_id = v.provider_id.trim();
		if (v.status.trim()) payload.status = v.status.trim();
		payload.is_verified = v.is_verified;
	}

	return payload;
}

/**
 * Compacts the details editor state into the coupons.details JSONB shape —
 * `{summary?, trial?, sections?, faq?}` with empty parts dropped. Returns
 * undefined when nothing is filled so `details` is omitted from the payload.
 */
export function buildDetails(d: DealDetailsValues): Record<string, unknown> | undefined {
	const out: Record<string, unknown> = {};

	const summary = d.summary.trim();
	if (summary) out.summary = summary;

	const trial: Record<string, string> = {};
	if (d.trial.title.trim()) trial.title = d.trial.title.trim();
	if (d.trial.body.trim()) trial.body = d.trial.body.trim();
	if (d.trial.url.trim()) trial.url = d.trial.url.trim();
	if (Object.keys(trial).length > 0) out.trial = trial;

	const sections = d.sections
		.map((s) => ({ title: s.title.trim(), body: s.body.trim() }))
		.filter((s) => s.title || s.body);
	if (sections.length > 0) out.sections = sections;

	const faq = d.faq
		.map((f) => ({ q: f.q.trim(), a: f.a.trim() }))
		.filter((f) => f.q || f.a);
	if (faq.length > 0) out.faq = faq;

	return Object.keys(out).length > 0 ? out : undefined;
}

/**
 * Editable shape for the inline "+ New provider" block (staff only,
 * gated by the caller via canManageProviders).
 */
export interface NewProviderValues {
	name: string;
	slug: string;
	category: string;
	base_url: string;
	description: string;
}

export function emptyNewProviderValues(): NewProviderValues {
	return { name: '', slug: '', category: '', base_url: '', description: '' };
}

/**
 * Body for POST /admin/affiliate-providers — keys match the json tags on
 * `CreateProviderInput` (internal/coupons/service.go). Deal partners created
 * inline are coupon providers (§55); the list endpoints surface all active
 * providers regardless of type.
 */
export function buildProviderPayload(v: NewProviderValues): Record<string, unknown> {
	const payload: Record<string, unknown> = {
		name: v.name.trim(),
		slug: v.slug.trim(),
		provider_type: 'coupon'
	};
	if (v.description.trim()) payload.description = v.description.trim();
	if (v.category.trim()) payload.category = v.category.trim();
	if (v.base_url.trim()) payload.base_url = v.base_url.trim();
	return payload;
}
