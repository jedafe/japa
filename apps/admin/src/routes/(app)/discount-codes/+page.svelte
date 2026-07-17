<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
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
					<span class="font-mono text-sm font-medium text-gray-900">${row.code}</span>
					<button onclick="navigator.clipboard.writeText('${row.code}')" class="text-gray-400 hover:text-gray-600" title="Copy code">
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
			render: (value: number, row: any) => `<span class="text-sm text-gray-600">${value ?? 0}${row.usage_limit ? ` / ${row.usage_limit}` : ''}</span>`
		},
		{
			key: 'valid_until',
			label: 'Expires',
			sortable: true,
			render: (value: string) => value ? `<span class="text-sm">${new Date(value).toLocaleDateString()}</span>` : '<span class="text-sm text-gray-400">Never</span>'
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
					active: 'bg-success-100 text-success-800 border-success-200',
					inactive: 'bg-gray-100 text-gray-800 border-gray-200',
					expired: 'bg-gray-100 text-gray-800 border-gray-200',
					used: 'bg-warning-100 text-warning-800 border-warning-200'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[status]}">${status}</span>`;
			}
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__viewUsage('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-info-700 hover:bg-info-50" title="View usage">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"></path></svg>
					</button>
					<button onclick="window.__editCode('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100" title="Edit">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path></svg>
					</button>
					<button onclick="window.__deleteCode('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50" title="Delete">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
					</button>
				</div>
			`
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

	$effect(() => {
		load();
		(window as any).__editCode = (id: string) => {
			const item = codes.find((c) => c.id === id);
			if (item) openEdit(item);
		};
		(window as any).__deleteCode = deleteCode;
		(window as any).__viewUsage = (id: string) => {
			const item = codes.find((c) => c.id === id);
			if (item) viewUsage(item);
		};
		return () => {
			delete (window as any).__editCode;
			delete (window as any).__deleteCode;
			delete (window as any).__viewUsage;
		};
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
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-gray-500">{t('common.loading')}</p>
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
			<label for="dc-code" class="mb-1 block text-sm font-medium text-gray-700">{t('discounts.code')}</label>
			<input id="dc-code" type="text" bind:value={code} required class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm uppercase placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
		</div>
		<div>
			<label for="dc-desc" class="mb-1 block text-sm font-medium text-gray-700">{t('discounts.description')}</label>
			<input id="dc-desc" type="text" bind:value={description} class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
			<label for="dc-type" class="mb-1 block text-sm font-medium text-gray-700">{t('discounts.type')}</label>
			<Select
				options={[
					{ value: 'percentage', label: t('discounts.percentage') },
					{ value: 'fixed', label: t('discounts.fixed') }
				]}
				bind:value={discountType}
			/>
			</div>
			<div>
				<label for="dc-value" class="mb-1 block text-sm font-medium text-gray-700">{t('discounts.value')}</label>
				<input id="dc-value" type="number" bind:value={discountValue} required class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="dc-from" class="mb-1 block text-sm font-medium text-gray-700">{t('discounts.valid_from')}</label>
				<input id="dc-from" type="date" bind:value={validFrom} class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
			</div>
			<div>
				<label for="dc-until" class="mb-1 block text-sm font-medium text-gray-700">{t('discounts.valid_until')}</label>
				<input id="dc-until" type="date" bind:value={validUntil} class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="dc-limit" class="mb-1 block text-sm font-medium text-gray-700">{t('discounts.usage_limit')}</label>
				<input id="dc-limit" type="number" bind:value={usageLimit} class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="dc-active" type="checkbox" bind:checked={isActive} class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
				<label for="dc-active" class="text-sm font-medium text-gray-700">{t('discounts.active')}</label>
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
		<p class="mb-3 text-sm text-gray-500">{t('discounts.usage_total')}: {usageData.total}</p>
		{#if usageData.users.length === 0}
			<p class="text-sm text-gray-500">{t('discounts.usage_empty')}</p>
		{:else}
			<div class="max-h-64 overflow-y-auto rounded-lg border border-gray-200">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-gray-200 bg-gray-50">
							<th class="px-4 py-2 text-left text-xs font-medium text-gray-500">{t('app.discount-codes.user')}</th>
							<th class="px-4 py-2 text-left text-xs font-medium text-gray-500">{t('discounts.used_at')}</th>
						</tr>
					</thead>
					<tbody>
						{#each usageData.users as u (u.user_id)}
							<tr class="border-b border-gray-200 last:border-0">
								<td class="px-4 py-2 font-mono text-xs">{u.user_id}</td>
								<td class="px-4 py-2 text-xs text-gray-500">{new Date(u.used_at).toLocaleString()}</td>
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
