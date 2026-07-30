<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import {
		comparisonAdminApi,
		type ComparisonType,
		type ComparisonTypeDetail,
		type ComparisonTypeAccessRow,
		type CustomFieldGroupOption
	} from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { GitCompare, ChevronDown, ChevronRight, Shield } from 'lucide-svelte';

	let types = $state<ComparisonType[]>([]);
	let fieldGroups = $state<CustomFieldGroupOption[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');

	const filterDefs = [
		{
			key: 'is_system',
			label: t('comparison_types.visibility'),
			options: [
				{ value: 'true', label: t('comparison_types.system') },
				{ value: 'false', label: t('comparison_types.custom') }
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

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	let form = $state({
		name: '',
		slug: '',
		description: '',
		icon: '',
		item_label: '',
		field_group_id: '',
		is_system: false
	});

	let expandedId = $state<string | null>(null);
	let expandedSlug = $state<string | null>(null);
	let detail = $state<ComparisonTypeDetail | null>(null);
	let detailLoading = $state(false);
	let accessRows = $state<ComparisonTypeAccessRow[]>([]);
	let accessForm = $state({ user_group_id: '', can_create: false, can_fill: true, ai_can_fill: false });
	let accessSaving = $state(false);

	const columns = [
		{ key: 'name', label: t('comparison_types.name'), sortable: true },
		{ key: 'slug', label: t('comparison_types.slug'), render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
		{ key: 'description', label: t('comparison_types.description_label'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>` },
		{
			key: 'is_system',
			label: t('comparison_types.visibility'),
			render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('comparison_types.system') : t('comparison_types.custom')}</span>`
		},
		{ key: 'items', label: t('comparison_types.items'), render: () => `<span class="text-xs" style="color: var(--color-muted)">—</span>` },
		{
			key: 'actions',
			label: '',
			componentSnippet: ComparisonTypeActionsCell
		}
	];

	const filteredTypes = $derived.by(() => {
		let items = types;
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			items = items.filter((ct) => ct.name.toLowerCase().includes(q) || ct.slug.toLowerCase().includes(q) || ct.description?.toLowerCase().includes(q));
		}
		if (filterValues.is_system) {
			const val = filterValues.is_system === 'true';
			items = items.filter((ct) => ct.is_system === val);
		}
		return items;
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
		const [typesResult, groupsResult] = await Promise.all([
			comparisonAdminApi.listTypes(),
			comparisonAdminApi.listFieldGroups()
		]);
		if (typesResult.ok) {
			types = typesResult.data ?? [];
		} else {
			errorMessage = typesResult.error;
		}
		if (groupsResult.ok) {
			fieldGroups = groupsResult.data ?? [];
		}
		loading = false;
	}

	async function toggleExpand(row: ComparisonType) {
		if (expandedId === row.id) {
			expandedId = null;
			expandedSlug = null;
			detail = null;
			accessRows = [];
			return;
		}
		expandedId = row.id;
		expandedSlug = row.slug;
		detailLoading = true;
		detail = null;
		accessRows = [];

		const [detailResult, accessResult] = await Promise.all([
			comparisonAdminApi.getType(row.slug),
			comparisonAdminApi.getTypeAccess(row.id)
		]);

		if (detailResult.ok) {
			detail = detailResult.data;
		}
		if (accessResult.ok) {
			accessRows = accessResult.data ?? [];
		}
		detailLoading = false;
	}

	function openCreate() {
		editingId = null;
		form = { name: '', slug: '', description: '', icon: '', item_label: '', field_group_id: '', is_system: false };
		showModal = true;
	}

	function openEdit(item: ComparisonType) {
		editingId = item.id;
		form = {
			name: item.name,
			slug: item.slug,
			description: item.description ?? '',
			icon: item.icon ?? '',
			item_label: item.item_label ?? '',
			field_group_id: item.field_group_id ?? '',
			is_system: item.is_system
		};
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		if (!editingId && !form.field_group_id) {
			errorMessage = t('comparison_types.field_group_required');
			return;
		}
		saving = true;
		errorMessage = '';

		const slug = form.slug.trim() || slugify(form.name);
		const itemLabel = form.item_label.trim() || 'Item';

		if (editingId) {
			const result = await comparisonAdminApi.updateType(editingId, {
				name: form.name.trim(),
				slug,
				description: form.description.trim(),
				icon: form.icon.trim(),
				item_label: itemLabel
			});
			if (result.ok) {
				showModal = false;
				load();
				if (expandedId === editingId && expandedSlug) {
					const row = types.find((r) => r.id === editingId);
					if (row) toggleExpand(row);
				}
			} else {
				errorMessage = result.error;
			}
		} else {
			const result = await comparisonAdminApi.createType({
				name: form.name.trim(),
				slug,
				description: form.description.trim(),
				icon: form.icon.trim(),
				field_group_id: form.field_group_id,
				item_label: itemLabel
			});
			if (result.ok) {
				showModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
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
		const result = await comparisonAdminApi.deleteType(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			if (expandedId === deleteTargetId) {
				expandedId = null;
				expandedSlug = null;
				detail = null;
				accessRows = [];
			}
			deleteTargetId = null;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	async function saveAccess() {
		if (!expandedId) return;
		accessSaving = true;
		errorMessage = '';
		const result = await comparisonAdminApi.setTypeAccess(expandedId, {
			user_group_id: accessForm.user_group_id.trim(),
			can_create: accessForm.can_create,
			can_fill: accessForm.can_fill,
			ai_can_fill: accessForm.ai_can_fill
		});
		if (result.ok) {
			const fresh = await comparisonAdminApi.getTypeAccess(expandedId);
			if (fresh.ok) accessRows = fresh.data ?? [];
			accessForm = { user_group_id: '', can_create: false, can_fill: true, ai_can_fill: false };
		} else {
			errorMessage = result.error;
		}
		accessSaving = false;
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('comparison_types.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('comparison_types.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('comparison_types.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('comparison_types.add')}
	</button>
</div>

<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		columns={columns}
		data={filteredTypes}
		pageSize={25}
		emptyMessage={t('comparison_types.empty')}
		searchable
		onsearch={(q) => { searchQuery = q; }}
	/>

	{#if expandedId && (detailLoading || detail)}
		<div class="mt-4 rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			{#if detailLoading}
				<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
			{:else if detail}
				<div class="space-y-4">
					<div>
						<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
							<GitCompare class="h-4 w-4" style="color: var(--color-primary)" />
							{t('comparison_types.dimensions')}
						</h3>
						{#if detail.dimensions.length > 0}
							<div class="space-y-1">
								{#each detail.dimensions as dim}
									<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-border)">
										<span class="font-medium">{dim.name}</span>
										{#if dim.description}<span style="color: var(--color-muted)"> — {dim.description}</span>{/if}
									</div>
								{/each}
							</div>
						{:else}
							<p class="text-xs" style="color: var(--color-muted)">{t('comparison_types.no_dimensions_defined')}</p>
						{/if}
					</div>

					<p class="text-xs" style="color: var(--color-muted)">
						{t('comparison_types.items')}: <span class="font-medium">{detail.item_count}</span>
					</p>

					<div>
						<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
							<Shield class="h-4 w-4" style="color: var(--color-primary)" />
							{t('comparison_types.access')}
						</h3>
						<div class="space-y-3">
							{#if accessRows.length > 0}
								<div class="space-y-1">
									{#each accessRows as row}
										<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-border)">
											<span class="font-medium">{row.group_name ?? t('comparison_types.ai_only')}</span>
											<span style="color: var(--color-muted)">
												— {row.can_create ? t('comparison_types.can_create') : ''}{row.can_fill ? ` · ${t('comparison_types.can_fill')}` : ''}{row.ai_can_fill ? ` · ${t('comparison_types.ai_can_fill')}` : ''}
											</span>
										</div>
									{/each}
								</div>
							{:else}
								<p class="text-xs" style="color: var(--color-muted)">{t('comparison_types.access_empty')}</p>
							{/if}

							<div class="space-y-2 rounded-md border p-3" style="border-color: var(--color-border)">
								<div>
									<label for="ct-user-group" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.user_group')}</label>
									<input
										id="ct-user-group"
										type="text"
										class="w-full rounded-md border px-3 py-2 text-sm"
										style="border-color: var(--color-border); background-color: var(--color-background)"
										placeholder={t('comparison_types.user_group_placeholder')}
										bind:value={accessForm.user_group_id}
									/>
								</div>
								<div class="flex flex-wrap items-center gap-4">
									<label class="flex items-center gap-2 text-xs">
										<input type="checkbox" bind:checked={accessForm.can_create} />
										{t('comparison_types.can_create')}
									</label>
									<label class="flex items-center gap-2 text-xs">
										<input type="checkbox" bind:checked={accessForm.can_fill} />
										{t('comparison_types.can_fill')}
									</label>
									<label class="flex items-center gap-2 text-xs">
										<input type="checkbox" bind:checked={accessForm.ai_can_fill} />
										{t('comparison_types.ai_can_fill')}
									</label>
								</div>
								<button
									class="rounded-md px-3 py-1.5 text-xs font-medium text-white transition-colors"
									style="background-color: var(--color-primary)"
									onclick={saveAccess}
									disabled={accessSaving}
								>
									{accessSaving ? t('common.saving') : t('comparison_types.add_access')}
								</button>
							</div>
						</div>
					</div>
				</div>
			{:else}
				<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('common.error')}</p>
			{/if}
		</div>
	{/if}
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={editingId ? t('comparison_types.edit') : t('comparison_types.add')} onConfirm={submit} confirmLabel={saving ? t('common.saving') : (editingId ? t('common.save') : t('common.create'))}>
	<div class="space-y-3">
		<div>
			<label for="ct-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.name')}</label>
			<input id="ct-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.name} oninput={() => { if (!editingId) form.slug = slugify(form.name); }} />
		</div>
		<div>
			<label for="ct-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.slug')}</label>
			<input id="ct-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.slug} />
		</div>
		<div>
			<label for="ct-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.description_label')}</label>
			<textarea id="ct-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="2" bind:value={form.description}></textarea>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="ct-icon" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.icon')}</label>
				<input id="ct-icon" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.icon} />
			</div>
			<div>
				<label for="ct-item-label" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.item_label')}</label>
				<input id="ct-item-label" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.item_label} />
			</div>
		</div>
		<div>
			<label for="ct-field-group" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.field_group')}</label>
			{#if editingId}
				<input id="ct-field-group" type="text" disabled value={form.field_group_id} class="w-full rounded-md border px-3 py-2 text-sm font-mono opacity-60" style="border-color: var(--color-border); background-color: var(--color-surface)" />
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('comparison_types.field_group_locked')}</p>
			{:else}
				<select id="ct-field-group" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.field_group_id}>
					<option value="">{t('comparison_types.field_group_select')}</option>
					{#each fieldGroups as fg (fg.id)}
						<option value={fg.id}>{fg.name}</option>
					{/each}
				</select>
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<input id="ct-system" type="checkbox" bind:checked={form.is_system} disabled={!editingId} />
			<label for="ct-system" class="text-sm">{t('comparison_types.system')}</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.delete')} onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('comparison_types.delete_confirm')}</p>
</Modal>

{#snippet ComparisonTypeActionsCell({ value, row }: { value: any; row: ComparisonType })}
	<div class="flex items-center gap-2">
		<button
			class="rounded p-1 hover:bg-black/5 dark:hover:bg-surface/10"
			onclick={() => toggleExpand(row)}
			aria-label={t('comparison_types.expand')}
		>
			{#if expandedId === row.id}
				<ChevronDown class="h-4 w-4" style="color: var(--color-muted)" />
			{:else}
				<ChevronRight class="h-4 w-4" style="color: var(--color-muted)" />
			{/if}
		</button>
		<button
			class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
			style="color: var(--color-info)"
			onclick={() => openEdit(row)}
		>
			{t('common.edit')}
		</button>
		<button
			class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
			style="color: var(--color-danger)"
			onclick={() => confirmDelete(row.id)}
		>
			{t('common.delete')}
		</button>
	</div>
{/snippet}
