<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from "$lib/components/ui/data-table";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Button } from '$lib/components/ui/button';
	import { Banknote, Plus, RefreshCw, Pencil, Download } from 'lucide-svelte';
	import { api } from '$lib/api';

	type RemittanceProvider = {
		id: string;
		name: string;
		slug: string;
		website_url: string;
		data_source: string;
		is_active: boolean;
		display_order: number;
		created_at: string;
	};

	const canManage = $derived(hasPermission('remittance.admin'));

	let providers = $state<RemittanceProvider[]>([]);
	let loading = $state(true);
	let error = $state('');
	let createSlideOverOpen = $state(false);
	let editSlideOverOpen = $state(false);
	let editingProvider = $state<RemittanceProvider | null>(null);
	let saving = $state(false);
	let fetchingId = $state('');
	let message = $state('');
	let messageType = $state('');

	let newProvider = $state({
		name: '',
		slug: '',
		website_url: '',
		data_source: 'manual',
		is_active: true,
		display_order: 0
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api<RemittanceProvider[]>('/admin/remittance/providers');
			if (result.ok) {
				providers = result.data ?? [];
			} else {
				error = result.error;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('remittance.load_error', 'Failed to load remittance providers');
		} finally {
			loading = false;
		}
	}

	async function createProvider() {
		saving = true;
		message = '';
		try {
			const result = await api('/admin/remittance/providers', { method: 'POST', body: newProvider });
			if (result.ok) {
				messageType = 'success';
				message = t('remittance.create_success', 'Remittance provider created.');
				createSlideOverOpen = false;
				newProvider = { name: '', slug: '', website_url: '', data_source: 'manual', is_active: true, display_order: 0 };
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('remittance.create_error', 'Failed to create provider');
		} finally {
			saving = false;
		}
	}

	async function updateProvider() {
		if (!editingProvider) return;
		saving = true;
		message = '';
		try {
			const result = await api(`/admin/remittance/providers/${editingProvider.id}`, {
				method: 'PATCH',
				body: {
					name: editingProvider.name,
					website_url: editingProvider.website_url,
					is_active: editingProvider.is_active,
					display_order: editingProvider.display_order
				}
			});
			if (result.ok) {
				messageType = 'success';
				message = t('remittance.update_success', 'Remittance provider updated.');
				editSlideOverOpen = false;
				editingProvider = null;
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('remittance.update_error', 'Failed to update provider');
		} finally {
			saving = false;
		}
	}

	async function triggerFetch(id: string) {
		fetchingId = id;
		message = '';
		try {
			const result = await api(`/admin/remittance/providers/${id}/fetch`, { method: 'POST' });
			if (result.ok) {
				messageType = 'success';
				message = t('remittance.fetch_triggered', 'Rate fetch triggered.');
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('remittance.fetch_error', 'Failed to trigger fetch');
		} finally {
			fetchingId = '';
		}
	}

	function openEdit(provider: RemittanceProvider) {
		editingProvider = { ...provider };
		editSlideOverOpen = true;
	}

	const columns = $derived([
		{
			key: 'name',
			label: t('remittance.name', 'Name'),
			sortable: true,
			render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
		},
		{
			key: 'slug',
			label: t('remittance.slug', 'Slug'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value}</span>`
		},
		{
			key: 'data_source',
			label: t('remittance.data_source', 'Data Source'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground">${value}</span>`
		},
		{
			key: 'website_url',
			label: t('remittance.website', 'Website'),
			render: (value: string) => value ? `<a href="${value}" target="_blank" rel="noopener" class="text-primary text-xs hover:underline">${value}</a>` : '<span class="text-muted-foreground text-xs">—</span>'
		},
		{
			key: 'is_active',
			label: t('remittance.active', 'Active'),
			sortable: true,
			render: (value: boolean) => value
				? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">Active</span>`
				: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">Inactive</span>`
		},
		{
			key: 'display_order',
			label: t('remittance.order', 'Order'),
			sortable: true,
			render: (value: number) => `<span class="font-mono text-foreground">${value}</span>`
		}
	]);

	const rowActions: ActionItem[] = [
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: RemittanceProvider) => openEdit(row),
			show: () => canManage
		},
		{
			label: t('remittance.fetch', 'Fetch Rates'),
			icon: Download,
			onclick: (row: RemittanceProvider) => triggerFetch(row.id)
		}
	];

	load();
</script>

<PageHeader
	title={t('remittance.title', 'Remittance Rate Tracker')}
	description={t('remittance.description', 'Manage remittance providers and trigger rate fetches.')}
	actions={canManage ? [
		{ label: t('remittance.create', 'Add Provider'), icon: Plus, onclick: () => (createSlideOverOpen = true) }
	] : []}
/>

{#if message}
	<div
		class="mb-4 rounded-lg border p-3 text-sm"
		style={messageType === 'success'
			? 'border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)'
			: 'border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)'}
	>
		{message}
	</div>
{/if}

{#if loading}
	<div class="flex items-center gap-2 p-8" style="color: var(--color-muted)">
		<RefreshCw class="h-4 w-4 animate-spin" />
		<span>{t('remittance.loading', 'Loading providers...')}</span>
	</div>
{:else if error}
	<div class="rounded-lg border p-4" style="border-color: var(--color-danger); color: var(--color-danger)">
		{error}
	</div>
{:else}
	<DataTable
		data={providers}
		{columns}
		actions={rowActions}
		emptyMessage={t('remittance.empty', 'No remittance providers configured. Add one to enable rate tracking.')}
	/>
{/if}

<!-- Create SlideOver -->
<SlideOver bind:open={createSlideOverOpen} title={t('remittance.create_title', 'Add Remittance Provider')}>
	<div class="space-y-3">
		<div>
			<label for="remittance-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.name', 'Name')}</label>
			<input id="remittance-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newProvider.name} placeholder={t('app.remittance.ph_wise', 'Wise')} />
		</div>
		<div>
			<label for="remittance-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.slug', 'Slug')}</label>
			<input id="remittance-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newProvider.slug} placeholder="wise" />
		</div>
		<div>
			<label for="remittance-website" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.website', 'Website URL')}</label>
			<input id="remittance-website" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newProvider.website_url} placeholder="https://wise.com" />
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="remittance-data-source" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.data_source', 'Data Source')}</label>
				<select id="remittance-data-source" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newProvider.data_source}>
					<option value="manual">Manual</option>
					<option value="api">API</option>
					<option value="scraper">Scraper</option>
				</select>
			</div>
			<div>
				<label for="remittance-order" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.order', 'Display Order')}</label>
				<input id="remittance-order" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newProvider.display_order} />
			</div>
		</div>
		<div class="flex items-center gap-2">
			<input type="checkbox" id="remittance-active" bind:checked={newProvider.is_active} />
			<label for="remittance-active" class="text-sm" style="color: var(--color-foreground)">{t('remittance.active', 'Active')}</label>
		</div>
	</div>
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => (createSlideOverOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={createProvider} disabled={saving || !newProvider.name || !newProvider.slug}>
				{saving ? t('common.saving', 'Saving...') : t('common.create', 'Create')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Edit SlideOver -->
<SlideOver bind:open={editSlideOverOpen} title={t('remittance.edit_title', 'Edit Remittance Provider')}>
	{#if editingProvider}
		<div class="space-y-3">
			<div>
				<label for="remittance-edit-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.name', 'Name')}</label>
				<input id="remittance-edit-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editingProvider.name} />
			</div>
			<div>
				<label for="remittance-edit-website" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.website', 'Website URL')}</label>
				<input id="remittance-edit-website" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editingProvider.website_url} />
			</div>
			<div>
				<label for="remittance-edit-order" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.order', 'Display Order')}</label>
				<input id="remittance-edit-order" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editingProvider.display_order} />
			</div>
			<div class="flex items-center gap-2">
				<input type="checkbox" id="remittance-edit-active" bind:checked={editingProvider.is_active} />
				<label for="remittance-edit-active" class="text-sm" style="color: var(--color-foreground)">{t('remittance.active', 'Active')}</label>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => (editSlideOverOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={updateProvider} disabled={saving}>
				{saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
			</Button>
		</div>
	{/snippet}
</SlideOver>
