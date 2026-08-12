<script lang="ts">
	// /discount-codes — migrated to the EntityPage scaffold with computed-status filter
	// and a page-local usage SlideOver (view-detail sub-flow).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { dateInputToStartOfDayISO, dateInputToEndOfDayISO } from '$lib/utils/date';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { Eye, Trash2 } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type DiscountCode = {
		id: string;
		code: string;
		description?: string;
		discount_type: string;
		discount_value: number;
		applicable_to?: string;
		max_uses?: number;
		uses_per_user?: number;
		valid_from?: string;
		valid_until?: string;
		used_count?: number;
		created_at: string;
		// Computed client-side for filtering/display.
		computed_status?: string;
	};

	type UsageData = {
		users: Array<{ user_id: string; used_at: string }>;
		total: number;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	// Usage slide-over (view-detail sub-flow)
	let showUsageSlideOver = $state(false);
	let usageCode = $state<DiscountCode | null>(null);
	let usageData = $state<UsageData | null>(null);
	let usageLoading = $state(false);

	const canManageDiscounts = $derived(hasPermission('payments.discount_codes.manage'));

	function computeStatus(row: DiscountCode): string {
		const now = new Date();
		const isExpired = !!row.valid_until && new Date(row.valid_until) < now;
		const isUsedUp = row.max_uses != null && row.used_count != null && row.used_count >= row.max_uses;
		if (isExpired) return 'expired';
		if (isUsedUp) return 'used';
		return 'active';
	}

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => api(`/admin/discount-codes/${id}`, { method: 'DELETE' })));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete codes');
		} finally {
			bulkDeleting = false;
		}
	}

	async function viewUsage(codeItem: DiscountCode) {
		usageCode = codeItem;
		usageData = null;
		usageLoading = true;
		showUsageSlideOver = true;
		try {
			const result = await api<UsageData>(`/admin/discount-codes/${codeItem.id}/usage`);
			if (result.ok) {
				usageData = result.data;
			} else {
				pageError = result.error;
			}
		} catch {
			pageError = t('common.error', 'Failed to load usage');
		} finally {
			usageLoading = false;
		}
	}

	const discountTypeOptions = [
		{ value: 'percent', label: t('discounts.percentage', 'Percentage') },
		{ value: 'fixed', label: t('discounts.fixed', 'Fixed') }
	];

	const applicableToOptions = [
		{ value: 'all', label: t('discounts.applicable_all', 'All purposes') },
		{ value: 'course_access', label: t('discounts.applicable_course', 'Course access') },
		{ value: 'pro_membership', label: t('discounts.applicable_pro', 'Pro membership') }
	];

	const fields: FieldDef[] = [
		{ key: 'code', label: t('discounts.code', 'Code'), type: 'text', required: true },
		{ key: 'description', label: t('discounts.description', 'Description'), type: 'text' },
		{ key: 'discount_type', label: t('discounts.type', 'Type'), type: 'select', options: discountTypeOptions },
		{ key: 'discount_value', label: t('discounts.value', 'Value'), type: 'number', required: true },
		{ key: 'applicable_to', label: t('discounts.applicable_to', 'Applies To'), type: 'select', options: applicableToOptions },
		{ key: 'valid_from', label: t('discounts.valid_from', 'Valid From'), type: 'date' },
		{ key: 'valid_until', label: t('discounts.valid_until', 'Valid Until'), type: 'date' },
		{ key: 'max_uses', label: t('discounts.usage_limit', 'Usage Limit'), type: 'number', min: 0 }
	];

	// Custom actions: View Usage + default Edit/Delete (permission-gated).
	// When `actions` is provided, defaults are replaced — include Edit/Delete explicitly.
	const actions: ActionItem[] = [
		{ label: t('discounts.usage', 'Usage'), icon: Eye, onclick: (row) => viewUsage(row) },
		{
			label: t('common.edit', 'Edit'),
			onclick: (row, ctx) => ctx?.openEdit(row),
			show: () => canManageDiscounts
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row, ctx) => ctx?.openDelete(row),
			show: () => canManageDiscounts
		}
	];

	const config: EntityPageConfig = {
		title: t('discounts.title', 'Discount Codes'),
		description: t('discounts.description', 'Manage discount codes'),
		createLabel: t('discounts.create', 'Create Code'),
		createPermission: 'payments.discount_codes.manage',
		pageSize: 25,
		columns: [
			{
				key: 'code',
				label: t('discounts.code', 'Code'),
				sortable: true,
				render: (v: string) => `<div class="flex items-center gap-2"><span class="font-mono text-sm font-medium text-foreground">${v}</span><button onclick="navigator.clipboard.writeText('${v}')" class="text-muted hover:text-foreground-alt" title="${t('discounts.copy_code', 'Copy code')}"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg></button></div>`
			},
			{
				key: 'discount_value',
				label: t('discounts.discount', 'Discount'),
				sortable: true,
				render: (v: number, row: DiscountCode) => `<span class="font-medium">${row.discount_type === 'percent' ? `${v}%` : `${v} credits`}</span>`
			},
			{
				key: 'used_count',
				label: t('discounts.used', 'Used'),
				sortable: true,
				render: (v: number, row: DiscountCode) => `<span class="text-sm text-foreground-alt">${v ?? 0}${row.max_uses ? ` / ${row.max_uses}` : ''}</span>`
			},
			{
				key: 'valid_until',
				label: t('discounts.expires', 'Expires'),
				sortable: true,
				render: (v: string) => (v ? `<span class="text-sm">${new Date(v).toLocaleDateString()}</span>` : '<span class="text-sm text-muted">Never</span>')
			},
			{
				key: 'computed_status',
				label: t('common.status', 'Status'),
				render: (_: any, row: DiscountCode) => {
					const status = computeStatus(row);
					const classes: Record<string, string> = {
						active: 'bg-success/15 text-success border-success',
						expired: 'bg-surface text-foreground border',
						used: 'bg-warning/15 text-warning border-warning'
					};
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[status]}">${status}</span>`;
				}
			}
		],
		filters: [
			{
				key: 'computed_status',
				label: t('common.status', 'Status'),
				options: [
					{ value: 'active', label: 'Active' },
					{ value: 'inactive', label: 'Inactive' },
					{ value: 'expired', label: 'Expired' },
					{ value: 'used', label: 'Used Up' }
				]
			}
		],
		bulkActions: [
			{ label: t('common.deleteSelected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }
		],
		actions,
		fields,
		load: async ({ q, filter }) => {
			const result = await api<DiscountCode[]>('/admin/discount-codes');
			if (result.ok) {
				let data = (result.data ?? []) as DiscountCode[];
				// Compute status for each row (used by column render + filter).
				data = data.map((c) => ({ ...c, computed_status: computeStatus(c) }));
				if (filter.computed_status === 'inactive') data = data.filter((c) => c.computed_status !== 'active');
			else if (filter.computed_status) data = data.filter((c) => c.computed_status === filter.computed_status);
				if (q) {
					const lq = q.toLowerCase();
					data = data.filter((c) => c.code?.toLowerCase().includes(lq) || (c.description ?? '').toLowerCase().includes(lq));
				}
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load discount codes'));
		},
		create: async (body) => {
			const payload: Record<string, unknown> = {
				code: String(body.code ?? '').trim().toUpperCase(),
				description: String(body.description ?? '').trim() || undefined,
				discount_type: body.discount_type ?? 'percent',
				discount_value: Number(body.discount_value),
				applicable_to: body.applicable_to ?? 'all',
				valid_from: body.valid_from ? dateInputToStartOfDayISO(String(body.valid_from)) : null,
				valid_until: body.valid_until ? dateInputToEndOfDayISO(String(body.valid_until)) : null,
				max_uses: body.max_uses != null && body.max_uses !== '' ? Number(body.max_uses) : null
			};
			const res = await api('/admin/discount-codes', { method: 'POST', body: payload });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = {
				code: String(body.code ?? '').trim().toUpperCase(),
				description: String(body.description ?? '').trim() || undefined,
				discount_type: body.discount_type ?? 'percent',
				discount_value: Number(body.discount_value),
				applicable_to: body.applicable_to ?? 'all',
				valid_from: body.valid_from ? dateInputToStartOfDayISO(String(body.valid_from)) : null,
				valid_until: body.valid_until ? dateInputToEndOfDayISO(String(body.valid_until)) : null,
				max_uses: body.max_uses != null && body.max_uses !== '' ? Number(body.max_uses) : null
			};
			const res = await api(`/admin/discount-codes/${id}`, { method: 'PATCH', body: payload });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/discount-codes/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: CodeView
	};
</script>

<svelte:head>
	<title>{t('discounts.title', 'Discount Codes')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Usage SlideOver (view-detail sub-flow) -->
<SlideOver bind:open={showUsageSlideOver} title={usageCode ? `${t('discounts.usage', 'Usage')}: ${usageCode.code}` : t('discounts.usage', 'Usage')} size="lg">
	{#if usageLoading}
		<p class="py-8 text-center text-sm text-muted">{t('common.loading', 'Loading...')}</p>
	{:else if usageData}
		<p class="mb-3 text-sm text-muted">{t('discounts.usage_total', 'Total uses')}: {usageData.total}</p>
		{#if usageData.users.length === 0}
			<p class="text-sm text-muted">{t('discounts.usage_empty', 'No usage recorded yet')}</p>
		{:else}
			<div class="max-h-96 overflow-y-auto rounded-lg border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b" style="border-color: var(--color-border); background: var(--color-surface)">
							<th class="px-4 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('app.discount-codes.user', 'User')}</th>
							<th class="px-4 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('discounts.used_at', 'Used At')}</th>
						</tr>
					</thead>
					<tbody>
						{#each usageData.users as u (u.user_id + u.used_at)}
							<tr class="border-b last:border-0" style="border-color: var(--color-border)">
								<td class="px-4 py-2 font-mono text-xs">{u.user_id}</td>
								<td class="px-4 py-2 text-xs" style="color: var(--color-muted)">{new Date(u.used_at).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{:else}
		<p class="py-8 text-center text-sm text-muted">{t('common.no_data', 'No data available')}</p>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (showUsageSlideOver = false)}>{t('common.close', 'Close')}</Button>
		</div>
	{/snippet}
</SlideOver>

{#snippet CodeView({ row }: { row: DiscountCode })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('discounts.code', 'Code')}</p>
			<p class="mt-1 font-mono text-sm font-medium" style="color: var(--color-foreground)">{row.code}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('discounts.description', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description ?? '—'}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('discounts.discount', 'Discount')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.discount_type === 'percent' ? `${row.discount_value}%` : `${row.discount_value} credits`}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('discounts.used', 'Used')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.used_count ?? 0}{row.max_uses ? ` / ${row.max_uses}` : ''}</p>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('discounts.valid_from', 'Valid From')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.valid_from ? new Date(row.valid_from).toLocaleDateString() : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('discounts.valid_until', 'Valid Until')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.valid_until ? new Date(row.valid_until).toLocaleDateString() : 'Never'}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{computeStatus(row)}</p>
		</div>
	</div>
{/snippet}
