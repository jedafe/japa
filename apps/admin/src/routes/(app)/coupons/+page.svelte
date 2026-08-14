<script lang="ts">
	// /coupons — migrated to the EntityPage scaffold; declarative CRUD entity with date conversion.
	// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { couponsAdminApiExtended, affiliateProvidersAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';
	import { dateInputToStartOfDayISO, dateInputToEndOfDayISO, isoTimestampToDateInput } from '$lib/utils/date';
	import { onMount } from 'svelte';

	type Coupon = {
		id: string;
		title: string;
		code: string;
		coupon_type: string;
		provider_id: string | null;
		provider_name?: string | null;
		description: string | null;
		discount_type: string | null;
		discount_value: number | null;
		currency: string | null;
		terms_url: string | null;
		referral_url: string | null;
		is_verified: boolean;
		status: string;
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
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);
	let providerOptions = $state<Array<{ value: string; label: string }>>([]);

	// Load affiliate providers for the provider_id select (Section 55 — provider linkage).
	onMount(async () => {
		try {
			const res = await affiliateProvidersAdminApi.list('page_size=100');
			if (res.ok && Array.isArray(res.data)) {
				providerOptions = (res.data as Provider[]).map((p) => ({ value: p.id, label: p.name }));
			}
		} catch {
			// Providers list is best-effort; the select will just be empty.
		}
	});

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => couponsAdminApiExtended.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete coupons');
		} finally {
			bulkDeleting = false;
		}
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
		{ value: 'suspended', label: t('coupons.suspended', 'Suspended') }
	];

	// `is_active` is derived from `status`; `valid_from`/`expires_at` are converted
	// to date-input format in `load` so EntityForm's date inputs seed correctly.
	// `provider_id` and `is_verified` are included per Section 55 (provider linkage + verify).
	let fields = $derived<FieldDef[]>([
		{ key: 'title', label: t('coupons.title_field', 'Title'), type: 'text', required: true, placeholder: 'e.g. 20% off Remitly — first transfer' },
		{ key: 'code', label: t('coupons.code', 'Code'), type: 'text', required: true },
		{ key: 'coupon_type', label: t('coupons.coupon_type', 'Coupon Type'), type: 'select', options: couponTypeOptions },
		{ key: 'provider_id', label: t('coupons.provider', 'Provider'), type: 'select', options: providerOptions },
		{ key: 'discount_type', label: t('coupons.type', 'Discount Type'), type: 'select', options: discountTypeOptions },
		{ key: 'discount_value', label: t('coupons.value', 'Value'), type: 'number' },
		{ key: 'currency', label: t('coupons.currency', 'Currency'), type: 'text', placeholder: 'e.g. NGN, USD' },
		{ key: 'description', label: t('coupons.description_field', 'Description'), type: 'textarea' },
		{ key: 'referral_url', label: t('coupons.referral_url', 'Referral URL'), type: 'url' },
		{ key: 'terms_url', label: t('coupons.terms_url', 'Terms URL'), type: 'url' },
		{ key: 'valid_from', label: t('coupons.valid_from', 'Valid From'), type: 'date' },
		{ key: 'expires_at', label: t('coupons.valid_until', 'Valid Until'), type: 'date' },
		{ key: 'is_verified', label: t('coupons.verified', 'Verified'), type: 'checkbox' },
		{ key: 'is_active', label: t('coupons.active', 'Active'), type: 'checkbox' }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('coupons.title', 'Coupons'),
		description: t('coupons.description', 'Manage discount coupons'),
		createLabel: t('coupons.create', 'Create Coupon'),
		createPermission: 'coupons.manage',
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('coupons.title_field', 'Title'), sortable: true },
			{ key: 'code', label: t('coupons.code', 'Code'), sortable: true },
			{
				key: 'discount_value',
				label: t('coupons.discount', 'Discount'),
				sortable: true,
				render: (v: number, row: Coupon) => {
					if (v == null) return '—';
					if (row.discount_type === 'percent') return `${v}%`;
					if (row.discount_type === 'free_shipping') return t('coupons.free_shipping', 'Free Shipping');
					return `${v} ${row.currency ?? ''}`.trim();
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
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, page, pageSize, sort, filter }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (page) params.set('page', String(page));
			if (pageSize) params.set('page_size', String(pageSize));
			if (filter.status) params.set('status', filter.status);
			if (filter.coupon_type) params.set('coupon_type', filter.coupon_type);
			// EntityPage emits sort as "key:dir"; the backend ignores sort for now
			// (fixed ORDER BY created_at DESC), but we pass it for forward-compat.
			const sortKey = sort ? sort.split(':')[0] : '';
			if (sortKey) params.set('sort', sortKey);
			const result = await couponsAdminApiExtended.list(params.toString() || undefined);
			if (result.ok) {
				const raw = (result.data as Coupon[]) ?? [];
				// Convert ISO timestamps to date-input format for form seeding; add is_active derived from status.
				const data = raw.map((c) => ({
					...c,
					valid_from: c.valid_from ? isoTimestampToDateInput(c.valid_from) : '',
					expires_at: c.expires_at ? isoTimestampToDateInput(c.expires_at) : '',
					is_active: c.status === 'active'
				}));
				// Use server-provided total if available; fall back to data.length.
				const total = (result as any).total ?? data.length;
				const totalPages = pageSize ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load coupons'));
		},
		create: async (body) => {
			const payload: Record<string, unknown> = {
				title: String(body.title ?? '').trim(),
				code: String(body.code ?? '').trim().toUpperCase(),
				coupon_type: body.coupon_type ?? 'external',
				discount_type: body.discount_type ?? 'percent',
				discount_value: body.discount_value ? Number(body.discount_value) : null,
				is_verified: !!body.is_verified,
				status: body.is_active ? 'active' : 'suspended'
			};
			if (body.provider_id) payload.provider_id = String(body.provider_id);
			if (body.currency) payload.currency = String(body.currency).trim();
			if (body.description) payload.description = String(body.description).trim();
			if (body.referral_url) payload.referral_url = String(body.referral_url).trim();
			if (body.terms_url) payload.terms_url = String(body.terms_url).trim();
			if (body.valid_from) payload.valid_from = dateInputToStartOfDayISO(String(body.valid_from));
			if (body.expires_at) payload.valid_until = dateInputToEndOfDayISO(String(body.expires_at));
			const res = await couponsAdminApiExtended.create(payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = {
				title: String(body.title ?? '').trim(),
				code: String(body.code ?? '').trim().toUpperCase(),
				coupon_type: body.coupon_type ?? 'external',
				discount_type: body.discount_type ?? 'percent',
				discount_value: body.discount_value ? Number(body.discount_value) : null,
				is_verified: !!body.is_verified,
				status: body.is_active ? 'active' : 'suspended'
			};
			if (body.provider_id) payload.provider_id = String(body.provider_id);
			if (body.currency) payload.currency = String(body.currency).trim();
			if (body.description) payload.description = String(body.description).trim();
			if (body.referral_url) payload.referral_url = String(body.referral_url).trim();
			if (body.terms_url) payload.terms_url = String(body.terms_url).trim();
			if (body.valid_from) payload.valid_from = dateInputToStartOfDayISO(String(body.valid_from));
			if (body.expires_at) payload.valid_until = dateInputToEndOfDayISO(String(body.expires_at));
			const res = await couponsAdminApiExtended.update(id, payload);
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
	<title>{t('coupons.title', 'Coupons')} — JapaConnect Admin</title>
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
			<p class="mt-1 font-mono text-sm font-medium" style="color: var(--color-foreground)">{row.code}</p>
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
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.description_field', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.discount', 'Discount')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.discount_type === 'percent' ? `${row.discount_value}%` : row.discount_type === 'free_shipping' ? t('coupons.free_shipping', 'Free Shipping') : `${row.discount_value ?? ''} ${row.currency ?? ''}`.trim()}
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
