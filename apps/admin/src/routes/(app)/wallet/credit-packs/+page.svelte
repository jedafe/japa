<script lang="ts">
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	import { t } from '$lib/stores/i18n.svelte';
	import { walletApi } from '$lib/api';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Plus, Pencil, Trash2, X } from 'lucide-svelte';

	type CreditPack = {
		id: string;
		name: string;
		description: string | null;
		credit_amount: number;
		bonus_credits: number;
		min_user_level: string | null;
		is_active: boolean;
		position: number;
	};

	let packs = $state<CreditPack[]>([]);
	let loading = $state(true);
	let showForm = $state(false);
	let editingPack = $state<CreditPack | null>(null);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let deleteTarget = $state<CreditPack | null>(null);
	let deleteModalOpen = $state(false);
	let deleting = $state(false);

	let formData = $state({
		name: '',
		description: '',
		credit_amount: 0,
		bonus_credits: 0,
		min_user_level: '',
		position: 0
	});

	async function loadPacks() {
		loading = true;
		errorMessage = '';
		try {
			const res = await walletApi.listCreditPacks();
			packs = (res as { data: CreditPack[] }).data ?? [];
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : 'Failed to load credit packs';
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingPack = null;
		formData = { name: '', description: '', credit_amount: 0, bonus_credits: 0, min_user_level: '', position: 0 };
		showForm = true;
	}

	function openEdit(pack: CreditPack) {
		editingPack = pack;
		formData = {
			name: pack.name,
			description: pack.description ?? '',
			credit_amount: pack.credit_amount,
			bonus_credits: pack.bonus_credits,
			min_user_level: pack.min_user_level ?? '',
			position: pack.position
		};
		showForm = true;
	}

	async function savePack() {
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const body = {
				name: formData.name,
				description: formData.description || undefined,
				credit_amount: formData.credit_amount,
				bonus_credits: formData.bonus_credits,
				min_user_level: formData.min_user_level || undefined,
				position: formData.position
			};
			if (editingPack) {
				await walletApi.updateCreditPack(editingPack.id, { ...body, is_active: editingPack.is_active });
				successMessage = 'Pack updated successfully';
			} else {
				await walletApi.createCreditPack(body);
				successMessage = 'Pack created successfully';
			}
			showForm = false;
			await loadPacks();
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : 'Failed to save pack';
		} finally {
			saving = false;
		}
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		deleting = true;
		errorMessage = '';
		try {
			await walletApi.deleteCreditPack(deleteTarget.id);
			successMessage = 'Pack deactivated successfully';
			deleteTarget = null;
			deleteModalOpen = false;
			await loadPacks();
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : 'Failed to deactivate pack';
		} finally {
			deleting = false;
		}
	}

	const packColumns = [
		{ key: 'name', label: t('app.wallet.pack_name', 'Pack Name'), sortable: true,
			render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: 'credit_amount', label: t('app.wallet.credit_amount', 'Credit Amount'), sortable: true,
			render: (v: number) => v.toLocaleString() },
		{ key: 'bonus_credits', label: t('app.wallet.bonus_credits', 'Bonus Credits'),
			render: (v: number) =>
				v > 0
					? `<span class="text-success font-semibold">+${v.toLocaleString()}</span>`
					: '<span class="text-muted">—</span>' },
		{ key: 'total', label: t('app.wallet.total_credits', 'Total Credits'),
			render: (_: unknown, row: CreditPack) => (row.credit_amount + row.bonus_credits).toLocaleString() },
		{ key: 'min_user_level', label: t('app.wallet.min_user_level', 'Min User Level'),
			render: (v: string | null) => v ?? '<span class="text-muted">—</span>' },
		{ key: 'position', label: t('app.wallet.position', 'Position'), sortable: true,
			render: (v: number) => String(v) },
		{ key: 'is_active', label: 'Status',
			render: (_: unknown, row: CreditPack) => {
				const label = row.is_active ? 'Active' : 'Inactive';
				const cls = row.is_active
					? 'bg-success/15 text-success border-success'
					: 'bg-surface text-foreground border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${label}</span>`;
			} },
		{ key: '_actions', label: t('common.actions', 'Actions'),
			render: (_: unknown, row: CreditPack) => {
				const editBtn = `<button class="p-1 hover:text-primary" data-action="edit" data-id="${row.id}">✏</button>`;
				const delBtn = row.is_active
					? `<button class="p-1 hover:text-danger" data-action="delete" data-id="${row.id}">🗑</button>`
					: '';
				return `<div class="flex gap-1">${editBtn}${delBtn}</div>`;
			} }
	];

	function handleTableClick(e: MouseEvent) {
		const target = e.target as HTMLElement;
		const actionEl = target.closest('[data-action]') as HTMLElement | null;
		if (!actionEl) return;
		const id = actionEl.dataset.id;
		const action = actionEl.dataset.action;
		const pack = packs.find((p) => p.id === id);
		if (!pack) return;
		if (action === 'edit') openEdit(pack);
		else if (action === 'delete') { deleteTarget = pack; deleteModalOpen = true; }
	}

	loadPacks();
