<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import { Pencil, Plus, RefreshCw, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import type { DataTableColumn, DataTableRowAction } from '$lib/api/types';
	import { onMount } from 'svelte';

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
		position: 0,
		is_active: true,
		reason: ''
	});

	let confirmOpen = $state(false);
	let deleteTarget = $state<Currency | null>(null);
	let deleteReason = $state('');
	let deleteResultMessage = $state('');

	// All wallet currency endpoints require wallet.currency.manage
	// (server.go), including the list read.
	const canManage = $derived(hasAnyPermission(['wallet.currency.manage']));

	async function load() {
		loading = true;
		errorMessage = '';
		if (!canManage) {
			loading = false;
			return;
		}
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
		formData = { code: '', name: '', symbol: '', credits_per_unit: 0, position: 0, is_active: true, reason: '' };
		showFormModal = true;
	}

	function openEdit(c: Currency) {
		editingCurrency = c;
		formData = {
			code: c.code,
			name: c.name,
			symbol: c.symbol,
			credits_per_unit: c.credits_per_unit,
			position: c.position,
			is_active: c.is_active,
			reason: ''
		};
		showFormModal = true;
	}

	function closeFormModal() {
		showFormModal = false;
	}

	async function save() {
		saving = true;
		errorMessage = '';
		const body = {
			name: formData.name,
			symbol: formData.symbol,
			credits_per_unit: formData.credits_per_unit,
			position: formData.position,
			reason: formData.reason
		};
		if (editingCurrency) {
			const res = await api<Currency>(
				`/control/wallet/currencies/${editingCurrency.code}`,
				{ method: 'PATCH', body: { ...body, is_active: formData.is_active } }
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

	function requestDelete(c: Currency) {
		deleteTarget = c;
		deleteReason = '';
		deleteResultMessage = '';
		confirmOpen = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		if (!deleteReason.trim()) return;
		const res = await api<{ spaces_affected: number }>(
			`/control/wallet/currencies/${deleteTarget.code}`,
			{ method: 'DELETE', body: { reason: deleteReason } }
		);
		if (res.ok) {
			const affected = res.data?.spaces_affected ?? 0;
			deleteResultMessage = t('currencies.deactivated_with_spaces', '{count} spaces were using this currency.').replace('{count}', String(affected));
			confirmOpen = false;
			deleteTarget = null;
			deleteReason = '';
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
				const label = isActive ? t('currencies.status_active', 'Active') : t('currencies.status_inactive', 'Inactive');
				const cls = isActive
					? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
					: 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400';
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${cls}">${escapeHtml(label)}</span>`;
			}
		},
	];

	const rowActions: DataTableRowAction[] = [
		{
			label: t('currencies.edit', 'Edit Currency'),
			icon: Pencil,
			onClick: (row) => {
				const c = currencies.find((x) => x.code === (row as Record<string, unknown>).code);
				if (c) openEdit(c);
			},
			hidden: () => !canManage
		},
		{
			label: t('currencies.deactivate', 'Deactivate Currency'),
			icon: Trash2,
			variant: 'destructive',
			hidden: (row) => !canManage || !(row as Record<string, unknown>).is_active,
			onClick: (row) => {
				const c = currencies.find((x) => x.code === (row as Record<string, unknown>).code);
				if (c) requestDelete(c);
			}
		}
	];

	onMount(load);
</script>

<svelte:head>
	<title>{t('currencies.title', 'Currencies')} — Control Panel</title>
</svelte:head>

{#if !canManage}
	<div class="p-8 text-center text-sm text-[var(--color-muted)]">
		{t('common.access_denied', 'Access denied')}
	</div>
{:else}
<PageHeader title={t('currencies.title', 'Currencies')} description={t('currencies.description', 'Manage fiat currency exchange rates for credit conversion')} />

<div class="space-y-4">
	{#if errorMessage}
		<div class="border-destructive bg-destructive/10 text-destructive rounded-lg border p-3 text-sm">
			{errorMessage}
		</div>
	{/if}

	{#if deleteResultMessage}
		<div class="border-border-card bg-muted/30 text-foreground rounded-lg border p-3 text-sm">
			{deleteResultMessage}
		</div>
	{/if}

	<div class="flex justify-end gap-2">
		<button class="border-border-input bg-background-alt text-foreground hover:bg-muted inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm transition-colors" onclick={load}>
			<RefreshCw class="size-4" />
			{t('common.refresh', 'Refresh')}
		</button>
		{#if canManage}
			<button class="bg-foreground text-background hover:bg-foreground/90 inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors" onclick={openCreate}>
				<Plus class="size-4" />
				{t('currencies.add', 'Add Currency')}
			</button>
		{/if}
	</div>

	{#if loading}
		<div class="text-muted-foreground flex items-center justify-center py-12">
			{t('common.loading', 'Loading...')}
		</div>
	{:else if currencies.length === 0}
		<div class="text-muted-foreground flex flex-col items-center justify-center py-12">
			<p>{t('currencies.empty', 'No currencies configured.')}</p>
		</div>
	{:else}
		<DataTable
			{columns}
			rows={currencies as unknown as Record<string, unknown>[]}
			totalRows={currencies.length}
			{loading}
			{rowActions}
			pageSize={20}
			emptyMessage={t('currencies.empty', 'No currencies configured.')}
			getRowId={(row) => (row as Record<string, unknown>).code as string}
		/>
	{/if}
</div>
{/if}

<!-- Create/Edit Modal -->
<Modal
	bind:open={showFormModal}
	onOpenChange={(isOpen) => { if (!isOpen) closeFormModal(); }}
	title={editingCurrency ? t('currencies.edit', 'Edit Currency') : t('currencies.add', 'Add Currency')}
>
	{#snippet children(close)}
			<div class="space-y-4">
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium" for="curr-code">{t('currencies.col_code', 'Code')}</label>
					<HintIcon text={t('currencies.code_hint', 'ISO code — e.g. NGN, USD.')} />
					<input
						id="curr-code"
						type="text"
						maxlength="3"
						bind:value={formData.code}
						disabled={!!editingCurrency}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 w-full rounded-md border px-3 py-2 text-sm uppercase transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
						placeholder="USD"
					/>
					<HintIcon text={t('currencies.code_hint', 'ISO 4217 — 3 letter code')} />
				</div>
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium" for="curr-name">{t('currencies.col_name', 'Name')}</label>
					<HintIcon text={t('currencies.name_hint', 'The display name of the currency.')} />
					<input
						id="curr-name"
						type="text"
						bind:value={formData.name}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="US Dollar"
					/>
				</div>
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium" for="curr-symbol">{t('currencies.col_symbol', 'Symbol')}</label>
					<HintIcon text={t('currencies.symbol_hint', 'The symbol shown next to amounts — e.g. ₦, $.')} />
					<input
						id="curr-symbol"
						type="text"
						bind:value={formData.symbol}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="$"
					/>
				</div>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label class="text-foreground mb-1 block text-sm font-medium" for="curr-rate">{t('currencies.col_rate', 'Credits per Unit')}</label>
						<HintIcon text={t('currencies.rate_hint', 'Wallet credits granted per unit of this currency.')} />
						<input
							id="curr-rate"
							type="number"
							min="1"
							bind:value={formData.credits_per_unit}
							class="border-border-input bg-background text-foreground w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						/>
						<p class="text-muted-foreground mt-1 text-xs">{t('currencies.rate_hint', '1 {code} = N credits').replace('{code}', formData.code || 'XXX')}</p>
					</div>
					<div>
						<label class="text-foreground mb-1 block text-sm font-medium" for="curr-pos">{t('currencies.col_position', 'Position')}</label>
						<HintIcon text={t('currencies.pos_hint', 'Ordering position — lower numbers show first.')} />
						<input
							id="curr-pos"
							type="number"
							min="0"
							bind:value={formData.position}
							class="border-border-input bg-background text-foreground w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						/>
					</div>
				</div>
				{#if editingCurrency}
					<div class="flex items-center gap-2">
						<input
							id="curr-active"
							type="checkbox"
							bind:checked={formData.is_active}
							class="border-border-input bg-background size-4 rounded"
						/>
						<label class="text-foreground text-sm font-medium" for="curr-active">{t('currencies.col_status', 'Status')} — {formData.is_active ? t('currencies.status_active', 'Active') : t('currencies.status_inactive', 'Inactive')}</label>
						<HintIcon text={t('currencies.active_hint', 'Inactive currencies are hidden from pickers.')} />
					</div>
				{/if}
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium" for="curr-reason">{t('currencies.reason', 'Reason')} <span class="text-destructive">*</span></label>
					<textarea
						id="curr-reason"
						bind:value={formData.reason}
						rows="2"
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder={t('currencies.reason_placeholder', 'Why is this currency being added/modified?')}
					></textarea>
				</div>
				{#if errorMessage}
					<p class="text-destructive text-sm">{errorMessage}</p>
				{/if}
				<div class="flex justify-end gap-2 pt-2">
					<button
						type="button"
						class="border-border-input bg-background-alt text-foreground hover:bg-muted rounded-md border px-4 py-2 text-sm transition-colors"
						onclick={close}
					>
						{t('common.cancel', 'Cancel')}
					</button>
					<button
						class="bg-foreground text-background hover:bg-foreground/90 rounded-md px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
						onclick={save}
						disabled={saving || !formData.name || !formData.symbol || formData.credits_per_unit <= 0 || (!editingCurrency && !formData.code) || !formData.reason.trim()}
					>
						{saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
					</button>
				</div>
			</div>
	{/snippet}
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={confirmOpen} onOpenChange={(isOpen) => { if (!isOpen) { deleteReason = ''; } }} title={t('currencies.deactivate', 'Deactivate Currency')}>
	{#snippet children(close)}
			<div class="bg-destructive/10 mt-3 rounded-md p-3">
				<p class="text-destructive text-sm">{t('currencies.deactivate_confirm', 'This will hide the currency from new purchases. Historical payment records are preserved.')}</p>
			</div>
			<div class="mt-4">
				<label class="text-foreground mb-1 block text-sm font-medium" for="delete-reason">{t('currencies.reason', 'Reason')} <span class="text-destructive">*</span></label>
				<textarea
					id="delete-reason"
					bind:value={deleteReason}
					rows="2"
					class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					placeholder={t('currencies.deactivate_reason_placeholder', 'Why is this currency being deactivated?')}
				></textarea>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<button
					type="button"
					class="border-border-input bg-background-alt text-foreground hover:bg-muted rounded-md border px-4 py-2 text-sm transition-colors"
					onclick={close}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					class="bg-destructive text-white hover:bg-destructive/90 rounded-md px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={!deleteReason.trim()}
					onclick={confirmDelete}
				>
					{t('common.deactivate', 'Deactivate')}
				</button>
			</div>
	{/snippet}
</Modal>
