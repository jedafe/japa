<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { sharePlatformsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Button } from '$lib/components/ui/button';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Eye, Pencil, Trash2, Share2, Plus } from 'lucide-svelte';

	type SharePlatform = {
		key: string;
		label: string;
		icon: string;
		url_template: string;
		color: string;
		enabled: boolean;
		position: number;
		created_at: string;
	};

	let platforms = $state<SharePlatform[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let filterValues = $state<Record<string, string>>({});

	let showModal = $state(false);
	let editingPlatform = $state<SharePlatform | null>(null);
	let form = $state({ key: '', label: '', icon: '', url_template: '', color: '#3b82f6', enabled: true, position: 0 });
	let saving = $state(false);

	let showDeleteModal = $state(false);
	let deleteTarget = $state<SharePlatform | null>(null);

	let showViewModal = $state(false);
	let viewTarget = $state<SharePlatform | null>(null);

	const canManage = $derived(hasPermission('content.reaction_types.manage'));

	const filterDefs = [
		{
			key: 'enabled',
			label: t('share_platforms.status', 'Status'),
			options: [
				{ value: 'true', label: t('share_platforms.active', 'Active') },
				{ value: 'false', label: t('share_platforms.inactive', 'Inactive') }
			]
		}
	];

	const columns = [
		{ key: 'key', label: t('share_platforms.key', 'Key'), sortable: true, render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
		{ key: 'label', label: t('share_platforms.label', 'Label'), sortable: true },
		{ key: 'icon', label: t('share_platforms.icon', 'Icon'), componentSnippet: IconCell },
		{ key: 'url_template', label: t('share_platforms.url_template', 'URL Template'), render: (v: string) => `<span class="font-mono text-xs truncate" style="color: var(--color-muted)">${v}</span>` },
		{ key: 'color', label: t('share_platforms.color', 'Color'), componentSnippet: ColorCell },
		{ key: 'enabled', label: t('share_platforms.active', 'Enabled'), sortable: true, componentSnippet: StatusCell },
		{ key: 'position', label: t('share_platforms.position', 'Position'), sortable: true },
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-32', componentSnippet: PlatformActionsCell }
	];

	const filteredPlatforms = $derived(
		searchQuery
			? platforms.filter(
					(p) =>
						p.key.toLowerCase().includes(searchQuery.toLowerCase()) ||
						p.label.toLowerCase().includes(searchQuery.toLowerCase()) ||
						p.icon.toLowerCase().includes(searchQuery.toLowerCase()) ||
						p.url_template.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: platforms
	);

	function openView(platform: SharePlatform) {
		viewTarget = platform;
		showViewModal = true;
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (filterValues.enabled) params.set('enabled', filterValues.enabled);
			if (searchQuery) params.set('q', searchQuery);
			const result = await sharePlatformsAdminApi.list(params.toString() || undefined);
			if (result.ok) {
				platforms = (result.data as SharePlatform[]) ?? [];
			} else {
				errorMessage = result.error as string;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterValues = values;
		load();
	}

	function handleClear() {
		searchQuery = '';
		filterValues = {};
		load();
	}

	function openCreate() {
		editingPlatform = null;
		form = { key: '', label: '', icon: '', url_template: '', color: '#3b82f6', enabled: true, position: platforms.length };
		showModal = true;
	}

	function openEdit(platform: SharePlatform) {
		editingPlatform = platform;
		form = {
			key: platform.key,
			label: platform.label,
			icon: platform.icon,
			url_template: platform.url_template,
			color: platform.color,
			enabled: platform.enabled,
			position: platform.position
		};
		showModal = true;
	}

	async function submit() {
		if (!form.key.trim() || !form.label.trim() || !form.icon.trim() || !form.url_template.trim()) return;
		saving = true;
		errorMessage = '';
		const body = {
			key: form.key.trim(),
			label: form.label.trim(),
			icon: form.icon.trim(),
			url_template: form.url_template.trim(),
			color: form.color,
			enabled: form.enabled,
			position: Number(form.position)
		};
		const result = editingPlatform
			? await sharePlatformsAdminApi.update(editingPlatform.key, {
					label: body.label,
					icon: body.icon,
					url_template: body.url_template,
					color: body.color,
					enabled: body.enabled,
					position: body.position
				})
			: await sharePlatformsAdminApi.create(body);
		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(platform: SharePlatform) {
		deleteTarget = platform;
		showDeleteModal = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		const result = await sharePlatformsAdminApi.delete(deleteTarget.key);
		if (result.ok) {
			showDeleteModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('share_platforms.title', 'Share Platforms')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 flex items-center gap-2 text-2xl font-bold tracking-tight">
	<Share2 class="h-6 w-6" style="color: var(--color-primary)" />
	{t('share_platforms.title', 'Share Platforms')}
</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('share_platforms.description', 'Manage share platforms available across the platform for content sharing.')}</p>

<div class="mb-4 flex justify-end">
	{#if canManage}
		<button
			class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={openCreate}
		>
			<Plus class="h-4 w-4" />
			{t('share_platforms.create', 'Create')}
		</button>
	{/if}
</div>

<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

{#if loading && platforms.length === 0}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		{columns}
		data={filteredPlatforms}
		searchable={true}
		searchPlaceholder={t('common.search', 'Search')}
		onsearch={(q: string) => { searchQuery = q; }}
		pageSize={25}
		emptyMessage={t('share_platforms.no_platforms', 'No share platforms found')}
	/>
{/if}

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('share_platforms.view', 'View')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.icon', 'Icon')}</p>
				<p class="mt-1 text-2xl" style="color: var(--color-foreground)">{viewTarget.icon}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.label', 'Label')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.label}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.key', 'Key')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.key}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.url_template', 'URL Template')}</p>
				<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.url_template}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.color', 'Color')}</p>
				<div class="mt-1 flex items-center gap-2">
					<span class="inline-block h-5 w-5 rounded-full border" style="background-color: {viewTarget.color}; border-color: var(--color-border)"></span>
					<span class="text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.color}</span>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.position', 'Position')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.position}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.active', 'Enabled')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.enabled ? t('share_platforms.active', 'Active') : t('share_platforms.inactive', 'Inactive')}</p>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showViewModal = false)}>{t('common.close')}</button>
	</div>
</Modal>

<!-- Create/Edit Modal -->
<Modal
	bind:open={showModal}
	title={editingPlatform ? t('share_platforms.edit', 'Edit') : t('share_platforms.create', 'Create')}
	onConfirm={submit}
	confirmLabel={saving ? t('common.saving') : editingPlatform ? t('common.save') : t('share_platforms.create', 'Create')}
>
	<div class="space-y-3">
		<div>
			<label for="sp-key" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('share_platforms.key', 'Key')}</label>
			<input
				id="sp-key"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="twitter"
				bind:value={form.key}
				disabled={!!editingPlatform}
			/>
		</div>
		<div>
			<label for="sp-label" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('share_platforms.label', 'Label')}</label>
			<input
				id="sp-label"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="Twitter"
				bind:value={form.label}
			/>
		</div>
		<div>
			<label for="sp-icon" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('share_platforms.icon', 'Icon')}</label>
			<input
				id="sp-icon"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="🐦"
				bind:value={form.icon}
			/>
		</div>
		<div>
			<label for="sp-url" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('share_platforms.url_template', 'URL Template')}</label>
			<input
				id="sp-url"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="https://twitter.com/intent/tweet?url={'{url}'}&text={'{title}'}"
				bind:value={form.url_template}
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('share_platforms.url_template_hint', 'Use {url} and {title} as placeholders.')}</p>
		</div>
		<div>
			<label for="sp-color" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('share_platforms.color', 'Color')}</label>
			<div class="flex items-center gap-2">
				<input
					id="sp-color"
					type="color"
					class="h-9 w-12 cursor-pointer rounded-md border p-1"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					bind:value={form.color}
				/>
				<input
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm font-mono"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					bind:value={form.color}
				/>
			</div>
		</div>
		<div>
			<label for="sp-position" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('share_platforms.position', 'Position')}</label>
			<input
				id="sp-position"
				type="number"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				bind:value={form.position}
			/>
		</div>
		<div class="flex items-center gap-2">
			<input id="sp-active" type="checkbox" bind:checked={form.enabled} />
			<label for="sp-active" class="text-sm">{t('share_platforms.active', 'Enabled')}</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteModal} title={t('common.confirm')} onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('share_platforms.delete_confirm', 'Are you sure you want to delete the share platform')} <strong>{deleteTarget?.label}</strong>?
	</p>
</Modal>

{#snippet IconCell({ value }: { value: any; row: SharePlatform })}
	<span class="text-xl">{value}</span>
{/snippet}

{#snippet ColorCell({ value }: { value: string; row: SharePlatform })}
	<div class="flex items-center gap-2">
		<span class="inline-block h-4 w-4 rounded-full border" style="background-color: {value}; border-color: var(--color-border)"></span>
		<span class="font-mono text-xs" style="color: var(--color-muted)">{value}</span>
	</div>
{/snippet}

{#snippet StatusCell({ value }: { value: boolean; row: SharePlatform })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('share_platforms.active', 'Active') : t('share_platforms.inactive', 'Inactive')}
	</span>
{/snippet}

{#snippet PlatformActionsCell({ row }: { value: any; row: SharePlatform })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)} aria-label={t('common.view', 'View')}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)} aria-label={t('common.edit', 'Edit')}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)" aria-label={t('common.delete', 'Delete')}>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
