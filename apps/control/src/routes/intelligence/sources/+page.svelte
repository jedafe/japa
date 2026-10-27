<script lang="ts">
	import { api, apiList } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import SourceForm from '$lib/components/intelligence/SourceForm.svelte';
	import type { SourceFormValues, SourceKindOption } from '$lib/components/intelligence/SourceForm.svelte';
	import ChannelsTable from '$lib/components/intelligence/ChannelsTable.svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Plus, Pencil, Trash2, Power, RefreshCw, Database, Zap, Ban, Waypoints } from 'lucide-svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	interface IntelSource {
		id: string;
		name: string;
		domain: string;
		source_url: string;
		category: string;
		categories?: string[] | null;
		source_type: string;
		source_kind?: string | null;
		slug?: string | null;
		description?: string | null;
		logo_url?: string | null;
		trust_level: number;
		crawl_method: string;
		crawl_frequency: string;
		crawl_frequency_mins: number;
		attribution_required: boolean;
		source_license?: string | null;
		usage_restriction?: string | null;
		enabled: boolean;
		status?: string;
		persona_id?: string | null;
		/** Extraction-domain override from provider_config.entity_types —
		 *  a mixed-domain source declares every entity type it publishes. */
		entity_types?: string[] | null;
		keywords?: string[] | null;
		detected_methods?: string[] | null;
		probe_report?: Record<string, unknown> | null;
		last_probed_at?: string | null;
		last_crawled_at?: string | null;
		last_success_at?: string | null;
		last_error_at?: string | null;
		last_error_message?: string | null;
		consecutive_failures?: number;
		created_at?: string;
	}

	interface PersonaItem {
		id: string;
		profile_name: string;
		profile_slug?: string;
		backing_username?: string;
		status?: string;
	}

	let {} = $props();

	// RBAC CRUD gate: managing sources requires intel.sources.manage (or
	// wildcard). Viewing the registry requires intel.sources.view.
	const canView = $derived(hasAnyPermission(['intel.sources.view', 'intel.sources.manage', 'intel.sources.*']));
	const canManage = $derived(hasAnyPermission(['intel.sources.manage', 'intel.sources.*']));
	// INTEL-018 fix: crawl-now and disable-source permissions.
	const canCrawlNow = $derived(hasAnyPermission(['intel.sources.crawl_now', 'intel.sources.*']));
	const canDisableSource = $derived(hasAnyPermission(['intel.review.disable_source', 'intel.review.*', 'intel.sources.*']));
	// intel-channels Phase 1: the channels editor lists a source's
	// intel_source_channels rows; per-action nodes are enforced inside
	// ChannelsTable (migration 000684).
	const canListChannels = $derived(hasAnyPermission(['intel.channels.list', 'intel.channels.*']));

	let sources = $state<IntelSource[]>([]);
	let personas = $state<PersonaItem[]>([]);
	let entityTypeOptions = $state<string[]>([]);
	let sourceTypeOptions = $state<string[]>([]);
	let categoryOptions = $state<string[]>([]);
	let sourceKindOptions = $state<SourceKindOption[]>([]);
	let crawlMethodOptions = $state<string[]>([]);
	let crawlFrequencyOptions = $state<string[]>([]);
	let pipelineOptions = $state<string[]>([]);
	let personaNameById = $derived.by(() => {
		const m = new Map<string, string>();
		for (const p of personas) m.set(p.id, p.profile_name);
		return m;
	});
	let loading = $state(true);
	let refreshing = $state(false);
	let errorMessage = $state('');
	let noticeMessage = $state('');
	let filter = $state('');
	let filterCategory = $state('all');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());
	let totalRows = $state(0);
	let bulkBusy = $state(false);

	let formOpen = $state(false);
	let editingSource = $state<IntelSource | null>(null);
	let formSaving = $state(false);
	let formError = $state('');

	let channelsOpen = $state(false);
	let channelsSource = $state<IntelSource | null>(null);

	let confirmDelete = $state<IntelSource | null>(null);
	let deleteOpen = $state(false);
	let deleting = $state(false);

	async function load() {
		refreshing = true;
		// Use include_disabled=true so ALL sources (including disabled and
		// proposed/rejected rows) are returned with full crawl-state fields
		// (BUG-004 fix). Filtering, sorting, and pagination run server-side so
		// the registry isn't silently capped at one page.
		const params = new URLSearchParams();
		params.set('include_disabled', 'true');
		params.set('page_size', String(pageSize));
		params.set('page', String(page + 1)); // backend pagination is 1-based
		if (debouncedFilter) params.set('q', debouncedFilter);
		if (filterCategory !== 'all') params.set('category', filterCategory);
		if (sortKey) {
			params.set('sort', sortKey);
			params.set('dir', sortDirection);
		}
		const result = await apiList<IntelSource[]>(
			`/api/v1/intel/sources?${params.toString()}`
		);
		if (result.ok) {
			sources = Array.isArray(result.data) ? (result.data as IntelSource[]) : [];
			totalRows = Number(result.meta?.total ?? sources.length);
			errorMessage = ''; // clear a stale banner after a successful refresh
		} else {
			errorMessage = result.error;
		}
		// Fetch the AI persona list so the table can resolve persona_id →
		// profile name and the SourceForm dropdown can list them. The
		// endpoint requires persona.view (ADR-068).
		if (personas.length === 0) {
			const presult = await api<PersonaItem[]>(
				'/api/v1/admin/personas?status=active&page_size=200'
			);
			if (presult.ok) {
				personas = Array.isArray(presult.data) ? (presult.data as PersonaItem[]) : [];
			}
		}
		// Fetch the registered extraction entity types so the SourceForm can
		// offer the entity_types multi-domain override (Rule 16 — options
		// come from the API, not a hardcoded list). The same meta payload
		// carries source_types plus the admin category/crawl option sets,
		// which feed the Source Type, Category, Method, and Frequency selects.
		if (entityTypeOptions.length === 0) {
			const mresult = await api<{
				registered_entity_types?: string[];
				source_types?: string[];
				admin_categories?: string[];
				source_kinds?: { value: string; label: string }[];
				crawl_methods?: string[];
				crawl_frequencies?: string[];
				channel_pipelines?: string[];
			}>('/api/v1/sources/meta');
			if (mresult.ok && mresult.data) {
				entityTypeOptions = mresult.data.registered_entity_types ?? [];
				sourceTypeOptions = mresult.data.source_types ?? [];
				categoryOptions = mresult.data.admin_categories ?? [];
				sourceKindOptions = mresult.data.source_kinds ?? [];
				crawlMethodOptions = mresult.data.crawl_methods ?? [];
				crawlFrequencyOptions = mresult.data.crawl_frequencies ?? [];
				pipelineOptions = mresult.data.channel_pipelines ?? [];
			}
		}
		loading = false;
		refreshing = false;
	}

	// Server-side list: debounce the search box, then reload whenever any
	// server parameter (page/page-size/sort/category/search) changes.
	let filterDebounce: ReturnType<typeof setTimeout> | null = null;
	let debouncedFilter = $state('');

	$effect(() => {
		void filter;
		if (filterDebounce) clearTimeout(filterDebounce);
		filterDebounce = setTimeout(() => {
			if (debouncedFilter !== filter) page = 0;
			debouncedFilter = filter;
		}, 300);
	});

	$effect(() => {
		void page;
		void pageSize;
		void sortKey;
		void sortDirection;
		void filterCategory;
		void debouncedFilter;
		load();
	});

	// Category options come from the API taxonomy (admin_categories via
	// /sources/meta) — the filtered list is server-side, so deriving the
	// picker from the loaded page would hide most categories.
	const categoryItems = $derived.by(() => {
		const items = [{ value: 'all', label: t('intel.filter_all_categories', 'All categories') }];
		for (const c of categoryOptions) {
			items.push({
				value: c,
				label: t(`intel.category_${c}`, c)
			});
		}
		return items;
	});

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
			case 5: return t('intel.trust_unverified', 'Unverified');
			default: return t('intel.trust_unrated', 'Unrated');
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
			render: (val, row) => {
				const cats = (row as unknown as IntelSource).categories;
				const extra = cats && cats.length > 1 ? ` <span class="text-xs">+${cats.length - 1}</span>` : '';
				return `<span class="text-muted-foreground capitalize">${escapeHtml(val)}</span>${extra}`;
			}
		},
		{
			key: 'persona_id',
			label: t('intel.field_persona', 'Persona'),
			sortable: false,
			render: (val) => {
				if (!val) return `<span class="text-muted-foreground text-xs">${t('intel.field_persona_none', 'No persona')}</span>`;
				const name = personaNameById.get(val as string);
				return `<span class="inline-flex items-center gap-1 text-xs font-medium text-violet-500"><span class="size-1.5 rounded-full bg-violet-500"></span>${escapeHtml(name ?? t('intel.persona_assigned', 'Assigned'))}</span>`;
			}
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
			label: t('intel.field_enabled', 'Enabled'),
			sortable: true,
			render: (val) =>
				val
					? `<span class="inline-flex items-center gap-1 text-xs font-medium text-emerald-500"><span class="size-1.5 rounded-full bg-emerald-500"></span>${t('intel.enabled', 'Enabled')}</span>`
					: `<span class="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground"><span class="size-1.5 rounded-full bg-muted-foreground/40"></span>${t('intel.disabled', 'Disabled')}</span>`
		},
		{
			key: 'status',
			label: t('intel.status', 'Status'),
			sortable: true,
			render: (val) => {
				const cls = val === 'active'
					? 'text-emerald-500 bg-emerald-500/10'
					: val === 'proposed'
						? 'text-amber-500 bg-amber-500/10'
						: 'text-muted-foreground bg-muted';
				return `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${escapeHtml(String(val))}</span>`;
			}
		}
	];

	const rowActions = $derived.by<DataTableRowAction[]>(() => {
		const acts: DataTableRowAction[] = [];
		if (canCrawlNow) {
			acts.push({
				label: t('intel.crawl_now', 'Crawl Now'),
				icon: Zap,
				onClick: (row) => crawlNow(row as unknown as IntelSource)
			});
		}
		if (canListChannels) {
			acts.push({
				label: t('intel.channels_title', 'Channels'),
				icon: Waypoints,
				onClick: (row) => openChannels(row as unknown as IntelSource)
			});
		}
		if (canManage) {
			acts.push({
				label: t('intel.edit', 'Edit'),
				icon: Pencil,
				onClick: (row) => openEdit(row as unknown as IntelSource)
			});
			acts.push({
				label: t('intel.toggle_enable', 'Toggle Enable'),
				icon: Power,
				// Enabling is only valid on approved (active) sources — a
				// proposed/rejected row must go through the approve/reject
				// flow on the Proposed queue, not a raw enabled toggle.
				hidden: (row) => (row as unknown as IntelSource).status !== 'active',
				onClick: (row) => toggleEnable(row as unknown as IntelSource)
			});
		}
		if (canDisableSource) {
			acts.push({
				label: t('intel.disable_source', 'Disable Source'),
				icon: Ban,
				variant: 'destructive',
				onClick: (row) => disableSource(row as unknown as IntelSource)
			});
		}
		if (canManage) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onClick: (row) => openDelete(row as unknown as IntelSource)
			});
		}
		return acts;
	});

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

	function openChannels(s: IntelSource) {
		channelsSource = s;
		channelsOpen = true;
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
		// Build the body. persona_id is sent as null when unassigned: on PATCH
		// an explicit null clears the attribution (the update query COALESCEs,
		// so null is the only way to clear), and on create nil is skipped.
		const body: Record<string, unknown> = { ...values };
		body.persona_id = values.persona_id || null;
		// description/logo_url: null on create stores NULL; on PATCH the raw
		// string is sent so clearing the field writes "".
		body.description = isEdit ? values.description : values.description || null;
		body.logo_url = isEdit ? values.logo_url : values.logo_url || null;
		// slug is optional — absent lets the backend generate it from the name.
		if (!body.slug) {
			delete body.slug;
		}
		if (!body.source_kind) {
			delete body.source_kind;
		}
		// Dual-write categories: the admin create/update handlers still read
		// the single `category` field (already set to categories[0] by the
		// form), while `categories` carries the full multi-select for the
		// intel-channels schema — the backend ignores it until the handlers
		// accept the column.
		const result = await api<IntelSource>(path, {
			method,
			body
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

	// INTEL-018 fix: crawl-now triggers an immediate fetch via intel:fetch_source.
	async function crawlNow(s: IntelSource) {
		const result = await api(`/api/v1/intel/sources/${s.id}/crawl-now`, { method: 'POST' });
		if (result.ok) {
			await load();
		} else {
			errorMessage = result.error;
		}
	}

	// INTEL-018 fix: disable-source from the review queue surface.
	async function disableSource(s: IntelSource) {
		const result = await api(`/api/v1/intel/sources/${s.id}/disable`, { method: 'POST' });
		if (result.ok) await load();
		else errorMessage = result.error;
	}

	async function bulkToggle(ids: string[], enabled: boolean) {
		if (bulkBusy) return;
		bulkBusy = true;
		noticeMessage = '';
		errorMessage = '';
		// Only active sources can be enabled — proposed/rejected rows must go
		// through the approve flow. Skip them up front instead of collecting
		// a 409 per row (the row-level Toggle action hides them the same way).
		const statusById = new Map(sources.map((s) => [s.id, s.status]));
		const eligible = enabled ? ids.filter((id) => statusById.get(id) === 'active') : ids;
		const skipped = ids.length - eligible.length;
		const results = await Promise.allSettled(
			eligible.map((id) =>
				api(`/api/v1/intel/sources/${id}`, { method: 'PATCH', body: { enabled } })
					.then((r) => (r.ok ? null : r.error))
			)
		);
		const failures: string[] = [];
		results.forEach((r, i) => {
			if (r.status === 'rejected') {
				failures.push(ids[i].slice(0, 8));
			} else if (r.value) {
				failures.push(`${ids[i].slice(0, 8)}: ${r.value}`);
			}
		});
		const succeeded = eligible.length - failures.length;
		if (failures.length === 0) {
			noticeMessage = t('intel.bulk_done', '{done} of {total} sources updated.')
				.replace('{done}', String(succeeded))
				.replace('{total}', String(eligible.length));
			if (skipped > 0) {
				noticeMessage += ' ' + t('intel.bulk_skipped_not_active', '{n} skipped — approve proposed sources first.')
					.replace('{n}', String(skipped));
			}
		} else {
			errorMessage = t('intel.bulk_partial', '{done} of {total} sources updated; {failed} failed: {list}')
				.replace('{done}', String(succeeded))
				.replace('{total}', String(ids.length))
				.replace('{failed}', String(failures.length))
				.replace('{list}', failures.slice(0, 5).join('; '));
		}
		selectedIds = new Set();
		await load();
		bulkBusy = false;
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

	// --- Autocomplete fetcher for category filter ---
	// Filters the derived categoryItems (from fetched sources) by query.
	async function fetchCategories(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return categoryItems
			.filter((c) => c.label.toLowerCase().includes(query) || c.value.toLowerCase().includes(query))
			.map((c) => ({ id: c.value, label: c.label }));
	}

	async function fetchCategoriesInitial(): Promise<AutocompleteItem[]> {
		return categoryItems.map((c) => ({ id: c.value, label: c.label }));
	}

	function getCategorySelectedLabel(value: string): string | null {
		if (!value || value === 'all') return null;
		return categoryItems.find((c) => c.value === value)?.label ?? value;
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
	{#if noticeMessage}
		<div class="bg-emerald-500/10 text-emerald-500 mb-4 rounded-md p-3 text-sm">
			{noticeMessage}
		</div>
	{/if}

	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<div class="flex flex-1 flex-wrap items-center gap-2">
			<SearchInput bind:value={filter} placeholder={t('intel.search_sources', 'Search sources...')} class="w-64" />
			<Autocomplete
				placeholder={t('intel.filter_all_categories', 'All categories')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchCategories}
				initialFetcher={fetchCategoriesInitial}
				selectedId={filterCategory !== 'all' ? filterCategory : null}
				selectedLabel={getCategorySelectedLabel(filterCategory)}
				onSelect={(item) => { filterCategory = item.id; page = 0; }}
				onClear={() => { filterCategory = 'all'; page = 0; }}
			/>
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
		rows={sources}
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
		personas={personas}
		entityTypeOptions={entityTypeOptions}
		sourceTypeOptions={sourceTypeOptions}
		categoryOptions={categoryOptions}
		sourceKindOptions={sourceKindOptions}
		crawlMethodOptions={crawlMethodOptions}
		crawlFrequencyOptions={crawlFrequencyOptions}
		saving={formSaving}
		errorMessage={formError}
		onSubmit={handleSubmit}
	/>

	<ChannelsTable
		bind:open={channelsOpen}
		source={channelsSource}
		entityTypeOptions={entityTypeOptions}
		crawlMethodOptions={crawlMethodOptions}
		pipelineOptions={pipelineOptions}
	/>

	<ConfirmModal
		bind:open={deleteOpen}
		title={t('intel.delete_source', 'Delete Source')}
		consequence={confirmDelete ? t('intel.delete_consequence', 'This removes the source from the registry. Existing promoted records are preserved.') : ''}
		confirmText={confirmDelete?.name ?? ''}
		onConfirm={confirmDeleteSource}
	/>
{/if}