</script>

<svelte:head>
	<title>{t('app.wallet.credit_packs_title', 'Credit Pack Management')} — Admin</title>
</svelte:head>

<PageHeader
	title={t('app.wallet.credit_packs_title', 'Credit Pack Management')}
	description={t('app.wallet.credit_packs_desc', 'Create and manage credit bundles with optional bonus credits')}
	actions={[
		{ label: t('app.wallet.add_pack', 'Add Pack'), icon: Plus, variant: 'default', onclick: openCreate }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="rounded-lg border border-success bg-success-50 px-4 py-3 text-sm text-success-700">
			{successMessage}
		</div>
	{/if}

	<Card>
		<CardContent>
			{#if loading}
				<div class="flex items-center justify-center py-12 text-muted">
					Loading...
				</div>
			{:else if packs.length === 0}
				<div class="flex flex-col items-center justify-center py-12 text-muted">
					<p>No credit packs yet. Create one to get started.</p>
				</div>
			{:else}
				<!-- svelte-ignore a11y_click_events_have_key_events -->
				<div role="presentation" onclick={handleTableClick}>
					<DataTable columns={packColumns} data={packs} pageSize={20} searchable
						emptyMessage="No credit packs found" />
				</div>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create/Edit Modal -->
<Modal bind:open={showForm} title={editingPack ? t('app.wallet.edit_pack', 'Edit Pack') : t('app.wallet.add_pack', 'Add Pack')}>
	<div class="space-y-4">
		<div>
			<label class="mb-1 block text-sm font-medium" for="pack-name">
				{t('app.wallet.pack_name', 'Pack Name')}
			</label>
			<input
				id="pack-name"
				type="text"
				bind:value={formData.name}
				class="w-full rounded-md border border-input bg-surface px-3 py-2 text-sm"
				placeholder="e.g. Starter Pack"
			/>
		</div>
		<div>
			<label class="mb-1 block text-sm font-medium" for="pack-desc">
				{t('app.wallet.pack_description', 'Description')}
			</label>
			<textarea
				id="pack-desc"
				bind:value={formData.description}
				class="w-full rounded-md border border-input bg-surface px-3 py-2 text-sm"
				rows="2"
				placeholder="Optional description shown to users"
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label class="mb-1 block text-sm font-medium" for="pack-credits">
					{t('app.wallet.credit_amount', 'Credit Amount')}
				</label>
				<input
					id="pack-credits"
					type="number"
					min="1"
					bind:value={formData.credit_amount}
					class="w-full rounded-md border border-input bg-surface px-3 py-2 text-sm"
				/>
			</div>
			<div>
				<label class="mb-1 block text-sm font-medium" for="pack-bonus">
					{t('app.wallet.bonus_credits', 'Bonus Credits')}
				</label>
				<input
					id="pack-bonus"
					type="number"
					min="0"
					bind:value={formData.bonus_credits}
					class="w-full rounded-md border border-input bg-surface px-3 py-2 text-sm"
				/>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label class="mb-1 block text-sm font-medium" for="pack-level">
					{t('app.wallet.min_user_level', 'Min User Level')}
				</label>
				<input
					id="pack-level"
					type="text"
					bind:value={formData.min_user_level}
					class="w-full rounded-md border border-input bg-surface px-3 py-2 text-sm"
					placeholder="e.g. pro (optional)"
				/>
			</div>
			<div>
				<label class="mb-1 block text-sm font-medium" for="pack-pos">
					{t('app.wallet.position', 'Position')}
				</label>
				<input
					id="pack-pos"
					type="number"
					min="0"
					bind:value={formData.position}
					class="w-full rounded-md border border-input bg-surface px-3 py-2 text-sm"
				/>
			</div>
		</div>
		{#if errorMessage}
			<p class="text-sm text-danger">{errorMessage}</p>
		{/if}
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" onclick={() => (showForm = false)}>
				<X class="size-4" />
				Cancel
			</Button>
			<Button onclick={savePack} disabled={saving || formData.credit_amount <= 0 || !formData.name}>
				{saving ? 'Saving...' : 'Save'}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteModalOpen} title={t('app.wallet.deactivate_pack', 'Deactivate Pack')}>
	<div class="space-y-4">
		<p class="text-sm text-muted">
			{t('app.wallet.deactivate_pack_confirm', 'Are you sure you want to deactivate this credit pack?')}
		</p>
		<p class="rounded-md bg-muted p-3 text-sm font-medium">
			{deleteTarget?.name} — {deleteTarget ? (deleteTarget.credit_amount + deleteTarget.bonus_credits).toLocaleString() : ''} credits
		</p>
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { deleteTarget = null; deleteModalOpen = false; }}>Cancel</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={deleting}>
				{deleting ? 'Deactivating...' : 'Deactivate'}
			</Button>
		</div>
	</div>
</Modal>
