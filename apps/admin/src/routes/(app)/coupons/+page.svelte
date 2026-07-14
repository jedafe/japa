<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Plus, Pencil, Trash2, Tag } from 'lucide-svelte';

	type Coupon = {
		id: string;
		code: string;
		discount_type: string;
		discount_value: number;
		is_active: boolean;
		valid_from?: string;
		valid_until?: string;
		usage_limit?: number;
		used_count: number;
		created_at: string;
	};

	let coupons = $state<Coupon[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingCoupon = $state<Coupon | null>(null);
	let saving = $state(false);

	let code = $state('');
	let discountType = $state('percentage');
	let discountValue = $state('');
	let validFrom = $state('');
	let validUntil = $state('');
	let usageLimit = $state('');
	let isActive = $state(true);

	let activeTab = $state('active');
	let filters = $state<Record<string, string>>({});

	const tabs = $derived([
		{ value: 'active', label: 'Active', count: coupons.filter((c) => c.is_active).length },
		{ value: 'inactive', label: 'Inactive', count: coupons.filter((c) => !c.is_active).length },
		{ value: 'all', label: 'All', count: coupons.length }
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
					<button onclick="window.__editCoupon('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
					<button onclick="window.__deleteCoupon('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
				</div>
			`
		}
	];

	let filteredCoupons = $derived.by(() => {
		let result = [...coupons];
		if (activeTab === 'active') result = result.filter((c) => c.is_active);
		else if (activeTab === 'inactive') result = result.filter((c) => !c.is_active);
		if (filters.type) result = result.filter((c) => c.discount_type === filters.type);
		return result;
	});

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Coupon[]>('/admin/coupons');
			if (result.ok) {
				coupons = result.data ?? [];
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
		discountType = 'percentage';
		discountValue = '';
		validFrom = '';
		validUntil = '';
		usageLimit = '';
		isActive = true;
		editingCoupon = null;
		showCreateModal = true;
	}

	function openEdit(coupon: Coupon) {
		editingCoupon = coupon;
		code = coupon.code;
		discountType = coupon.discount_type;
		discountValue = String(coupon.discount_value);
		validFrom = coupon.valid_from || '';
		validUntil = coupon.valid_until || '';
		usageLimit = coupon.usage_limit ? String(coupon.usage_limit) : '';
		isActive = coupon.is_active;
		showCreateModal = true;
	}

	async function saveCoupon() {
		if (!code.trim() || !discountValue) return;
		saving = true;
		errorMessage = '';
		try {
			const body: Record<string, unknown> = {
				code: code.trim().toUpperCase(),
				discount_type: discountType,
				discount_value: Number(discountValue),
				is_active: isActive
			};
			if (validFrom) body.valid_from = validFrom;
			if (validUntil) body.valid_until = validUntil;
			if (usageLimit) body.usage_limit = Number(usageLimit);

			const url = editingCoupon ? `/admin/coupons/${editingCoupon.id}` : '/admin/coupons';
			const method = editingCoupon ? 'PATCH' : 'POST';
			const result = await api(url, { method, body });
			if (result.ok) {
				showCreateModal = false;
				editingCoupon = null;
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

	async function deleteCoupon(id: string) {
		errorMessage = '';
		try {
			const result = await api(`/admin/coupons/${id}`, { method: 'DELETE' });
			if (result.ok) {
				coupons = coupons.filter((c) => c.id !== id);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	// Expose functions for DataTable inline actions
	if (typeof window !== 'undefined') {
		(window as any).__editCoupon = openEdit;
		(window as any).__deleteCoupon = deleteCoupon;
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('coupons.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('coupons.title')}
	description={t('coupons.description')}
	actions={[
		{ label: t('coupons.create'), icon: Plus, onclick: openCreate }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[
			{
				key: 'type',
				label: 'Type',
				options: [
					{ value: 'percentage', label: 'Percentage' },
					{ value: 'fixed', label: 'Fixed Amount' }
				]
			}
		]}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-gray-500">{t('common.loading')}</p>
			{:else}
				<DataTable
					columns={filteredCoupons.length > 0 ? columns : []}
					data={filteredCoupons}
					pageSize={20}
					selectable={true}
					emptyMessage={t('coupons.empty')}
				/>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create/Edit Modal -->
<Modal bind:open={showCreateModal} title={editingCoupon ? t('coupons.edit') : t('coupons.create')}>
	<div class="space-y-4">
		<div>
			<label for="cp-code" class="mb-1 block text-sm font-medium text-gray-700">{t('coupons.code')}</label>
			<input id="cp-code" type="text" bind:value={code} required class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm uppercase placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="cp-type" class="mb-1 block text-sm font-medium text-gray-700">{t('coupons.type')}</label>
				<select id="cp-type" bind:value={discountType} class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2">
					<option value="percentage">{t('coupons.percentage')}</option>
					<option value="fixed">{t('coupons.fixed')}</option>
				</select>
			</div>
			<div>
				<label for="cp-value" class="mb-1 block text-sm font-medium text-gray-700">{t('coupons.value')}</label>
				<input id="cp-value" type="number" bind:value={discountValue} required class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="cp-from" class="mb-1 block text-sm font-medium text-gray-700">{t('coupons.valid_from')}</label>
				<input id="cp-from" type="date" bind:value={validFrom} class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
			</div>
			<div>
				<label for="cp-until" class="mb-1 block text-sm font-medium text-gray-700">{t('coupons.valid_until')}</label>
				<input id="cp-until" type="date" bind:value={validUntil} class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="cp-limit" class="mb-1 block text-sm font-medium text-gray-700">{t('coupons.usage_limit')}</label>
				<input id="cp-limit" type="number" bind:value={usageLimit} class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2" />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="cp-active" type="checkbox" bind:checked={isActive} class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
				<label for="cp-active" class="text-sm font-medium text-gray-700">{t('coupons.active')}</label>
			</div>
		</div>
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" onclick={() => showCreateModal = false}>{t('common.cancel')}</Button>
			<Button onclick={saveCoupon} disabled={saving}>
				{saving ? t('common.saving') : editingCoupon ? t('common.save') : t('common.create')}
			</Button>
		</div>
	</div>
</Modal>
