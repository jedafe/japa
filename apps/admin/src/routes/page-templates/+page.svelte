<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { goto } from '$app/navigation';
	import { Plus, Pencil, Trash2, FilePlus } from 'lucide-svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import Modal from '$lib/components/Modal.svelte';
	import { FilterBar } from '$lib/components/ui/filter-bar';

	const canManage = $derived(hasPermission('pagebuilder.templates.manage'));

	type PageTemplate = {
		id: string;
		tenant_id: string;
		name: string;
		slug: string;
		description?: string;
		page_type: string;
		blocks: unknown[];
		is_system: boolean;
		created_at: string;
		updated_at: string;
	};

	let templates = $state<PageTemplate[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let searchTerm = $state('');

	const categoryOptions = [
		{ value: 'landing', label: t('pageTemplates.categoryLanding') },
		{ value: 'about', label: t('pageTemplates.categoryAbout') },
		{ value: 'service', label: t('pageTemplates.categoryService') },
		{ value: 'event', label: t('pageTemplates.categoryEvent') },
		{ value: 'custom', label: t('pageTemplates.categoryCustom') }
	];

	const filterDefs = [
		{ key: 'page_type', label: t('pageTemplates.category'), options: categoryOptions },
		{ key: 'is_system', label: t('pageTemplates.type'), options: [
			{ value: 'true', label: t('pageTemplates.system') },
			{ value: 'false', label: t('pageTemplates.custom') }
		]}
	];
	let filterValues = $state<Record<string, string>>({});

	function handleSearch(values: Record<string, string>) {
		filterValues = values;
		searchTerm = values.search ?? "";
		load();
	}

	function handleClear() {
		filterValues = {};
		searchTerm = "";
		load();
	}

	let modalOpen = $state(false);
	let editingTemplate = $state<PageTemplate | null>(null);
	let saving = $state(false);

	// Delete confirmation state
	let showDeleteConfirm = $state(false);
	let deletingTemplate = $state<PageTemplate | null>(null);
	let deleting = $state(false);

	let name = $state('');
	let description = $state('');
	let pageType = $state('custom');

	// Auto-generate slug from name for display purposes
	const generatedSlug = $derived(
		name
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '')
	);

	const filteredTemplates = $derived.by(() => {
		let result = templates;
		const q = searchTerm.toLowerCase().trim();
		if (q) {
			result = result.filter(
				(tpl) =>
					tpl.name.toLowerCase().includes(q) ||
					tpl.slug.toLowerCase().includes(q) ||
					(tpl.description ?? '').toLowerCase().includes(q)
			);
		}
		const systemFilter = filterValues.is_system;
		if (systemFilter !== undefined && systemFilter !== '') {
			const wantSystem = systemFilter === 'true';
			result = result.filter((tpl) => tpl.is_system === wantSystem);
		}
		const categoryFilter = filterValues.page_type;
		if (categoryFilter !== undefined && categoryFilter !== '') {
			result = result.filter((tpl) => tpl.page_type === categoryFilter);
		}
		return result;
	});

	const columns = [
		{ key: 'name', label: t('pageTemplates.name'), sortable: true,
			render: (v: string) => `<span class="font-medium">${v}</span>`
		},
		{ key: 'slug', label: t('pageTemplates.slug'), sortable: true,
			render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{ key: 'page_type', label: t('pageTemplates.category'), sortable: true,
			render: (v: string) => {
				const label = categoryOptions.find((c) => c.value === v)?.label ?? v;
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-primary/15 text-primary">${label}</span>`;
			}
		},
		{ key: 'blocks', label: t('pageTemplates.blocks'),
			render: (v: unknown[]) => String(Array.isArray(v) ? v.length : 0)
		},
		{ key: 'is_system', label: t('pageTemplates.status'), sortable: true,
			render: (v: boolean) => {
				const cls = v ? 'bg-warning/15 text-warning' : 'bg-success/15 text-success';
				const text = v ? t('pageTemplates.system') : t('pageTemplates.custom');
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${text}</span>`;
			}
		},
		{ key: 'actions', label: t('common.actions'), class: 'w-24', componentSnippet: TemplateActionsCell }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (searchTerm.trim()) params.set('q', searchTerm.trim());
			const categoryFilter = filterValues.page_type;
			if (categoryFilter) params.set('category', categoryFilter);
			const qs = params.toString();
			const result = await api<PageTemplate[]>(`/admin/page-templates${qs ? `?${qs}` : ''}`);
			if (result.ok) {
				templates = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingTemplate = null;
		name = '';
		description = '';
		pageType = 'custom';
		modalOpen = true;
	}

	function openEdit(tpl: PageTemplate) {
		editingTemplate = tpl;
		name = tpl.name;
		description = tpl.description || '';
		pageType = tpl.page_type || 'custom';
		modalOpen = true;
	}

	async function createTemplate() {
		if (!name.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/page-templates', {
				method: 'POST',
				body: { name: name.trim(), description: description.trim() || undefined, page_type: pageType }
			});
			if (result.ok) {
				modalOpen = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function updateTemplate() {
		if (!editingTemplate || !name.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/page-templates/${editingTemplate.id}`, {
				method: 'PATCH',
				body: { name: name.trim(), description: description.trim() || undefined, page_type: pageType }
			});
			if (result.ok) {
				modalOpen = false;
				editingTemplate = null;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDelete(tpl: PageTemplate) {
		deletingTemplate = tpl;
		showDeleteConfirm = true;
	}

	async function deleteTemplate() {
		if (!deletingTemplate) return;
		deleting = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/page-templates/${deletingTemplate.id}`, { method: 'DELETE' });
			if (result.ok) {
				templates = templates.filter((tpl) => tpl.id !== deletingTemplate!.id);
				showDeleteConfirm = false;
				deletingTemplate = null;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			deleting = false;
		}
	}

	async function createPageFromTemplate(tpl: PageTemplate) {
		errorMessage = '';
		try {
			const result = await api<{ id: string }>('/admin/pages', {
				method: 'POST',
				body: { title: `${tpl.name} Page`, slug: '', template_id: tpl.id }
			});
			if (!result.ok) {
				errorMessage = result.error;
			} else if (result.data) {
				goto(`/page-builder/pages/${result.data.id}`);
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('pageTemplates.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('pageTemplates.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('pageTemplates.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between gap-2">
	<FilterBar
		filters={filterDefs}
		bind:value={filterValues}
		searchPlaceholder={t('common.search')}
		onsearch={handleSearch}
		onclear={handleClear}
		class="flex-1"
	/>
	{#if canManage}<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('pageTemplates.create')}
	</button>{/if}
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		{columns}
		data={filteredTemplates}
		pageSize={25}
		emptyMessage={t('pageTemplates.empty')}
	/>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingTemplate ? t('pageTemplates.edit') : t('pageTemplates.create')} onConfirm={editingTemplate ? updateTemplate : createTemplate} confirmLabel={saving ? t('common.saving') : editingTemplate ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="pt-name" class="mb-1 block text-sm font-medium">{t('pageTemplates.name')}</label>
			<input id="pt-name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<span class="mb-1 block text-sm font-medium">{t('pageTemplates.slug')}</span>
			<p class="rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-muted)">{generatedSlug || '—'}</p>
		</div>
		<div>
			<label for="pt-category" class="mb-1 block text-sm font-medium">{t('pageTemplates.category')}</label>
			<select id="pt-category" bind:value={pageType} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				{#each categoryOptions as opt}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="pt-desc" class="mb-1 block text-sm font-medium">{t('pageTemplates.description')}</label>
			<textarea id="pt-desc" bind:value={description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteConfirm} title={t('pageTemplates.deleteConfirm')} onConfirm={deleteTemplate} confirmLabel={deleting ? t('common.deleting') : t('common.delete')}>
	<div class="space-y-2">
		<p class="text-sm" style="color: var(--color-muted)">{t('pageTemplates.deleteConfirmMessage')}</p>
		{#if deletingTemplate}
			<p class="text-sm font-medium">{deletingTemplate.name}</p>
		{/if}
	</div>
</Modal>

{#snippet TemplateActionsCell({ value, row }: { value: any; row: PageTemplate })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => createPageFromTemplate(row)} title={t('pageTemplates.createPage', 'Create Page from Template')}>
			<FilePlus class="h-3 w-3" style="color: var(--color-success)" />
		</Button>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
			</Button>
			{#if !row.is_system}
				<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
					<Trash2 class="h-3 w-3" />
				</Button>
			{/if}
		{/if}
	</div>
{/snippet}
