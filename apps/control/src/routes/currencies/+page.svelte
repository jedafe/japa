<script lang="ts">
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Pencil, Plus, X, RefreshCw, Trash2 } from 'lucide-svelte';
	import type { DataTableColumn } from '$lib/api/types';

	type Currency = {
		code: string;
		name: string;
		symbol: string;
		credits_per_unit: number;
		is_active: boolean;
		position: number;
	};

	let currencies = $state<Currency[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let saving = $state(false);

	let showFormModal = $state(false);
	let editingCurrency = $state<Currency | null>(null);
	let formData = $state({
		code: '',
		name: '',
		symbol: '',
		credits_per_unit: 0,
		position: 0
	});

	let confirmOpen = $state(false);
	let deleteTarget = $state<Currency | null>(null);
	let deleteSpacesAffected = $state(0);

	async function load() {
		loading = true;
		errorMessage = '';
		const res = await api<Currency[]>('/control/wallet/currencies');
		if (res.ok) {
			currencies = res.data ?? [];
		} else {
			errorMessage = res.error;
		}
		loading = false;
	}

	function openCreate() {
		editingCurrency = null;
		formData = { code: '', name: '', symbol: '', credits_per_unit: 0, position: 0 };
		showFormModal = true;
	}

	function openEdit(c: Currency) {
		editingCurrency = c;
		formData = {
			code: c.code,
			name: c.name,
			symbol: c.symbol,
			credits_per_unit: c.credits_per_unit,
			position: c.position
		};
		showFormModal = true;
	}

	async function save() {
		saving = true;
		errorMessage = '';
		const body = {
			name: formData.name,
			symbol: formData.symbol,
			credits_per_unit: formData.credits_per_unit,
			position: formData.position
		};
		if (editingCurrency) {
			const res = await api<Currency>(
				`/control/wallet/currencies/${editingCurrency.code}`,
				{ method: 'PATCH', body }
			);
			if (!res.ok) errorMessage = res.error;
		} else {
			const res = await api<Currency>('/control/wallet/currencies', {
				method: 'POST',
				body: { ...body, code: formData.code.toUpperCase() }
			});
			if (!res.ok) errorMessage = res.error;
		}
		saving = false;
		if (!errorMessage) {
			showFormModal = false;
			await load();
		}
	}

	async function requestDelete(c: Currency) {
		deleteTarget = c;
		// The delete endpoint returns spaces_affected in the response,
		// but we can also estimate from the loaded currencies.
		deleteSpacesAffected = 0;
		confirmOpen = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		const res = await api<{ spaces_affected: number }>(
			`/control/wallet/currencies/${deleteTarget.code}`,
			{ method: 'DELETE' }
		);
		if (res.ok) {
			deleteSpacesAffected = res.data?.spaces_affected ?? 0;
			confirmOpen = false;
			deleteTarget = null;
			await load();
		} else {
			errorMessage = res.error;
			confirmOpen = false;
		}
	}

	const columns: DataTableColumn[] = [
		{
			key: 'code',
			label: t('currencies.col_code', 'Code'),
			sortable: true,
			render: (v: unknown) => `<span class="font-mono font-semibold">${escapeHtml(String(v))}</span>`
		},
		{
			key: 'name',
			label: t('currencies.col_name', 'Name'),
			sortable: true,
			render: (v: unknown) => escapeHtml(String(v))
		},
		{
			key: 'symbol',
			label: t('currencies.col_symbol', 'Symbol'),
			render: (v: unknown) => `<span class="text-lg">${escapeHtml(String(v))}</span>`
		},
		{
			key: 'credits_per_unit',
			label: t('currencies.col_rate', 'Credits per Unit'),
			sortable: true,
			render: (v: unknown) => `<span class="font-semibold">${Number(v).toLocaleString()}</span>`
		},
		{
			key: 'position',
			label: t('currencies.col_position', 'Position'),
			sortable: true,
			render: (v: unknown) => String(v)
		},
		{
			key: 'is_active',
			label: t('currencies.col_status', 'Status'),
			render: (_: unknown, row: Record<string, unknown>) => {
				const isActive = row.is_active as boolean;
				const label = isActive ? 'Active' : 'Inactive';
				const cls = isActive
					? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
					: 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400';
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${cls}">${label}</span>`;
			}
		},
		{
			key: '_actions',
			label: t('common.actions', 'Actions'),
			render: (_: unknown, row: Record<string, unknown>) => {
				const code = row.code as string;
				const isActive = row.is_active as boolean;
				const edit = `<button class="p-1 text-blue-600 hover:text-blue-800" data-action="edit" data-code="${code}">✏</button>`;
				const del = isActive
					? `<button class="p-1 text-red-600 hover:text-red-800" data-action="delete" data-code="${code}">🗑</button>`
					: '';
				return `<div class="flex gap-1">${edit}${del}</div>`;
			}
		}
	];

	function handleTableClick(e: MouseEvent) {
		const target = e.target as HTMLElement;
		const btn = target.closest('[data-action]') as HTMLElement | null;
		if (!btn) return;
		const code = btn.dataset.code;
		const action = btn.dataset.action;
		const c = currencies.find((x) => x.code === code);
		if (!c) return;
		if (action === 'edit') openEdit(c);
		else if (action === 'delete') requestDelete(c);
	}

	load();
</script>

<svelte:head>
	<title>{t('currencies.title', 'Currencies')} — Control Panel</title>
</svelte:head>

<PageHeader title={t('currencies.title', 'Currencies')} description={t('currencies.description', 'Manage fiat currency exchange rates for credit conversion')} />

<div class="space-y-4">
	{#if errorMessage}
		<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
			{errorMessage}
		</div>
	{/if}

	<div class="flex justify-end gap-2">
		<button class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm hover:bg-gray-50 dark:hover:bg-gray-800" onclick={load}>
			<RefreshCw class="size-4" />
			{t('common.refresh', 'Refresh')}
		</button>
		<button class="inline-flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700" onclick={openCreate}>
			<Plus class="size-4" />
			{t('currencies.add', 'Add Currency')}
		</button>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12 text-gray-500">
			{t('common.loading', 'Loading...')}
		</div>
	{:else if currencies.length === 0}
		<div class="flex flex-col items-center justify-center py-12 text-gray-500">
			<p>{t('currencies.empty', 'No currencies configured.')}</p>
		</div>
	{:else}
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div role="presentation" onclick={handleTableClick}>
			<DataTable
				{columns}
				rows={currencies as unknown as Record<string, unknown>[]}
				totalRows={currencies.length}
				{loading}
				pageSize={20}
				emptyMessage={t('currencies.empty', 'No currencies configured.')}
			/>
		</div>
	{/if}
</div>

<!-- Create/Edit Modal -->
{#if showFormModal}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" role="dialog" aria-modal="true">
		<div class="w-full max-w-md rounded-lg bg-white p-6 dark:bg-gray-900">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-lg font-semibold">
					{editingCurrency ? t('currencies.edit', 'Edit Currency') : t('currencies.add', 'Add Currency')}
				</h2>
				<button class="text-gray-400 hover:text-gray-600" onclick={() => (showFormModal = false)}>
					<X class="size-5" />
				</button>
			</div>
			<div class="space-y-4">
				<div>
					<label class="mb-1 block text-sm font-medium" for="curr-code">{t('currencies.col_code', 'Code')}</label>
					<input
						id="curr-code"
						type="text"
						maxlength="3"
						bind:value={formData.code}
						disabled={!!editingCurrency}
						class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm uppercase disabled:bg-gray-100 dark:border-gray-700 dark:bg-gray-800"
						placeholder="USD"
					/>
					<p class="mt-1 text-xs text-gray-500">ISO 4217 — 3 letter code</p>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="curr-name">{t('currencies.col_name', 'Name')}</label>
					<input
						id="curr-name"
						type="text"
						bind:value={formData.name}
						class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-800"
						placeholder="US Dollar"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="curr-symbol">{t('currencies.col_symbol', 'Symbol')}</label>
					<input
						id="curr-symbol"
						type="text"
						bind:value={formData.symbol}
						class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-800"
						placeholder="$"
					/>
				</div>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label class="mb-1 block text-sm font-medium" for="curr-rate">{t('currencies.col_rate', 'Credits per Unit')}</label>
						<input
							id="curr-rate"
							type="number"
							min="1"
							bind:value={formData.credits_per_unit}
							class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-800"
						/>
						<p class="mt-1 text-xs text-gray-500">1 {formData.code || 'XXX'} = N credits</p>
					</div>
					<div>
						<label class="mb-1 block text-sm font-medium" for="curr-pos">{t('currencies.col_position', 'Position')}</label>
						<input
							id="curr-pos"
							type="number"
							min="0"
							bind:value={formData.position}
							class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-800"
						/>
					</div>
				</div>
				{#if errorMessage}
					<p class="text-sm text-red-600">{errorMessage}</p>
				{/if}
				<div class="flex justify-end gap-2 pt-2">
					<button class="rounded-md border px-4 py-2 text-sm hover:bg-gray-50 dark:hover:bg-gray-800" onclick={() => (showFormModal = false)}>
						{t('common.cancel', 'Cancel')}
					</button>
					<button
						class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
						onclick={save}
						disabled={saving || !formData.name || !formData.symbol || formData.credits_per_unit <= 0 || (!editingCurrency && !formData.code)}
					>
						{saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<!-- Delete Confirmation -->
<ConfirmModal
	bind:open={confirmOpen}
	title={t('currencies.deactivate', 'Deactivate Currency')}
	consequence={t('currencies.deactivate_confirm', 'This will hide the currency from new purchases. Historical payment records are preserved.')}
	confirmText={t('common.deactivate', 'Deactivate')}
	onConfirm={confirmDelete}
/>
