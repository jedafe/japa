<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from "$lib/components/ui/data-table";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Button } from '$lib/components/ui/button';
	import { Plane, Plus, RefreshCw, Pencil } from 'lucide-svelte';
	import { api } from '$lib/api';

	type TravelProvider = {
		id: string;
		name: string;
		slug: string;
		provider_type: string;
		api_config: string;
		affiliate_config: string;
		is_active: boolean;
		created_at: string;
	};

	const canManage = $derived(hasPermission('travel.admin'));

	let providers = $state<TravelProvider[]>([]);
	let loading = $state(true);
	let error = $state('');
	let createSlideOverOpen = $state(false);
	let editSlideOverOpen = $state(false);
	let editingProvider = $state<TravelProvider | null>(null);
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');

	let newProvider = $state({
		name: '',
		slug: '',
		provider_type: 'api',
		api_config: '{}',
		affiliate_config: '{}',
		is_active: true
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api<TravelProvider[]>('/admin/travel/providers');
			if (result.ok) {
				providers = result.data ?? [];
			} else {
				error = result.error;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('travel.load_error', 'Failed to load travel providers');
		} finally {
			loading = false;
		}
	}

	async function createProvider() {
		saving = true;
		message = '';
		try {
			const result = await api('/admin/travel/providers', { method: 'POST', body: newProvider });
			if (result.ok) {
				messageType = 'success';
				message = t('travel.create_success', 'Travel provider created.');
				createSlideOverOpen = false;
				newProvider = { name: '', slug: '', provider_type: 'api', api_config: '{}', affiliate_config: '{}', is_active: true };
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('travel.create_error', 'Failed to create provider');
		} finally {
			saving = false;
		}
	}

	async function updateProvider() {
		if (!editingProvider) return;
		saving = true;
		message = '';
		try {
			const result = await api(`/admin/travel/providers/${editingProvider.id}`, {
				method: 'PATCH',
				body: {
					name: editingProvider.name,
					provider_type: editingProvider.provider_type,
					is_active: editingProvider.is_active
				}
			});
			if (result.ok) {
				messageType = 'success';
				message = t('travel.update_success', 'Travel provider updated.');
				editSlideOverOpen = false;
				editingProvider = null;
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('travel.update_error', 'Failed to update provider');
		} finally {
			saving = false;
		}
	}

	function openEdit(provider: TravelProvider) {
		editingProvider = { ...provider };
		editSlideOverOpen = true;
	}

	const columns = $derived([
		{
			key: 'name',
			label: t('travel.name', 'Name'),
			sortable: true,
			render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
		},
		{
			key: 'slug',
			label: t('travel.slug', 'Slug'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value}</span>`
		},
		{
			key: 'provider_type',
			label: t('travel.type', 'Type'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground">${value}</span>`
		},
		{
			key: 'is_active',
			label: t('travel.active', 'Active'),
			sortable: true,
			render: (value: boolean) => value
				? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">Active</span>`
				: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">Inactive</span>`
		},
		{
			key: 'created_at',
			label: t('travel.created', 'Created'),
			sortable: true,
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString() : '—'}</span>`
		}
	]);

	const rowActions: ActionItem[] = [
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: TravelProvider) => openEdit(row),
			show: () => canManage
		}
	];

	load();
</script>

<PageHeader
	title={t('travel.title', 'Travel Search')}
	description={t('travel.description', 'Manage travel search providers and configuration.')}
	actions={canManage ? [
		{ label: t('travel.create', 'Add Provider'), icon: Plus, onclick: () => (createSlideOverOpen = true) }
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
		<span>{t('travel.loading', 'Loading providers...')}</span>
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
		emptyMessage={t('travel.empty', 'No travel providers configured. Add one to enable travel search.')}
	/>
{/if}

<!-- Create SlideOver -->
<SlideOver bind:open={createSlideOverOpen} title={t('travel.create_title', 'Add Travel Provider')}>
	<div class="space-y-3">
		<div>
			<label for="travel-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('travel.name', 'Name')}</label>
			<input id="travel-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newProvider.name} placeholder="Skyscanner" />
		</div>
		<div>
			<label for="travel-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('travel.slug', 'Slug')}</label>
			<input id="travel-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newProvider.slug} placeholder="skyscanner" />
		</div>
		<div>
			<label for="travel-type" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('travel.type', 'Provider Type')}</label>
			<select id="travel-type" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newProvider.provider_type}>
				<option value="api">API</option>
				<option value="affiliate">Affiliate</option>
				<option value="scraper">Scraper</option>
			</select>
		</div>
		<div>
			<label for="travel-api-config" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('travel.api_config', 'API Config (JSON)')}</label>
			<textarea id="travel-api-config" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="3" bind:value={newProvider.api_config}></textarea>
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
<SlideOver bind:open={editSlideOverOpen} title={t('travel.edit_title', 'Edit Travel Provider')}>
	{#if editingProvider}
		<div class="space-y-3">
			<div>
				<label for="travel-edit-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('travel.name', 'Name')}</label>
				<input id="travel-edit-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editingProvider.name} />
			</div>
			<div>
				<label for="travel-edit-type" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('travel.type', 'Provider Type')}</label>
				<select id="travel-edit-type" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editingProvider.provider_type}>
					<option value="api">API</option>
					<option value="affiliate">Affiliate</option>
					<option value="scraper">Scraper</option>
				</select>
			</div>
			<div class="flex items-center gap-2">
				<input type="checkbox" id="travel-active" bind:checked={editingProvider.is_active} />
				<label for="travel-active" class="text-sm" style="color: var(--color-foreground)">{t('travel.active', 'Active')}</label>
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
