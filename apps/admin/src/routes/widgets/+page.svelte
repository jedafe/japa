<script lang="ts">
	import { api, widgetsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';

	type Widget = {
		id: string;
		name: string;
		widget_type: string;
		config: Record<string, unknown>;
		is_active: boolean;
		created_at: string;
	};

	let widgets = $state<Widget[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let filterValues = $state<Record<string, string>>({});
	let editingWidget = $state<Widget | null>(null);
	let saving = $state(false);

	let showWidgetModal = $state(false);
	let name = $state('');
	let widgetType = $state('');
	let isActive = $state(true);

	const columns = [
		{ key: 'name', label: t('widgets.name'), sortable: true },
		{ key: 'widget_type', label: t('widgets.type'), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>` },
		{
			key: 'is_active',
			label: t('widgets.status'),
			render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('widgets.active') : t('widgets.inactive')}</span>`
		},
		{
			key: 'actions',
			label: t('common.actions'),
			component: WidgetActionsCell
		}
	];

	const filteredWidgets = $derived.by(() => {
		let items = widgets;
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			items = items.filter((w) => w.name.toLowerCase().includes(q) || w.widget_type.toLowerCase().includes(q));
		}
		return items;
	});

	const filterDefs = [
		{
			key: 'is_active',
			label: t('widgets.status', 'Status'),
			options: [
				{ value: 'true', label: t('widgets.active', 'Active') },
				{ value: 'false', label: t('widgets.inactive', 'Inactive') }
			]
		}
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (filterValues.is_active) params.set('is_active', filterValues.is_active);
			if (searchQuery) params.set('q', searchQuery);
			const result = await api<Widget[]>(`/admin/widgets${params.toString() ? `?${params.toString()}` : ''}`);
			if (result.ok) {
				widgets = result.data ?? [];
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
		editingWidget = null;
		name = '';
		widgetType = '';
		isActive = true;
		showWidgetModal = true;
	}

	function openEdit(widget: Widget) {
		editingWidget = widget;
		name = widget.name;
		widgetType = widget.widget_type;
		isActive = widget.is_active;
		showWidgetModal = true;
	}

	async function createWidget() {
		if (!name.trim() || !widgetType.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/widgets', {
				method: 'POST',
				body: { name: name.trim(), widget_type: widgetType.trim(), is_active: isActive }
			});
			if (result.ok) {
				showWidgetModal = false;
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

	async function updateWidget() {
		if (!editingWidget || !name.trim() || !widgetType.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/widgets/${editingWidget.id}`, {
				method: 'PATCH',
				body: { name: name.trim(), widget_type: widgetType.trim(), is_active: isActive }
			});
			if (result.ok) {
				showWidgetModal = false;
				editingWidget = null;
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

	async function deleteWidget(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/widgets/${id}`, { method: 'DELETE' });
			widgets = widgets.filter((w) => w.id !== id);
		} catch {
			errorMessage = t('common.error');
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('widgets.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('widgets.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('widgets.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('widgets.create')}
	</button>
</div>

<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		columns={columns}
		data={filteredWidgets}
		pageSize={25}
		emptyMessage={t('widgets.empty')}
		searchable
		onsearch={(q) => { searchQuery = q; }}
	/>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={showWidgetModal} title={editingWidget ? t('widgets.edit') : t('widgets.create')} onConfirm={editingWidget ? updateWidget : createWidget} confirmLabel={saving ? t('common.saving') : editingWidget ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="wd-name" class="mb-1 block text-sm font-medium">{t('widgets.name')}</label>
			<input id="wd-name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="wd-type" class="mb-1 block text-sm font-medium">{t('widgets.type')}</label>
			<input id="wd-type" type="text" bind:value={widgetType} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div class="flex items-center gap-2">
			<input id="wd-active" type="checkbox" bind:checked={isActive} class="rounded" />
			<label for="wd-active" class="text-sm">{t('widgets.active')}</label>
		</div>
	</div>
</Modal>

{#snippet WidgetActionsCell({ value, row }: { value: any; row: Widget })}
	<div class="flex items-center justify-end gap-1">
		<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => openEdit(row)}>
			<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
		</button>
		<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => deleteWidget(row.id)}>
			<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
		</button>
	</div>
{/snippet}
