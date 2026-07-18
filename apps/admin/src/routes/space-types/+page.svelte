<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { spaceTypesApi } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type SpaceType = {
		id: string;
		name: string;
		slug: string;
		description: string;
		is_university: boolean;
		icon: string;
	};

	let spaceTypes = $state<SpaceType[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let errorMessage = $state('');

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);
	let showViewModal = $state(false);
	let viewTarget = $state<SpaceType | null>(null);

	// Backend gates space-type mutations on space_types.create/edit/delete
	const canManageSpaceTypes = $derived(hasPermission('space_types.create'));

	async function openView(item: SpaceType) {
		viewTarget = item;
		showViewModal = true;
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
		const params = new URLSearchParams({ page: String(page) });
		const result = await spaceTypesApi.list(params.toString());
		if (result.ok) {
			const data = result.data as { items: SpaceType[]; total_pages: number };
			spaceTypes = data.items ?? [];
			totalPages = data.total_pages ?? 1;
		} else {
			errorMessage = result.error;
		}
		loading = false;
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

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>Space Types — JapaConnect Admin</title>
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
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={openCreate}
		>
			Add Space Type
		</button>
	{/if}
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('space-types.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space-types.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space-types.slug')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space-types.description')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space-types.university')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space-types.icon')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each spaceTypes as item}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-surface/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{item.name}</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{item.slug}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.description || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {item.is_university ? 'bg-info/15 text-info dark:text-info' : 'bg-muted/15 text-muted dark:text-muted'}">
								{item.is_university ? 'Yes' : 'No'}
							</span>
						</td>
						<td class="px-4 py-3 text-lg">{item.icon || '—'}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-2">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
									style="color: var(--color-muted)"
									onclick={() => openView(item)}
								>
									View
								</button>
								{#if canManageSpaceTypes}
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
										style="color: var(--color-info)"
										onclick={() => openEdit(item)}
									>
										Edit
									</button>
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
										style="color: var(--color-danger)"
										onclick={() => confirmDelete(item.id)}
									>
										Delete
									</button>
								{/if}
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">{t('space-types.no_space_types_configured')}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-surface/10 disabled:opacity-40" disabled={page === 0} onclick={() => handlePageChange(page - 1)}>&larr;</button>
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-surface/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => handlePageChange(page + 1)}>&rarr;</button>
		</div>
	</div>
{/if}

<!-- View Modal -->
<Modal bind:open={showViewModal} title="Space Type Details">
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Name</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Slug</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Description</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.description || '—'}</p>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">University</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.is_university ? 'Yes' : 'No'}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Icon</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.icon || '—'}</p>
				</div>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showViewModal = false)}>Close</button>
	</div>
</Modal>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={editingId ? 'Edit Space Type' : 'Add Space Type'} onConfirm={submit} confirmLabel={saving ? 'Saving...' : (editingId ? 'Update' : 'Create')}>
	<div class="space-y-3">
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
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Confirm Delete" onConfirm={executeDelete} confirmLabel="Delete">
	<p class="text-sm" style="color: var(--color-muted)">{t('space-types.are_you_sure_you_want_to_delete_this_spa')}</p>
</Modal>
