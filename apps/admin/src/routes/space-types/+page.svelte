<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { spaceTypesApi } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2 } from 'lucide-svelte';

	type SpaceType = {
		id: string;
		name: string;
		slug: string;
		description: string;
		is_university: boolean;
		icon: string;
	};

	let spaceTypes = $state<SpaceType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);
	let showViewModal = $state(false);
	let viewTarget = $state<SpaceType | null>(null);

	const canManageSpaceTypes = $derived(hasPermission('space_types.create'));

	const filterDefs = [
		{
			key: 'is_university',
			label: t('app.space_types.filter_university', 'University'),
			options: [
				{ value: 'true', label: t('app.space_types.filter_yes', 'Yes') },
				{ value: 'false', label: t('app.space_types.filter_no', 'No') }
			]
		}
	];

	let filterValues = $state<Record<string, string>>({});

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterValues = values;
	}

	function handleClear() {
		searchQuery = '';
		filterValues = {};
	}

	let form = $state({
		name: '',
		slug: '',
		description: '',
		is_university: false,
		icon: ''
	});

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (searchQuery) params.set('q', searchQuery);
		if (filterValues.is_university) params.set('is_university', filterValues.is_university);
		const qs = params.toString();
		const result = await spaceTypesApi.list(qs || undefined);
		if (result.ok) {
			const d = result.data as SpaceType[] | { items: SpaceType[]; total_pages: number };
			spaceTypes = Array.isArray(d) ? d : (d.items ?? []);
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function openView(item: SpaceType) {
		viewTarget = item;
		showViewModal = true;
	}

	function openCreate() {
		editingId = null;
		form = { name: '', slug: '', description: '', is_university: false, icon: '' };
		showModal = true;
	}

	function openEdit(item: SpaceType) {
		editingId = item.id;
		form = {
			name: item.name,
			slug: item.slug,
			description: item.description ?? '',
			is_university: item.is_university,
			icon: item.icon ?? ''
		};
		showViewModal = false;
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = '';

		const slug = form.slug.trim() || slugify(form.name);
		const body = {
			name: form.name.trim(),
			slug,
			description: form.description.trim(),
			is_university: form.is_university,
			icon: form.icon.trim()
		};

		const result = editingId
			? await spaceTypesApi.update(editingId, body)
			: await spaceTypesApi.create(body);

		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(id: string) {
		deleteTargetId = id;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTargetId) return;
		errorMessage = '';
		const result = await spaceTypesApi.delete(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTargetId = null;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		errorMessage = '';
		try {
			for (const id of selectedIds) {
				await spaceTypesApi.delete(id);
			}
			selectedIds = new Set();
			load();
		} catch {
			errorMessage = t('app.space_types.failed_to_delete_selected', 'Failed to delete selected items');
		}
		bulkDeleting = false;
	}

	const columns = [
		{ key: 'name', label: t('space-types.name'), sortable: true },
		{ key: 'slug', label: t('space-types.slug'), class: 'font-mono text-xs' },
		{
			key: 'description',
			label: t('space-types.description'),
			render: (v: string) => v || '—'
		},
		{
			key: 'is_university',
			label: t('space-types.university'),
			render: (v: boolean) =>
				`<span class="inline-block rounded-full px-2 py-0.5 text-xs ${v ? 'bg-info/15 text-info dark:text-info' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('app.space_types.yes', 'Yes') : t('app.space_types.no', 'No')}</span>`
		},
		{
			key: 'icon',
			label: t('space-types.icon'),
			render: (v: string) => `<span class="text-lg">${v || '—'}</span>`
		},
		{ key: 'actions', label: '', class: 'w-32', componentSnippet: ActionsCell }
	];

	function handleRowClick(row: SpaceType) {
		openView(row);
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('app.space_types.page_title', 'Space Types')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('space-types.space_types')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('space-types.manage_space_type_categories_including_u')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	{#if canManageSpaceTypes}
		<Button onclick={openCreate}>{t('app.space_types.add_space_type', 'Add Space Type')}</Button>
	{/if}
</div>

<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('space-types.loading')}</p>
{:else}
	<DataTable
		{columns}
		data={spaceTypes}
		onrowclick={handleRowClick}
		emptyMessage={t('space-types.no_space_types_configured')}
		selectable
		bind:selectedIds
	>
		{#snippet bulkActions()}
			<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
				<Trash2 class="h-3.5 w-3.5 mr-1" />
				{bulkDeleting ? t('app.space_types.deleting', 'Deleting...') : t('app.space_types.delete_selected', 'Delete Selected')} ({selectedIds.size})
			</Button>
		{/snippet}
	</DataTable>
{/if}

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('app.space_types.space_type_details', 'Space Type Details')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.name_label', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.slug_label', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.description_label', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.description || '—'}</p>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.university_label', 'University')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.is_university ? t('app.space_types.yes', 'Yes') : t('app.space_types.no', 'No')}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.icon_label', 'Icon')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.icon || '—'}</p>
				</div>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		{#if canManageSpaceTypes && viewTarget}
			<Button variant="ghost" onclick={() => { openEdit(viewTarget!); }}>
				<Pencil class="h-4 w-4" />
				{t('app.space_types.edit', 'Edit')}
			</Button>
			<Button variant="destructive" onclick={() => { showViewModal = false; confirmDelete(viewTarget!.id); }}>
				<Trash2 class="h-4 w-4" />
				{t('app.space_types.delete', 'Delete')}
			</Button>
		{/if}
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('app.space_types.close', 'Close')}</Button>
	</div>
</Modal>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={editingId ? t('app.space_types.edit_space_type', 'Edit Space Type') : t('app.space_types.add_space_type', 'Add Space Type')}>
	<form class="space-y-3" onsubmit={(e) => { e.preventDefault(); submit(); }}>
		<div>
			<label for="st-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space-types.name')}</label>
			<input id="st-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="University" bind:value={form.name} oninput={() => { if (!editingId) form.slug = slugify(form.name); }} />
		</div>
		<div>
			<label for="st-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space-types.slug')}</label>
			<input id="st-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="university" bind:value={form.slug} />
		</div>
		<div>
			<label for="st-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space-types.description')}</label>
			<textarea id="st-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="2" bind:value={form.description}></textarea>
		</div>
		<div>
			<label for="st-icon" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space-types.icon_emoji_or_name')}</label>
			<input id="st-icon" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="🎓" bind:value={form.icon} />
		</div>
		<div class="flex items-center gap-2">
			<input id="st-uni" type="checkbox" bind:checked={form.is_university} />
			<label for="st-uni" class="text-sm">{t('space-types.university_type')}</label>
		</div>
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" type="button" onclick={() => showModal = false}>{t('common.cancel', 'Cancel')}</Button>
			<Button type="submit" disabled={saving}>{saving ? t('common.saving', 'Saving...') : editingId ? t('common.save', 'Save') : t('common.create', 'Create')}</Button>
		</div>
	</form>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete', 'Confirm Delete')}>
	<p class="text-sm" style="color: var(--color-foreground)">{t('space-types.are_you_sure_you_want_to_delete_this_spa')}</p>
	<p class="mt-1 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>

<!-- ActionsCell snippet for DataTable -->
{#snippet ActionsCell({ value, row }: { value: any; row: SpaceType })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); openView(row); }}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageSpaceTypes}
			<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); openEdit(row); }}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); confirmDelete(row.id); }} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
