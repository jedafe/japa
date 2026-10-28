<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /deals — migrated to the EntityPage scaffold; declarative CRUD entity with date conversion.
	// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import type { ActionContext } from '$lib/components/ui/data-table';
	import { couponsAdminApiExtended, affiliateProvidersAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { CheckCircle, Eye, Pencil, Trash2, XCircle } from 'lucide-svelte';
	import { dateInputToStartOfDayISO, dateInputToEndOfDayISO, isoTimestampToDateInput } from '$lib/utils/date';
	import { loadCurrencies, currencyOptions, formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import { onMount } from 'svelte';

	type Coupon = {
		id: string;
		title: string;
		slug: string;
		code: string | null;
		coupon_type: string;
		provider_id: string | null;
		provider_name?: string | null;
		provider_category?: string | null;
		description: string | null;
		discount_type: string | null;
		discount_value: number | null;
		currency: string | null;
		terms_url: string | null;
		referral_url: string | null;
		is_verified: boolean;
		status: string;
		// load() stringifies details for the JSON textarea, so rows carry the
		// object (API) or a string (form-seeded) — and builds accept either.
		details?: Record<string, unknown> | string | null;
		valid_from?: string;
		expires_at?: string;
		total_clicks: number;
		total_copies: number;
		created_at: string;
	};

	type Provider = {
		id: string;
		name: string;
		slug: string;
		is_active?: boolean;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let providerOptions = $state<Array<{ value: string; label: string }>>([]);
	// §58 — managed currency list: drives the currency select options and
	// symbol rendering for fixed-amount discounts.
	let currencies = $state<CurrencyInfo[]>([]);

	// Load affiliate providers for the provider_id select (Section 55 — provider
	// linkage). `is_active` is intentionally absent from the query so inactive
	// providers (e.g. org providers awaiting approval) still resolve as options.
	onMount(async () => {
		loadCurrencies().then((list) => (currencies = list));
		try {
			const res = await affiliateProvidersAdminApi.list('page_size=100');
			if (res.ok && Array.isArray(res.data)) {
				providerOptions = (res.data as Provider[]).map((p) => ({
					value: p.id,
					label:
						p.is_active === false
							? `${p.name} (${t('common.inactive', 'Inactive')})`
							: p.name
				}));
			}
		} catch {
			// Providers list is best-effort; the select will just be empty.
		}
	});

	// Shared payload builder for create/update/moderation. PATCH UpdateCoupon is
	// a full-column overwrite (internal/db/queries/coupons.sql UpdateCoupon sets
	// every sqlc.narg column from the input), so every payload must carry the
	// complete field set — omitting a key NULLs that column server-side.
	// `details` accepts either the stringified JSON produced by `load` (form
	// seeding / row data) or a raw object.
	//
	// `forModeration` (approve/reject) preserves the row verbatim instead of
	// re-deriving form-shaped values: discount_type passes through as NULL
	// rather than defaulting to 'percent', and valid_from/expires_at re-send
	// the raw ISO timestamps captured in `load` (_valid_from_iso /
	// _expires_at_iso) because the row's date-input values are collapsed to
	// YYYY-MM-DD and re-expanding them would shift real timestamps.
	// `forCreate` uppercases the coupon code (create-time normalization); on
	// update/moderation the stored casing is preserved verbatim so a save
	// can't silently change the displayed code.
	function buildCouponPayload(
		body: Record<string, unknown>,
		opts?: { forModeration?: boolean; forCreate?: boolean }
	): { ok: true; payload: Record<string, unknown> } | { ok: false; error: string } {
		const payload: Record<string, unknown> = {
			title: String(body.title ?? '').trim(),
			code: opts?.forCreate
				? String(body.code ?? '').trim().toUpperCase()
				: String(body.code ?? '').trim(),
			coupon_type: body.coupon_type ?? 'external',
			discount_type: body.discount_type ?? (opts?.forModeration ? null : 'percent'),
			discount_value:
				body.discount_value != null && String(body.discount_value) !== ''
					? Number(body.discount_value)
					: null,
			is_verified: !!body.is_verified,
			status: body.status ?? 'active'
		};
		if (body.details != null && body.details !== '') {
			let parsed: unknown;
			if (typeof body.details === 'object') {
				parsed = body.details;
			} else {
				try {
					parsed = JSON.parse(String(body.details));
				} catch {
					return { ok: false, error: t('deals.details_invalid', 'details must be a JSON object') };
				}
			}
			if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
				return { ok: false, error: t('deals.details_invalid', 'details must be a JSON object') };
			}
			payload.details = parsed;
		}
		if (body.slug) payload.slug = String(body.slug).trim();
		if (body.provider_id) payload.provider_id = String(body.provider_id);
		if (body.currency) payload.currency = String(body.currency).trim().toUpperCase();
		if (body.description) payload.description = String(body.description).trim();
		if (body.referral_url) payload.referral_url = String(body.referral_url).trim();
		if (body.terms_url) payload.terms_url = String(body.terms_url).trim();
		if (opts?.forModeration) {
			if (body._valid_from_iso) payload.valid_from = String(body._valid_from_iso);
			if (body._expires_at_iso) payload.valid_until = String(body._expires_at_iso);
		} else {
			if (body.valid_from) payload.valid_from = dateInputToStartOfDayISO(String(body.valid_from));
			if (body.expires_at) payload.valid_until = dateInputToEndOfDayISO(String(body.expires_at));
		}
		return { ok: true, payload };
	}

	// §55 — pending deals are moderated via PATCH /admin/coupons/{id} status
	// transitions. The PATCH overwrites every column, so the payload is rebuilt
	// from the row via buildCouponPayload with only `status` overridden —
	// sending a partial body would NULL provider, code, discount, URLs, dates
	// and details, and reset is_verified.
	async function moderateDeal(row: Coupon, status: 'active' | 'suspended', ctx?: ActionContext) {
		pageError = '';
		const built = buildCouponPayload(row as unknown as Record<string, unknown>, {
			forModeration: true
		});
		if (!built.ok) {
			pageError = built.error;
			return;
		}
		built.payload.status = status;
		const res = await couponsAdminApiExtended.update(row.id, built.payload);
		if (!res.ok) {
			pageError = res.error ?? t('common.error', 'Failed to update deal');
			return;
		}
		ctx?.reload();
	}

	const discountTypeOptions = [
		{ value: 'percent', label: t('coupons.percentage', 'Percentage') },
		{ value: 'fixed', label: t('coupons.fixed', 'Fixed') },
		{ value: 'free_shipping', label: t('coupons.free_shipping', 'Free Shipping') }
	];

	const couponTypeOptions = [
		{ value: 'external', label: t('coupons.external', 'External (Partner)') },
		{ value: 'internal', label: t('coupons.internal', 'Internal (Platform)') }
	];

	const statusFilterOptions = [
		{ value: 'active', label: t('coupons.active', 'Active') },
		{ value: 'expired', label: t('coupons.expired', 'Expired') },
		{ value: 'suspended', label: t('coupons.suspended', 'Suspended') },
		{ value: 'pending', label: t('deals.pending', 'Pending review') }
	];

	// Form status select — pending deals become active/suspended after review.
	const statusOptions = [
		{ value: 'active', label: t('coupons.active', 'Active') },
		{ value: 'suspended', label: t('coupons.suspended', 'Suspended') },
		{ value: 'pending', label: t('deals.pending', 'Pending review') },
		{ value: 'expired', label: t('coupons.expired', 'Expired') }
	];

	// ── Provider autocomplete helpers (filter pre-loaded options client-side) ──
	function fetchProviderOptions(q: string): Promise<{ id: string; label: string }[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? providerOptions
			: providerOptions.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchProviderOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(providerOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	function getProviderSelectedLabel(id: string): string {
		return providerOptions.find((o) => o.value === id)?.label ?? id;
	}

	// `valid_from`/`expires_at` are converted to date-input format in `load` so
	// EntityForm's date inputs seed correctly. `provider_id`, `is_verified`,
	// `status`, and `details` are included per Section 55 (provider linkage,
	// verification, moderation status, and deal page content).
	let fields = $derived<FieldDef[]>([
		{ key: 'title', label: t('coupons.title_field', 'Title'), type: 'text', help: t('coupons.title_hint', 'The title shown in listings and detail views.'), required: true, placeholder: 'e.g. 20% off Remitly — first transfer' },
		{ key: 'slug', label: t('coupons.slug', 'Slug'), type: 'text', help: t('coupons.slug_hint', 'URL-safe identifier — lowercase, hyphens, no spaces.'), placeholder: t('coupons.slug_placeholder', 'Auto-generated from title if blank') },
		{ key: 'code', label: t('coupons.code', 'Code'), type: 'text', help: t('coupons.code_hint', 'The code users or systems enter — keep it short and unique.'), placeholder: t('coupons.code_placeholder', 'Optional — deal offers may not need one') },
		{ key: 'coupon_type', label: t('coupons.coupon_type', 'Coupon Type'), type: 'select', help: t('coupons.coupon_type_hint', 'How the coupon applies — percent, fixed, or free shipping.'), options: couponTypeOptions, hideInEdit: true },
		{ key: 'provider_id', label: t('coupons.provider', 'Provider'), type: 'autocomplete', help: t('coupons.provider_id_hint', 'The external provider or network.'), placeholder: t('coupons.provider_placeholder', 'Search provider…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: fetchProviderOptions, initialFetcher: fetchProviderOptionsInitial, getSelectedLabel: getProviderSelectedLabel },
		{ key: 'discount_type', label: t('coupons.type', 'Discount Type'), type: 'select', help: t('coupons.discount_type_hint', 'How the discount applies — percent, fixed, or free shipping.'), options: discountTypeOptions },
		{ key: 'discount_value', label: t('coupons.value', 'Value'), type: 'number', help: t('coupons.discount_value_hint', 'The discount amount — percent or currency units.') },
		currencies.length > 0
			? { key: 'currency', label: t('coupons.currency', 'Currency'), type: 'select', help: t('coupons.currency_hint', 'ISO currency code, e.g. NGN or USD.'), options: currencyOptions(currencies) }
			: { key: 'currency', label: t('coupons.currency', 'Currency'), type: 'text', help: t('coupons.currency_hint', 'ISO currency code, e.g. NGN or USD.'), placeholder: 'e.g. NGN, USD' },
		{ key: 'description', label: t('coupons.description_field', 'Description'), type: 'textarea', help: t('coupons.description_hint', 'Shown in detail views — keep it concise.') },
		{ key: 'details', label: t('deals.details_json', 'Deal page content (JSON)'), type: 'textarea', help: t('deals.details_hint', 'Full details — supports multiple paragraphs.'), placeholder: '{ "summary": "…", "sections": [{ "title": "", "body": "" }], "faq": [{ "q": "", "a": "" }], "trial": { "title": "", "body": "", "url": "" } }' },
		{ key: 'referral_url', label: t('coupons.referral_url', 'Referral URL'), type: 'url', help: t('coupons.referral_url_hint', 'The affiliate or referral landing URL.') },
		{ key: 'terms_url', label: t('coupons.terms_url', 'Terms URL'), type: 'url', help: t('coupons.terms_url_hint', 'Full URL to the terms document.') },
		{ key: 'valid_from', label: t('coupons.valid_from', 'Valid From'), type: 'date', help: t('coupons.valid_from_hint', 'The date this becomes effective.') },
		{ key: 'expires_at', label: t('coupons.valid_until', 'Valid Until'), type: 'date', help: t('coupons.expires_at_hint', 'After this date the record is treated as expired.') },
		{ key: 'is_verified', label: t('coupons.verified', 'Verified'), type: 'checkbox', help: t('coupons.is_verified_hint', 'Verified records show a badge and rank higher.') },
		{ key: 'status', label: t('common.status', 'Status'), type: 'select', help: t('common.status_hint', 'The record state — controls visibility and actions.'), options: statusOptions }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('deals.title', 'Deals'),
		description: t('deals.description', 'Manage deals and discount offers'),
		createLabel: t('deals.create', 'New deal'),
		createPermission: 'coupons.manage',
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('coupons.title_field', 'Title'), sortable: true },
			{ key: 'code', label: t('coupons.code', 'Code'), sortable: true },
			{
				key: 'provider_category',
				label: t('deals.category', 'Category'),
				render: (v: string | null) => v ?? '—'
			},
			{
				key: 'discount_value',
				label: t('coupons.discount', 'Discount'),
				sortable: true,
				render: (v: number, row: Coupon) => {
					if (v == null) return '—';
					if (row.discount_type === 'percent') return `${v}%`;
					if (row.discount_type === 'free_shipping') return t('coupons.free_shipping', 'Free Shipping');
					return formatMoney(v, row.currency, currencies);
				}
			},
			{
				key: 'total_clicks',
				label: t('coupons.clicks', 'Clicks'),
				sortable: true,
				render: (v: number) => `${v ?? 0}`
			},
			{
				key: 'expires_at',
				label: t('coupons.expires', 'Expires'),
				sortable: true,
				render: (v: string) => (v ? new Date(v).toLocaleDateString() : t('coupons.never', 'Never'))
			},
			{ key: 'status', label: t('common.status', 'Status'), sortable: true }
		],
		filters: [
			{ key: 'status', label: t('common.status', 'Status'), options: statusFilterOptions },
			{ key: 'coupon_type', label: t('coupons.coupon_type', 'Coupon Type'), options: couponTypeOptions }
		],
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Coupon, ctx?: ActionContext) => ctx?.openView(row) },
			{ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (row: Coupon, ctx?: ActionContext) => ctx?.openEdit(row) },
			{
				label: t('deals.approve', 'Approve'),
				icon: CheckCircle,
				show: (row: Coupon) => row.status === 'pending',
				onclick: (row: Coupon, ctx?: ActionContext) => moderateDeal(row, 'active', ctx)
			},
			{
				label: t('deals.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				show: (row: Coupon) => row.status === 'pending',
				onclick: (row: Coupon, ctx?: ActionContext) => moderateDeal(row, 'suspended', ctx)
			},
			{ label: t('common.delete', 'Delete'), icon: Trash2, variant: 'destructive', onclick: (row: Coupon, ctx?: ActionContext) => ctx?.openDelete(row) }
		],
		fields,
		load: async ({ q, page, pageSize, sort, filter }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			params.set('page', String(page + 1));
			if (pageSize) params.set('page_size', String(pageSize));
			if (filter.status) params.set('status', filter.status);
			if (filter.coupon_type) params.set('coupon_type', filter.coupon_type);
			// EntityPage emits sort as "key:dir"; the backend honors the
			// sortable column keys (?sort=key&dir=asc|desc, §55) and falls
			// back to newest-first for anything else.
			const [sortKey = '', sortDir = ''] = (sort ?? '').split(':');
			if (sortKey) params.set('sort', sortKey);
			if (sortDir === 'asc' || sortDir === 'desc') params.set('dir', sortDir);
			const result = await couponsAdminApiExtended.list(params.toString() || undefined);
			if (result.ok) {
				// apiRaw — the body is { data: [...], total } (§55).
				const body = result.data;
				const raw = (Array.isArray(body?.data) ? body.data : []) as Coupon[];
				// Convert ISO timestamps to date-input format for form seeding; the
				// details object is stringified for the JSON textarea field.
				const data = raw.map((c) => ({
					...c,
					// Raw ISO kept for moderation payloads — the date-input values
					// below collapse to YYYY-MM-DD and must not be re-expanded on
					// approve/reject (they're only for form seeding).
					_valid_from_iso: c.valid_from ?? null,
					_expires_at_iso: c.expires_at ?? null,
					valid_from: c.valid_from ? isoTimestampToDateInput(c.valid_from) : '',
					expires_at: c.expires_at ? isoTimestampToDateInput(c.expires_at) : '',
					details: c.details ? JSON.stringify(c.details, null, 2) : ''
				}));
				// Server-provided total drives EntityPage pagination; fall back to
				// the page row count when the field is absent.
				const total = typeof body?.total === 'number' ? body.total : data.length;
				const totalPages =
					typeof body?.total_pages === 'number'
						? body.total_pages
						: pageSize
							? Math.max(1, Math.ceil(total / pageSize))
							: 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load deals'));
		},
		create: async (body) => {
			const built = buildCouponPayload(body, { forCreate: true });
			if (!built.ok) return { ok: false, error: built.error };
			const res = await couponsAdminApiExtended.create(built.payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const built = buildCouponPayload(body);
			if (!built.ok) return { ok: false, error: built.error };
			const res = await couponsAdminApiExtended.update(id, built.payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await couponsAdminApiExtended.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: CouponView
	});
</script>

<svelte:head>
	<title>{t('deals.title', 'Deals')} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

{#snippet CouponView({ row }: { row: Coupon })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.title_field', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.code', 'Code')}</p>
			<p class="mt-1 font-mono text-sm font-medium" style="color: var(--color-foreground)">{row.code ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.slug', 'Slug')}</p>
			<p class="mt-1 font-mono text-sm font-medium" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.coupon_type', 'Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.coupon_type}</p>
		</div>
		{#if row.provider_name}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.provider', 'Provider')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.provider_name}</p>
			</div>
		{/if}
		{#if row.provider_category}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('deals.category', 'Category')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.provider_category}</p>
			</div>
		{/if}
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.description_field', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		{#if row.details}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('deals.details', 'Deal details')}</p>
				<pre class="mt-1 max-h-64 overflow-auto rounded-lg p-3 text-xs" style="background: var(--color-surface-alt, var(--color-surface)); color: var(--color-foreground)">{typeof row.details === 'string' ? row.details : JSON.stringify(row.details, null, 2)}</pre>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.discount', 'Discount')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.discount_type === 'percent' ? `${row.discount_value}%` : row.discount_type === 'free_shipping' ? t('coupons.free_shipping', 'Free Shipping') : formatMoney(row.discount_value, row.currency, currencies)}
			</p>
		</div>
		{#if row.referral_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.referral_url', 'Referral URL')}</p>
				<p class="mt-1 break-all text-sm" style="color: var(--color-foreground)">{row.referral_url}</p>
			</div>
		{/if}
		{#if row.terms_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.terms_url', 'Terms URL')}</p>
				<p class="mt-1 break-all text-sm" style="color: var(--color-foreground)">{row.terms_url}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.clicks', 'Clicks')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.total_clicks ?? 0}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.copies', 'Copies')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.total_copies ?? 0}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.verified', 'Verified')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_verified ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.expires', 'Expires')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.expires_at ? new Date(row.expires_at).toLocaleDateString() : t('coupons.never', 'Never')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
	</div>
{/snippet}
