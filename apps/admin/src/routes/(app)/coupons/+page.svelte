<script lang="ts">
	// /coupons — migrated to the EntityPage scaffold; declarative CRUD entity with date conversion.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { couponsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';
	import { dateInputToStartOfDayISO, dateInputToEndOfDayISO, isoTimestampToDateInput } from '$lib/utils/date';

	type Coupon = {
		id: string;
		code: string;
		discount_type: string;
		discount_value: number;
		status: string;
		valid_from?: string;
		expires_at?: string;
		total_clicks: number;
		total_copies: number;
		created_at: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

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
		{ value: 'fixed', label: t('coupons.fixed', 'Fixed') }
	];

	// `is_active` is derived from `status`; `valid_from`/`expires_at` are converted
	// to date-input format in `load` so EntityForm's date inputs seed correctly.
	const fields: FieldDef[] = [
		{ key: 'code', label: t('coupons.code', 'Code'), type: 'text', required: true },
		{ key: 'discount_type', label: t('coupons.type', 'Type'), type: 'select', options: discountTypeOptions },
		{ key: 'discount_value', label: t('coupons.value', 'Value'), type: 'number', required: true },
		{ key: 'valid_from', label: t('coupons.valid_from', 'Valid From'), type: 'date' },
		{ key: 'expires_at', label: t('coupons.valid_until', 'Valid Until'), type: 'date' },
		{ key: 'usage_limit', label: t('coupons.usage_limit', 'Usage Limit'), type: 'number', min: 0 },
		{ key: 'is_active', label: t('coupons.active', 'Active'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('coupons.title', 'Coupons'),
		description: t('coupons.description', 'Manage discount coupons'),
		createLabel: t('coupons.create', 'Create Coupon'),
		createPermission: 'coupons.manage',
		pageSize: 25,
		columns: [
			{ key: 'code', label: t('coupons.code', 'Code'), sortable: true },
			{
				key: 'discount_value',
				label: t('coupons.discount', 'Discount'),
				sortable: true,
				render: (v: number, row: Coupon) => {
					if (v == null) return '—';
					return row.discount_type === 'percent' ? `${v}%` : `${v} credits`;
				}
			},
			{
				key: 'total_clicks',
				label: t('coupons.used', 'Used'),
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
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
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
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load coupons'));
		},
		create: async (body) => {
			const payload: Record<string, unknown> = {
				title: String(body.code ?? '').trim(),
				code: String(body.code ?? '').trim().toUpperCase(),
				coupon_type: 'external',
				discount_type: body.discount_type ?? 'percent',
				discount_value: Number(body.discount_value),
				status: body.is_active ? 'active' : 'suspended'
			};
			if (body.valid_from) payload.valid_from = dateInputToStartOfDayISO(String(body.valid_from));
			if (body.expires_at) payload.valid_until = dateInputToEndOfDayISO(String(body.expires_at));
			if (body.usage_limit != null && body.usage_limit !== '') payload.usage_limit = Number(body.usage_limit);
			const res = await couponsAdminApiExtended.create(payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = {
				title: String(body.code ?? '').trim(),
				code: String(body.code ?? '').trim().toUpperCase(),
				coupon_type: 'external',
				discount_type: body.discount_type ?? 'percent',
				discount_value: Number(body.discount_value),
				status: body.is_active ? 'active' : 'suspended'
			};
			if (body.valid_from) payload.valid_from = dateInputToStartOfDayISO(String(body.valid_from));
			if (body.expires_at) payload.valid_until = dateInputToEndOfDayISO(String(body.expires_at));
			if (body.usage_limit != null && body.usage_limit !== '') payload.usage_limit = Number(body.usage_limit);
			const res = await couponsAdminApiExtended.update(id, payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await couponsAdminApiExtended.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: CouponView
	};
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
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.code', 'Code')}</p>
			<p class="mt-1 font-mono text-sm font-medium" style="color: var(--color-foreground)">{row.code}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.discount', 'Discount')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.discount_type === 'percent' ? `${row.discount_value}%` : `${row.discount_value} credits`}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupons.used', 'Used')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.total_clicks ?? 0}</p>
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
