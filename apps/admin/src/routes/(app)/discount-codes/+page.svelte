<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { Modal } from '$lib/components/ui/modal';
	import { Plus, Pencil, Trash2, Eye } from 'lucide-svelte';

	type DiscountCode = {
		id: string;
		code: string;
		description?: string;
		discount_type: string;
		discount_value: number;
		is_active: boolean;
		valid_from?: string;
		valid_until?: string;
		usage_limit?: number;
		used_count: number;
		created_at: string;
	};

	let codes = $state<DiscountCode[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let showUsageModal = $state(false);
	let usageCode = $state<DiscountCode | null>(null);
	let usageData = $state<{ users: Array<{ user_id: string; used_at: string }>; total: number } | null>(null);
	let saving = $state(false);

	// Delete confirm
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<DiscountCode | null>(null);

	const canManageDiscounts = $derived(hasPermission('discount_codes.manage'));

	let code = $state('');
	let description = $state('');
	let discountType = $state('percentage');
	let discountValue = $state('');
	let validFrom = $state('');
	let validUntil = $state('');
	let usageLimit = $state('');
	let isActive = $state(true);

	let activeTab = $state('active');

	const tabs = $derived([
		{ value: 'active', label: 'Active', count: codes.filter((c) => c.is_active).length },
		{ value: 'inactive', label: 'Inactive', count: codes.filter((c) => !c.is_active).length },
		{ value: 'all', label: 'All', count: codes.length }
	]);

	const columns = [
		{
			key: 'code',
			label: 'Code',
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-2">
					<span class="font-mono text-sm font-medium text-foreground">${row.code}</span>
					<button onclick="navigator.clipboard.writeText('${row.code}')" class="text-muted hover:text-foreground-alt" title="Copy code">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
					</button>
				</div>
			`
		},
		{
			key: 'discount_value',
			label: 'Discount',
			sortable: true,
			render: (value: number, row: any) => `<span class="font-medium">${row.discount_type === 'percentage' ? `${value}%` : `${value} credits`}</span>`
		},
		{
			key: 'used_count',
			label: 'Used',
			sortable: true,
			render: (value: number, row: any) => `<span class="text-sm text-foreground-alt">${value ?? 0}${row.usage_limit ? ` / ${row.usage_limit}` : ''}</span>`
		},
		{
			key: 'valid_until',
			label: 'Expires',
			sortable: true,
			render: (value: string) => value ? `<span class="text-sm">${new Date(value).toLocaleDateString()}</span>` : '<span class="text-sm text-muted">Never</span>'
		},
		{
			key: 'is_active',
			label: 'Status',
			sortable: true,
			render: (_: any, row: any) => {
				const isExpired = row.valid_until && new Date(row.valid_until) < new Date();
				const isUsedUp = row.usage_limit && row.used_count >= row.usage_limit;
				const status = isExpired ? 'expired' : isUsedUp ? 'used' : row.is_active ? 'active' : 'inactive';
				const classes: Record<string, string> = {
					active: 'bg-success/15 text-success border-success',
					inactive: 'bg-surface text-foreground border',
					expired: 'bg-surface text-foreground border',
					used: 'bg-warning/15 text-warning border-warning'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[status]}">${status}</span>`;
			}
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			component: DiscountActionsCell
		}
	];

	let filteredCodes = $derived.by(() => {
		let result = [...codes];
		if (activeTab === 'active') result = result.filter((c) => c.is_active);
		else if (activeTab === 'inactive') result = result.filter((c) => !c.is_active);
		return result;
	});

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<DiscountCode[]>('/admin/discount-codes');
			if (result.ok) {
				codes = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		code = '';
		description = '';
		discountType = 'percentage';
		discountValue = '';
		validFrom = '';
		validUntil = '';
		usageLimit = '';
		isActive = true;
		showCreateModal = true;
	}

	function openEdit(codeItem: DiscountCode) {
		code = codeItem.code;
		description = codeItem.description || '';
		discountType = codeItem.discount_type;
		discountValue = String(codeItem.discount_value);
		validFrom = codeItem.valid_from || '';
		validUntil = codeItem.valid_until || '';
		usageLimit = codeItem.usage_limit ? String(codeItem.usage_limit) : '';
		isActive = codeItem.is_active;
		usageCode = codeItem;
		showCreateModal = true;
	}

	async function saveCode() {
		if (!code.trim() || !discountValue) return;
		saving = true;
		errorMessage = '';
		try {
			const body: Record<string, unknown> = {
				code: code.trim().toUpperCase(),
				description: description.trim() || undefined,
				discount_type: discountType,
				discount_value: Number(discountValue),
				is_active: isActive
			};
			if (validFrom) body.valid_from = validFrom;
			if (validUntil) body.valid_until = validUntil;
			if (usageLimit) body.usage_limit = Number(usageLimit);

			const url = usageCode ? `/admin/discount-codes/${usageCode.id}` : '/admin/discount-codes';
			const method = usageCode ? 'PATCH' : 'POST';
			const result = await api(url, { method, body });
			if (result.ok) {
				showCreateModal = false;
				usageCode = null;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function deleteCode(id: string) {
		errorMessage = '';
		try {
			const result = await api(`/admin/discount-codes/${id}`, { method: 'DELETE' });
			if (result.ok) {
				codes = codes.filter((c) => c.id !== id);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function viewUsage(codeItem: DiscountCode) {
		errorMessage = '';
		try {
			const result = await api<{ users: Array<{ user_id: string; used_at: string }>; total: number }>(`/admin/discount-codes/${codeItem.id}/usage`);
			if (result.ok) {
				usageCode = codeItem;
				usageData = result.data;
				showUsageModal = true;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	function confirmDelete(codeItem: DiscountCode) {
		deleteTarget = codeItem;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		await deleteCode(deleteTarget.id);
		showDeleteConfirm = false;
		deleteTarget = null;
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('discounts.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('discounts.title')}
	description={t('discounts.description')}
	actions={[
		{ label: t('discounts.create'), icon: Plus, onclick: openCreate }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
			{:else}
				<DataTable
					columns={filteredCodes.length > 0 ? columns : []}
					data={filteredCodes}
					pageSize={20}
					selectable={true}
					emptyMessage={t('discounts.empty')}
				/>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create/Edit Modal -->
<Modal bind:open={showCreateModal} title={usageCode ? t('discounts.edit') : t('discounts.create')}>
	<div class="space-y-4">
		<div>
			<label for="dc-code" class="mb-1 block text-sm font-medium text-foreground-alt">{t('discounts.code')}</label>
			<input id="dc-code" type="text" bind:value={code} required class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm uppercase placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" />
		</div>
		<div>
			<label for="dc-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('discounts.description')}</label>
			<input id="dc-desc" type="text" bind:value={description} class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
			<label for="dc-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('discounts.type')}</label>
			<Select
				options={[
					{ value: 'percentage', label: t('discounts.percentage') },
					{ value: 'fixed', label: t('discounts.fixed') }
				]}
				bind:value={discountType}
			/>
			</div>
			<div>
				<label for="dc-value" class="mb-1 block text-sm font-medium text-foreground-alt">{t('discounts.value')}</label>
				<input id="dc-value" type="number" bind:value={discountValue} required class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="dc-from" class="mb-1 block text-sm font-medium text-foreground-alt">{t('discounts.valid_from')}</label>
				<input id="dc-from" type="date" bind:value={validFrom} class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" />
			</div>
			<div>
				<label for="dc-until" class="mb-1 block text-sm font-medium text-foreground-alt">{t('discounts.valid_until')}</label>
				<input id="dc-until" type="date" bind:value={validUntil} class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="dc-limit" class="mb-1 block text-sm font-medium text-foreground-alt">{t('discounts.usage_limit')}</label>
				<input id="dc-limit" type="number" bind:value={usageLimit} class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="dc-active" type="checkbox" bind:checked={isActive} class="h-4 w-4 rounded border text-primary-600 focus:ring-primary" />
				<label for="dc-active" class="text-sm font-medium text-foreground-alt">{t('discounts.active')}</label>
			</div>
		</div>
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" onclick={() => { showCreateModal = false; usageCode = null; }}>{t('common.cancel')}</Button>
			<Button onclick={saveCode} disabled={saving}>
				{saving ? t('common.saving') : usageCode ? t('common.save') : t('common.create')}
			</Button>
		</div>
	</div>
</Modal>

<!-- Usage Modal -->
<Modal bind:open={showUsageModal} title={t('discounts.usage')}>
	{#if usageData}
		<p class="mb-3 text-sm text-muted">{t('discounts.usage_total')}: {usageData.total}</p>
		{#if usageData.users.length === 0}
			<p class="text-sm text-muted">{t('discounts.usage_empty')}</p>
		{:else}
			<div class="max-h-64 overflow-y-auto rounded-lg border border">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border bg-surface">
							<th class="px-4 py-2 text-left text-xs font-medium text-muted">{t('app.discount-codes.user')}</th>
							<th class="px-4 py-2 text-left text-xs font-medium text-muted">{t('discounts.used_at')}</th>
						</tr>
					</thead>
					<tbody>
						{#each usageData.users as u (u.user_id)}
							<tr class="border-b border last:border-0">
								<td class="px-4 py-2 font-mono text-xs">{u.user_id}</td>
								<td class="px-4 py-2 text-xs text-muted">{new Date(u.used_at).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
	<div class="flex justify-end pt-4">
		<Button variant="outline" onclick={() => showUsageModal = false}>{t('common.close')}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete', 'Confirm Delete')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('discounts.delete_confirm', 'Delete discount code')} <strong class="font-mono">{deleteTarget.code}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>

<!-- DiscountActionsCell component -->
{#snippet DiscountActionsCell(row: DiscountCode)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => viewUsage(row)} title={t('discounts.usage')}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageDiscounts}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)" title={t('common.delete')}>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
