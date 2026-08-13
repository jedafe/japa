<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import SourceForm from '$lib/components/intelligence/SourceForm.svelte';
	import type { SourceFormValues } from '$lib/components/intelligence/SourceForm.svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Plus, Pencil, Trash2, Power, RefreshCw, Database } from 'lucide-svelte';
	import { Select } from 'bits-ui';

	interface IntelSource {
		id: string;
		name: string;
		domain: string;
		source_url: string;
		category: string;
		source_type: string;
		trust_level: number;
		crawl_method: string;
		crawl_frequency: string;
		crawl_frequency_mins: number;
		attribution_required: boolean;
		source_license?: string | null;
		usage_restriction?: string | null;
		enabled: boolean;
		last_crawled_at?: string | null;
		last_success_at?: string | null;
		last_error_at?: string | null;
		last_error_message?: string | null;
		consecutive_failures?: number;
		created_at?: string;
	}

	let {} = $props();

	// RBAC CRUD gate: managing sources requires intel.sources.manage (or
	// wildcard). Viewing the registry requires intel.sources.view.
	const canView = $derived(hasAnyPermission(['intel.sources.view', 'intel.sources.*']));
	const canManage = $derived(hasAnyPermission(['intel.sources.manage', 'intel.sources.*']));

	let sources = $state<IntelSource[]>([]);
	let loading = $state(true);
	let refreshing = $state(false);
	let errorMessage = $state('');
	let filter = $state('');
	let filterCategory = $state('all');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());

	let formOpen = $state(false);
	let editingSource = $state<IntelSource | null>(null);
	let formSaving = $state(false);
	let formError = $state('');

	let confirmDelete = $state<IntelSource | null>(null);
	let deleteOpen = $state(false);
	let deleting = $state(false);

	async function load() {
		refreshing = true;
		errorMessage = '';
		const result = await api<{ data?: IntelSource[] } & Record<string, unknown>>(
			'/api/v1/intel/sources?page_size=100'
		);
		if (result.ok) {
			sources = Array.isArray(result.data?.data) ? (result.data!.data as IntelSource[]) : [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
		refreshing = false;
	}

	$effect(() => {
		load();
	});

	const categoryItems = $derived([
		{ value: 'all', label: t('intel.filter_all_categories', 'All categories') },
		{ value: 'scholarship', label: t('intel.category_scholarship', 'Scholarship') },
		{ value: 'job', label: t('intel.category_job', 'Job') },
		{ value: 'visa', label: t('intel.category_visa', 'Visa') },
		{ value: 'fx', label: t('intel.category_fx', 'FX') },
		{ value: 'remittance', label: t('intel.category_remittance', 'Remittance') },
		{ value: 'news', label: t('intel.category_news', 'News') },
		{ value: 'travel_alert', label: t('intel.category_travel_alert', 'Travel Alert') }
	]);

	let filtered = $derived(
		sources
			.filter((s) => {
				if (filterCategory !== 'all' && s.category !== filterCategory) return false;
				if (filter) {
					const q = filter.toLowerCase();
					if (
						!s.name?.toLowerCase().includes(q) &&
						!s.domain?.toLowerCase().includes(q) &&
						!s.source_url?.toLowerCase().includes(q)
					)
						return false;
				}
				return true;
			})
	);

	let totalRows = $derived(filtered.length);

	function trustColor(level: number): string {
		switch (level) {
			case 1: return 'text-emerald-500 bg-emerald-500/10';
			case 2: return 'text-sky-500 bg-sky-500/10';
			case 3: return 'text-amber-500 bg-amber-500/10';
			case 4: return 'text-muted-foreground bg-muted';
			default: return 'text-muted-foreground bg-muted';
		}
	}

	function trustLabel(level: number): string {
		switch (level) {
			case 1: return t('intel.trust_official', 'Official');
			case 2: return t('intel.trust_trusted', 'Trusted');
			case 3: return t('intel.trust_aggregator', 'Aggregator');
			case 4: return t('intel.trust_community', 'Community');
			default: return String(level);
		}
	}

	function formatTime(ts?: string | null): string {
		if (!ts) return '—';
		try {
			return new Date(ts).toLocaleDateString();
		} catch {
			return ts;
		}
	}

	const columns: DataTableColumn[] = [
		{
			key: 'name',
			label: t('intel.field_name', 'Name'),
			sortable: true,
			render: (_val, row) =>
				`<div class="flex flex-col"><span class="font-medium">${escapeHtml(row.name)}</span>` +
				`<span class="text-muted-foreground text-xs font-mono">${escapeHtml(row.domain)}</span></div>`
		},
		{
			key: 'category',
			label: t('intel.field_category', 'Category'),
			sortable: true,
			render: (val) => `<span class="text-muted-foreground capitalize">${escapeHtml(val)}</span>`
		},
		{
			key: 'crawl_method',
			label: t('intel.field_crawl_method', 'Method'),
			sortable: true,
			render: (val) => `<span class="font-mono text-xs text-muted-foreground uppercase">${escapeHtml(val)}</span>`
		},
		{
			key: 'trust_level',
			label: t('intel.field_trust_level', 'Trust'),
			sortable: true,
			render: (val) =>
				`<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${trustColor(Number(val))}">${trustLabel(Number(val))}</span>`
		},
		{
			key: 'last_crawled_at',
			label: t('intel.last_crawled', 'Last Crawled'),
			sortable: true,
			render: (val) => `<span class="text-muted-foreground text-xs">${formatTime(val as string)}</span>`
		},
		{
			key: 'enabled',
			label: t('intel.field_enabled', 'Status'),
			sortable: true,
			render: (val) =>
				val
					? `<span class="inline-flex items-center gap-1 text-xs font-medium text-emerald-500"><span class="size-1.5 rounded-full bg-emerald-500"></span>${t('intel.enabled', 'Enabled')}</span>`
					: `<span class="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground"><span class="size-1.5 rounded-full bg-muted-foreground/40"></span>${t('intel.disabled', 'Disabled')}</span>`
		}
	];

	const rowActions = $derived<DataTableRowAction[]>(
		canManage
			? [
					{
						label: t('intel.edit', 'Edit'),
						icon: Pencil,
						onClick: (row) => openEdit(row as unknown as IntelSource)
					},
					{
						label: t('intel.toggle_enable', 'Toggle Enable'),
						icon: Power,
						onClick: (row) => toggleEnable(row as unknown as IntelSource)
					},
					{
						label: t('common.delete', 'Delete'),
						icon: Trash2,
						variant: 'destructive',
						onClick: (row) => openDelete(row as unknown as IntelSource)
					}
				]
			: []
	);

	const bulkActions = $derived<DataTableBulkAction[]>(
		canManage
			? [
					{
						label: t('intel.enable_selected', 'Enable selected'),
						icon: Power,
						onClick: (ids) => bulkToggle(ids, true)
					},
					{
						label: t('intel.disable_selected', 'Disable selected'),
						icon: Power,
						variant: 'destructive',
						onClick: (ids) => bulkToggle(ids, false)
					}
				]
			: []
	);

	function openCreate() {
		editingSource = null;
		formError = '';
		formOpen = true;
	}

	function openEdit(s: IntelSource) {
		editingSource = s;
		formError = '';
		formOpen = true;
	}

	async function handleSubmit(values: SourceFormValues) {
		formSaving = true;
		formError = '';
		const isEdit = !!editingSource?.id;
		// The source registry CRUD endpoints follow the /api/v1/intel/sources
		// convention. Create = POST, update = PATCH/{id}.
		const path = isEdit
			? `/api/v1/intel/sources/${editingSource!.id}`
			: '/api/v1/intel/sources';
		const method = isEdit ? 'PATCH' : 'POST';
		const result = await api<{ data?: IntelSource } & Record<string, unknown>>(path, {
			method,
			body: values
		});
		formSaving = false;
		if (result.ok) {
			formOpen = false;
			await load();
		} else {
			formError = result.error;
		}
	}

	async function toggleEnable(s: IntelSource) {
		const result = await api(`/api/v1/intel/sources/${s.id}`, {
			method: 'PATCH',
			body: { enabled: !s.enabled }
		});
		if (result.ok) await load();
		else errorMessage = result.error;
	}

	async function bulkToggle(ids: string[], enabled: boolean) {
		await Promise.allSettled(
			ids.map((id) => api(`/api/v1/intel/sources/${id}`, { method: 'PATCH', body: { enabled } }))
		);
		await load();
	}

	function openDelete(s: IntelSource) {
		confirmDelete = s;
		deleteOpen = true;
	}

	async function confirmDeleteSource() {
		if (!confirmDelete) return;
		deleting = true;
		const result = await api(`/api/v1/intel/sources/${confirmDelete.id}`, { method: 'DELETE' });
		deleting = false;
		if (result.ok) {
			confirmDelete = null;
			await load();
		} else {
			errorMessage = result.error;
		}
	}
</script>

<PageHeader
	title={t('intel.sources_title', 'Intelligence Sources')}
	description={t('intel.sources_desc', 'Registry of crawl sources feeding the intelligence pipeline.')}
/>

{#if !canView}
	<AccessDenied message={t('intel.sources_access_denied', 'You need intel.sources.view permission to view the source registry.')} />
{:else}
	{#if errorMessage}
		<div class="bg-destructive/10 text-destructive mb-4 rounded-md p-3 text-sm">
			{errorMessage}
		</div>
	{/if}

	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<div class="flex flex-1 flex-wrap items-center gap-2">
			<SearchInput bind:value={filter} placeholder={t('intel.search_sources', 'Search sources...')} class="w-64" />
			<Select.Root
				type="single"
				value={filterCategory}
				onValueChange={(v) => { if (v !== undefined) { filterCategory = v; page = 0; } }}
			>
				<Select.Trigger class="border-border-input bg-background text-foreground h-input rounded-9px border px-3 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus">
					<Select.Value placeholder={t('intel.filter_all_categories', 'All categories')} />
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border">
						<Select.Viewport class="p-1">
							{#each categoryItems as c}
								<Select.Item class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none" value={c.value} label={c.label}>
									{c.label}
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
		</div>
		<div class="flex items-center gap-2">
			<button
				class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50"
				disabled={refreshing}
				onclick={load}
			>
				<RefreshCw class="size-4 {refreshing ? 'animate-spin' : ''}" />
				{t('common.refresh', 'Refresh')}
			</button>
			{#if canManage}
				<button
					class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90"
					onclick={openCreate}
				>
					<Plus class="size-4" />
					{t('intel.new_source', 'New Source')}
				</button>
			{/if}
		</div>
	</div>

	<DataTable
		{columns}
		rows={filtered}
		{totalRows}
		bind:page
		bind:pageSize
		bind:sortKey
		bind:sortDirection
		bind:selectedIds
		{rowActions}
		{bulkActions}
		{loading}
		emptyMessage={t('intel.no_sources', 'No intelligence sources registered')}
		emptyIcon={Database}
	/>

	<SourceForm
		bind:open={formOpen}
		source={editingSource}
		saving={formSaving}
		errorMessage={formError}
		onSubmit={handleSubmit}
	/>

	<ConfirmModal
		bind:open={deleteOpen}
		title={t('intel.delete_source', 'Delete Source')}
		consequence={confirmDelete ? t('intel.delete_consequence', 'This removes the source from the registry. Existing promoted records are preserved.') : ''}
		confirmText={confirmDelete?.name ?? ''}
		onConfirm={confirmDeleteSource}
	/>
{/if}
