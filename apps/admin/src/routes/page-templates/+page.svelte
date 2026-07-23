<script lang="ts">
	import { api, pageTemplatesAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import Modal from '$lib/components/Modal.svelte';
	import { FilterBar } from '$lib/components/ui/filter-bar';

	const canManage = $derived(hasPermission('pagebuilder.templates.manage'));

	type PageTemplate = {
		id: string;
		name: string;
		slug: string;
		description?: string;
		block_count: number;
		is_active: boolean;
		created_at: string;
	};

	let templates = $state<PageTemplate[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let searchTerm = $state('');

	const filterDefs = [
		{ key: "is_active", label: "Status", options: [
			{ value: "true", label: "Active" },
			{ value: "false", label: "Inactive" }
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

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingTemplate = $state<PageTemplate | null>(null);
	let saving = $state(false);

	let modalOpen = $state(false);

	let name = $state('');
	let slug = $state('');
	let description = $state('');
	let isActive = $state(true);

	const filteredTemplates = $derived(
		searchTerm
			? templates.filter(
					(t) =>
						t.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
						t.slug.toLowerCase().includes(searchTerm.toLowerCase()) ||
						(t.description ?? '').toLowerCase().includes(searchTerm.toLowerCase())
				)
			: templates
	);

	const columns = [
		{ key: 'name', label: t('pageTemplates.name'), sortable: true,
			render: (v: string) => `<span class="font-medium">${v}</span>`
		},
		{ key: 'slug', label: t('pageTemplates.slug'), sortable: true,
			render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{ key: 'block_count', label: t('pageTemplates.blocks'), sortable: true },
		{ key: 'is_active', label: t('pageTemplates.status'), sortable: true,
			render: (v: boolean) => {
				const cls = v ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted';
				const text = v ? t('pageTemplates.active') : t('pageTemplates.inactive');
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${text}</span>`;
			}
		},
		{ key: 'actions', label: t('common.actions'), class: 'w-24', component: TemplateActionsCell }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (searchTerm.trim()) params.set('q', searchTerm.trim());
			if (filterValues.is_active) params.set('is_active', filterValues.is_active);
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
		name = '';
		slug = '';
		description = '';
		isActive = true;
		modalOpen = true;
	}

	function openEdit(tpl: PageTemplate) {
		editingTemplate = tpl;
		name = tpl.name;
		slug = tpl.slug;
		description = tpl.description || '';
		isActive = tpl.is_active;
		modalOpen = true;
	}

	async function createTemplate() {
		if (!name.trim() || !slug.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/page-templates', {
				method: 'POST',
				body: { name: name.trim(), slug: slug.trim(), description: description.trim() || undefined, is_active: isActive }
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
		if (!editingTemplate || !name.trim() || !slug.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/page-templates/${editingTemplate.id}`, {
				method: 'PATCH',
				body: { name: name.trim(), slug: slug.trim(), description: description.trim() || undefined, is_active: isActive }
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

	async function deleteTemplate(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/page-templates/${id}`, { method: 'DELETE' });
			templates = templates.filter((t) => t.id !== id);
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
			<label for="pt-slug" class="mb-1 block text-sm font-medium">{t('pageTemplates.slug')}</label>
			<input id="pt-slug" type="text" bind:value={slug} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="pt-desc" class="mb-1 block text-sm font-medium">{t('pageTemplates.description')}</label>
			<textarea id="pt-desc" bind:value={description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input id="pt-active" type="checkbox" bind:checked={isActive} class="rounded" />
			<label for="pt-active" class="text-sm">{t('pageTemplates.active')}</label>
		</div>
	</div>
</Modal>

{#snippet TemplateActionsCell({ value, row }: { value: any; row: PageTemplate })}
	{#if canManage}
		<div class="flex items-center gap-1">
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => deleteTemplate(row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3 w-3" />
			</Button>
		</div>
	{/if}
{/snippet}
