<script lang="ts">
	// /tools/visa-routes — admin management page for visa route data.
	// Full CRUD: create, edit, deactivate (soft-delete), approve (HITL), generate (AI draft).
	// Ref: §90 (Visa Route Comparison), ADR-098, ADR-088 (Universal AI content generation).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles, Trash2 } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { visaRoutesAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';

	// RouteDTO — matches the backend response shape (internal/visaroutes/service.go).
	type VisaRoute = {
		id: string;
		destination_space_id: string;
		destination_space_name?: string;
		destination_space_code?: string;
		visa_category: string;
		name: string;
		slug: string;
		eligibility_summary?: string;
		requirements?: Record<string, unknown> | null;
		typical_fee_usd?: number | null;
		path_to_pr?: string;
		renewal_terms?: string;
		restrictions?: string;
		gov_source_url: string;
		source_url_last_checked?: string;
		status: string;
		is_active: boolean;
		ai_generated: boolean;
		ai_model?: string;
		ai_prompt_hash?: string;
		hitl_reviewed_by?: string;
		hitl_reviewed_at?: string;
		hitl_review_notes?: string;
		is_japabot_indexed: boolean;
		created_at: string;
		updated_at: string;
	};

	const canManage = $derived(hasPermission('visa_routes.manage'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	// ── State ───────────────────────────────────────────────────────────────

	let entityController = $state<EntityPageController | undefined>(undefined);

	let visaRouteStatusOptions = $state<CatalogOption[]>([
		{ value: 'draft', label: 'Draft' },
		{ value: 'ai_draft', label: 'AI Draft' },
		{ value: 'pending', label: 'Pending' },
		{ value: 'published', label: 'Published' },
		{ value: 'rejected', label: 'Rejected' }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('tools.visa_routes.status');
			if (res.ok && res.data?.length) visaRouteStatusOptions = res.data;
		})();
	});

	let pageError = $state('');
	let acting = $state(false);
	let deleting = $state(false);

	// ── Generate modal state ────────────────────────────────────────────────
	let showGenerateModal = $state(false);
	let generateDestSpaceId = $state('');
	let generateVisaCategory = $state('');

	function openGenerate() {
		generateDestSpaceId = '';
		generateVisaCategory = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateDestSpaceId) {
			pageError = t('app.tools.dest_required', 'Destination is required');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await visaRoutesAdminApi.generate({
				destination_space_id: generateDestSpaceId,
				visa_category: generateVisaCategory || undefined
			});
			if (result.ok) {
				showGenerateModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function approveItem(item: VisaRoute) {
		acting = true;
		pageError = '';
		try {
			const result = await visaRoutesAdminApi.approve(item.id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Delete confirm modal ────────────────────────────────────────────────
	let showDeleteModal = $state(false);
	let pendingDeleteId = $state('');

	function openDelete(row: VisaRoute) {
		pendingDeleteId = row.id;
		showDeleteModal = true;
	}

	async function confirmDelete() {
		if (!pendingDeleteId || deleting) return;
		deleting = true;
		pageError = '';
		try {
			const result = await visaRoutesAdminApi.delete(pendingDeleteId);
			if (result.ok) {
				showDeleteModal = false;
				pendingDeleteId = '';
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			deleting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: VisaRoute, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveItem,
				show: (row: VisaRoute) => row.status !== 'published'
			});
			acts.push({
				label: t('common.deactivate', 'Deactivate'),
				icon: Trash2,
				onclick: openDelete,
				variant: 'destructive',
				show: (row: VisaRoute) => row.is_active
			});
		}
		return acts;
	});

	// ── Country autocomplete helpers (250 primary geo space countries) ───────
	// Sourced from GET /api/v1/geo/countries via useCountries(); filtered
	// client-side for live debounced Autocomplete. Keyed by space ID.
	const countryReady = $derived(countryUtils.ready);

	function fetchCountryId(q: string): Promise<{ id: string; label: string; sublabel?: string }[]> {
		return countryUtils.fetcher(q);
	}

	function fetchCountryIdInitial(): Promise<{ id: string; label: string; sublabel?: string }[]> {
		return countryUtils.initialFetcher();
	}

	function getCountryIdLabel(id: string): string {
		return countryUtils.getSelectedLabel(id);
	}

	// ── Form fields (drives EntityForm for create/edit) ─────────────────────
	const fields = $derived.by<FieldDef[]>(() => {
		countryReady; // re-derive when the 250 primary geo space countries load
		return [
			{ key: 'name', label: t('app.tools.col_name', 'Name'), type: 'text', help: t('app.tools.name_hint', 'The name shown in admin lists and on the site where public.'),},
			{ key: 'visa_category', label: t('app.tools.col_visa_type', 'Visa Category'), type: 'text', help: t('app.tools.visa_category_hint', 'The visa category for this record.') },
			{ key: 'destination_space_id', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'autocomplete', help: t('app.tools.destination_space_id_hint', 'The destination space this is linked to.'), placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: fetchCountryId, initialFetcher: fetchCountryIdInitial, getSelectedLabel: getCountryIdLabel },
			{ key: 'slug', label: t('app.tools.col_slug', 'Slug'), type: 'text', help: t('app.tools.slug_hint', 'URL-safe identifier — lowercase, hyphens, no spaces.') },
			{ key: 'eligibility_summary', label: t('app.tools.col_summary', 'Eligibility Summary'), type: 'textarea', help: t('app.tools.eligibility_summary_hint', 'Free text — multiple lines allowed.') },
			{ key: 'gov_source_url', label: t('app.tools.col_gov_source_url', 'Government Source URL'), type: 'url', help: t('app.tools.gov_source_url_hint', 'Official government source — used to verify data.') },
			{ key: 'typical_fee_usd', label: t('app.tools.col_typical_fee', 'Typical Fee (USD)'), type: 'number', help: t('app.tools.typical_fee_usd_hint', 'Typical fee in USD.') },
			{ key: 'path_to_pr', label: t('app.tools.col_path_to_pr', 'Path to PR'), type: 'textarea', help: t('app.tools.path_to_pr_hint', 'How this route leads to permanent residency.') },
			{ key: 'renewal_terms', label: t('app.tools.col_renewal_terms', 'Renewal Terms'), type: 'textarea', help: t('app.tools.renewal_terms_hint', 'How renewal works — automatic, manual, price.') },
			{ key: 'restrictions', label: t('app.tools.col_restrictions', 'Restrictions'), type: 'textarea', help: t('app.tools.restrictions_hint', 'Limits or conditions that apply.') },
			{
				key: 'status',
				label: t('app.tools.col_status', 'Status'),
				type: 'select',
			help: t('app.tools.status_hint', 'The record state — controls visibility and actions.'),
				options: visaRouteStatusOptions
			},
			{ key: 'is_active', label: t('app.tools.col_is_active', 'Active'), type: 'checkbox', help: t('app.tools.is_active_hint', 'Inactive records are hidden or disabled on the site.') }
		];
	});

	// ── Filters ─────────────────────────────────────────────────────────────
	const statusFilterOptions = $derived<CatalogOption[]>([
		{ value: '', label: t('common.all', 'All') },
		...visaRouteStatusOptions
	]);

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.visa_routes_heading', 'Visa Routes Management'),
		description: t('app.tools.visa_routes_desc', 'Manage visa route data — eligibility, fees, path to PR, and renewal terms'),
		createLabel: t('common.create', 'Create'),
		createPermission: 'visa_routes.manage',
		editPermission: 'visa_routes.manage',
		deletePermission: 'visa_routes.manage',
		pageSize: 25,
		filters: [
			{ key: 'status', label: t('app.tools.col_status', 'Status'), options: statusFilterOptions },
			{
				key: 'dest',
				label: t('app.tools.col_destination', 'Destination'),
				type: 'autocomplete',
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: fetchCountryId,
				initialFetcher: fetchCountryIdInitial,
				getSelectedLabel: getCountryIdLabel
			}
		],
		columns: [
			{ key: 'name', label: t('app.tools.col_name', 'Name'), sortable: true },
			{
				key: 'destination_space_name',
				label: t('app.tools.col_dest_country', 'Destination'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'visa_category',
				label: t('app.tools.col_visa_type', 'Visa Category'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'status',
				label: t('app.tools.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'ai_generated',
				label: t('app.tools.col_ai_generated', 'AI Generated'),
				render: (value: boolean) => (value ? t('common.yes', 'Yes') : t('common.no', 'No'))
			},
			{
				key: 'is_active',
				label: t('app.tools.col_is_active', 'Active'),
				render: (value: boolean) => (value ? t('common.yes', 'Yes') : t('common.no', 'No'))
			},
			{
				key: 'updated_at',
				label: t('app.tools.col_updated_at', 'Updated'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// Server-side pagination — pass page/limit/q/status to the backend.
			// Ref: geo-aware-tool-pattern.md (admin list must filter+sort+paginate).
			const queryParts: string[] = [];
			queryParts.push(`page=${(params.page ?? 0) + 1}`);
			queryParts.push(`limit=${params.pageSize ?? 25}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				queryParts.push(`sort=${encodeURIComponent(params.sort)}`);
			}
			if (params.filter?.status) {
				queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			}
			if (params.filter?.dest) {
				queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			}
			const queryString = queryParts.join('&');

			const result = await visaRoutesAdminApi.listRaw(queryString);
			if (result.ok) {
				const data = (result.data.data as VisaRoute[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		create: async (body) => {
			const result = await visaRoutesAdminApi.create(body);
			if (result.ok) return { ok: true };
			return { ok: false, error: result.error };
		},
		update: async (id, body) => {
			const result = await visaRoutesAdminApi.update(id, body);
			if (result.ok) return { ok: true };
			return { ok: false, error: result.error };
		},
		remove: async (id) => {
			const result = await visaRoutesAdminApi.delete(id);
			if (result.ok) return { ok: true };
			return { ok: false, error: result.error };
		},
		viewSnippet: VisaRouteView
	});
</script>

<svelte:head>
	<title>{t('app.tools.visa_routes_page_title', 'Visa Routes Management — {site} Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	{#if canManage}
		<Button onclick={openGenerate}>
			<Sparkles size={16} />
			{t('app.tools.generate', 'Generate')}
		</Button>
	{/if}
</div>

<EntityPage {config} bind:controller={entityController} />

<!-- ── Generate modal ────────────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_visa_route', 'Generate Visa Route Draft')}>
	<div class="space-y-4">
		<div>
			<label for="gen-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_dest_country', 'Destination Country')}</label>
			<Autocomplete
				id="gen-dest"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtils.fetcher}
				initialFetcher={countryUtils.initialFetcher}
				selectedId={generateDestSpaceId || null}
				selectedLabel={generateDestSpaceId ? countryUtils.getSelectedLabel(generateDestSpaceId) : null}
				onSelect={(item: AutocompleteItem) => { generateDestSpaceId = item.id; }}
				onClear={() => { generateDestSpaceId = ''; }}
			/>
		</div>
		<div>
			<label for="gen-visa-cat" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_visa_category', 'Visa Category (optional)')}</label>
			<input
				id="gen-visa-cat"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateVisaCategory}
				placeholder={t('app.tools.visa_category_placeholder', 'e.g. work, student, family')}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={acting || !generateDestSpaceId}>
			{acting ? t('app.tools.generating', 'Generating...') : t('app.tools.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── Delete confirm modal ──────────────────────────────────────────────── -->
<Modal bind:open={showDeleteModal} title={t('app.tools.deactivate_visa_route', 'Deactivate Visa Route')}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t('app.tools.deactivate_confirm', 'Are you sure you want to deactivate this visa route? It will be hidden from public listings.')}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={deleting}>
			{deleting ? t('common.deleting', 'Deleting...') : t('common.deactivate', 'Deactivate')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet VisaRouteView({ row }: { row: VisaRoute })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || row.destination_space_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_visa_type', 'Visa Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visa_category || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_is_active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_ai_generated', 'AI Generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		{#if row.ai_model}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_ai_model', 'AI Model')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_model}</p>
			</div>
		{/if}
		{#if row.eligibility_summary}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_summary', 'Eligibility Summary')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.eligibility_summary}</p>
			</div>
		{/if}
		{#if row.typical_fee_usd != null}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_typical_fee', 'Typical Fee')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.typical_fee_usd}</p>
			</div>
		{/if}
		{#if row.path_to_pr}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_path_to_pr', 'Path to PR')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.path_to_pr}</p>
			</div>
		{/if}
		{#if row.renewal_terms}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_renewal_terms', 'Renewal Terms')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.renewal_terms}</p>
			</div>
		{/if}
		{#if row.restrictions}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_restrictions', 'Restrictions')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.restrictions}</p>
			</div>
		{/if}
		{#if row.gov_source_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_gov_source_url', 'Government Source')}</p>
				<a href={row.gov_source_url} target="_blank" rel="noopener noreferrer" class="mt-1 text-sm text-blue-500 hover:underline" style="color: var(--color-primary)">{row.gov_source_url}</a>
			</div>
		{/if}
		{#if row.hitl_reviewed_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_hitl_reviewed_by', 'HITL Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hitl_reviewed_by}</p>
			</div>
		{/if}
		{#if row.hitl_reviewed_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_hitl_reviewed_at', 'Reviewed At')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.hitl_reviewed_at).toLocaleString()}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
