<script lang="ts">
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Plus, X, RefreshCw } from 'lucide-svelte';
	import { Dialog } from 'bits-ui';
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
		position: 0,
		is_active: true,
		reason: ''
	});

	let confirmOpen = $state(false);
	let deleteTarget = $state<Currency | null>(null);
	let deleteReason = $state('');
	let deleteResultMessage = $state('');

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
		{
			key: '_actions',
			label: t('common.actions', 'Actions'),
			render: (_: unknown, row: Record<string, unknown>) => {
				const code = row.code as string;
				const isActive = row.is_active as boolean;
				const editLabel = t('currencies.edit', 'Edit Currency');
				const edit = `<button class="text-foreground-alt hover:text-foreground p-1" data-action="edit" data-code="${escapeHtml(code)}" aria-label="${escapeHtml(editLabel)}" title="${escapeHtml(editLabel)}"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/></svg></button>`;
				const delLabel = t('currencies.deactivate', 'Deactivate Currency');
				const del = isActive
					? `<button class="text-destructive hover:text-destructive/80 p-1" data-action="delete" data-code="${escapeHtml(code)}" aria-label="${escapeHtml(delLabel)}" title="${escapeHtml(delLabel)}"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg></button>`
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
		<button class="bg-foreground text-background hover:bg-foreground/90 inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors" onclick={openCreate}>
			<Plus class="size-4" />
			{t('currencies.add', 'Add Currency')}
		</button>
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
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div role="presentation" onclick={handleTableClick}>
			<DataTable
				{columns}
				rows={currencies as unknown as Record<string, unknown>[]}
				totalRows={currencies.length}
				{loading}
				pageSize={20}
				emptyMessage={t('currencies.empty', 'No currencies configured.')}
				getRowId={(row) => (row as Record<string, unknown>).code as string}
			/>
		</div>
	{/if}
</div>

<!-- Create/Edit Modal -->
<Dialog.Root bind:open={showFormModal} onOpenChange={(isOpen) => { if (!isOpen) closeFormModal(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 focus:outline-none"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<div class="mb-4 flex items-center justify-between">
				<Dialog.Title class="text-foreground text-lg font-semibold">
					{editingCurrency ? t('currencies.edit', 'Edit Currency') : t('currencies.add', 'Add Currency')}
				</Dialog.Title>
				<Dialog.Close class="text-muted-foreground hover:text-foreground rounded-9px p-1.5 transition-colors">
					<X class="size-4" />
				</Dialog.Close>
			</div>
			<div class="space-y-4">
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium" for="curr-code">{t('currencies.col_code', 'Code')}</label>
					<input
						id="curr-code"
						type="text"
						maxlength="3"
						bind:value={formData.code}
						disabled={!!editingCurrency}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 w-full rounded-md border px-3 py-2 text-sm uppercase transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
						placeholder="USD"
					/>
					<p class="text-muted-foreground mt-1 text-xs">{t('currencies.code_hint', 'ISO 4217 — 3 letter code')}</p>
				</div>
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium" for="curr-name">{t('currencies.col_name', 'Name')}</label>
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
					<Dialog.Close class="border-border-input bg-background-alt text-foreground hover:bg-muted rounded-md border px-4 py-2 text-sm transition-colors">
						{t('common.cancel', 'Cancel')}
					</Dialog.Close>
					<button
						class="bg-foreground text-background hover:bg-foreground/90 rounded-md px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
						onclick={save}
						disabled={saving || !formData.name || !formData.symbol || formData.credits_per_unit <= 0 || (!editingCurrency && !formData.code) || !formData.reason.trim()}
					>
						{saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
					</button>
				</div>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Delete Confirmation -->
<Dialog.Root bind:open={confirmOpen} onOpenChange={(isOpen) => { if (!isOpen) { deleteReason = ''; } }}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 focus:outline-none"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">{t('currencies.deactivate', 'Deactivate Currency')}</Dialog.Title>
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
				<Dialog.Close class="border-border-input bg-background-alt text-foreground hover:bg-muted rounded-md border px-4 py-2 text-sm transition-colors">
					{t('common.cancel', 'Cancel')}
				</Dialog.Close>
				<button
					class="bg-destructive text-white hover:bg-destructive/90 rounded-md px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={!deleteReason.trim()}
					onclick={confirmDelete}
				>
					{t('common.deactivate', 'Deactivate')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
