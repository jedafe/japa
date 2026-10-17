<script lang="ts">
	// /tools/attestation-guide — HITL review queue with Approve, Generate, and
	// community update review actions. Aligned with backend PathwayDTO fields.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { attestationGuideAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type PathwayItem = {
		id: string;
		document_type: string;
		issuing_space_name?: string | null;
		destination_space_name?: string | null;
		apostille_required: boolean;
		status: string;
		is_active: boolean;
		ai_generated: boolean;
		ai_model?: string | null;
		hitl_reviewed_by?: string | null;
		hitl_reviewed_at?: string | null;
		community_last_verified?: string | null;
		pathway_steps: unknown;
		created_at: string;
		updated_at: string;
	};

	type CommunityUpdateItem = {
		id: string;
		pathway_id: string;
		user_id: string;
		step_index?: number | null;
		fee_estimate?: string | null;
		turnaround_days?: number | null;
		note?: string | null;
		status: string;
		reviewed_by?: string | null;
		created_at: string;
	};

	const canManage = $derived(hasPermission('attestation_guide.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	
	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	const ATTEST_TABS = ['pathways', 'updates'] as const;
	let activeTab = $state<'pathways' | 'updates'>(initTabParam('pathways', { valid: ATTEST_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: ATTEST_TABS, preserve: [] });

	// Generate modal state
	let showGenerateModal = $state(false);
	let generateDocType = $state('');
	let generateIssuingId = $state('');
	let generateDestId = $state('');
	let generatePrompt = $state('');
	let generating = $state(false);

	// Community updates list state
	let updates = $state<CommunityUpdateItem[]>([]);
	let loadingUpdates = $state(false);
	let updatesPage = $state(1);
	let updatesTotal = $state(0);
	let updatesPageSize = $state(25);
	let updateStatusFilter = $state('pending');
	const updatesTotalPages = $derived(Math.max(1, Math.ceil(updatesTotal / updatesPageSize)));

	const UPDATE_STATUS_OPTIONS = [
		{ value: 'pending', label: 'Pending' },
		{ value: 'approved', label: 'Approved' },
		{ value: 'rejected', label: 'Rejected' },
		{ value: 'all', label: 'All' }
	];
	const PAGE_SIZE_OPTIONS = [10, 25, 50, 100].map((n) => ({ value: String(n), label: `${n} / page` }));

	function optionFetcher(options: { value: string; label: string }[]) {
		return (q?: string): Promise<AutocompleteItem[]> => {
			const query = (q ?? '').toLowerCase();
			const filtered = query === '' ? options : options.filter((o) => o.label.toLowerCase().includes(query));
			return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
		};
	}

	function optionLabel(options: { value: string; label: string }[], value: string): string {
		return options.find((o) => o.value === value)?.label ?? value;
	}

	async function approvePathway(item: PathwayItem) {
		// ADR-088 HITL gate: AI-generated content cannot be approved without HITL review.
		if (item.ai_generated && !item.hitl_reviewed_by) {
			pageError = t('app.attestation_guide.hitl_required', 'Cannot approve AI-generated content without HITL review');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await attestationGuideAdminApi.approve(item.id);
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

	function openGenerate() {
		generateDocType = '';
		generateIssuingId = '';
		generateDestId = '';
		generatePrompt = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateDocType.trim() || !generateIssuingId.trim() || !generateDestId.trim()) return;
		generating = true;
		pageError = '';
		try {
			const result = await attestationGuideAdminApi.generate({
				document_type: generateDocType.trim(),
				issuing_space_id: generateIssuingId.trim(),
				destination_space_id: generateDestId.trim(),
				prompt: generatePrompt.trim()
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
			generating = false;
		}
	}

	async function loadUpdates(page = 1) {
		loadingUpdates = true;
		updatesPage = page;
		try {
			// api helpers prepend '?' themselves — pass a bare query string.
			const statusParam = updateStatusFilter === 'all' ? '' : `status=${updateStatusFilter}&`;
			const result = await attestationGuideAdminApi.listUpdates(
				`${statusParam}page_offset=${(page - 1) * updatesPageSize}&page_size=${updatesPageSize}`
			);
			if (result.ok) {
				const resp = result.data as { data?: CommunityUpdateItem[]; pagination?: { total?: number } } | undefined;
				updates = resp?.data ?? [];
				updatesTotal = resp?.pagination?.total ?? updates.length;
			} else {
				updates = [];
				updatesTotal = 0;
			}
		} catch {
			updates = [];
			updatesTotal = 0;
		} finally {
			loadingUpdates = false;
		}
	}

	async function reviewUpdate(id: string, approve: boolean) {
		acting = true;
		pageError = '';
		try {
			const result = await attestationGuideAdminApi.reviewUpdate(id, approve);
			if (result.ok) {
				// Refetch the current page — a local filter would leave the page
				// short one row and stale vs. the server-side total.
				await loadUpdates(updatesPage);
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	function onTabChange(tab: 'pathways' | 'updates') {
		activeTab = tab;
	}

	// Lazy-load updates on first activation — covers clicks, ?tab=updates
	// deep-links, and back/forward navigation.
	let updatesLoaded = $state(false);
	$effect(() => {
		if (activeTab === 'updates' && !updatesLoaded) {
			updatesLoaded = true;
			void loadUpdates(1);
		}
	});

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: PathwayItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approvePathway,
				show: (row: PathwayItem) => row.status === 'ai_draft' || row.status === 'pending'
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields = $derived.by<FieldDef[]>(() => {
		countryUtils.ready;
		return [
			{ key: 'document_type', label: t('common.doc_type', 'Document Type'), type: 'text' },
			{ key: 'issuing_space_id', label: t('common.issuing_country', 'Issuing Country'), type: 'autocomplete', placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: countryUtils.fetcher, initialFetcher: countryUtils.initialFetcher, getSelectedLabel: countryUtils.getSelectedLabel },
			{ key: 'destination_space_id', label: t('common.dest_country', 'Destination Country'), type: 'autocomplete', placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: countryUtils.fetcher, initialFetcher: countryUtils.initialFetcher, getSelectedLabel: countryUtils.getSelectedLabel },
			{ key: 'apostille_required', label: t('common.apostille', 'Apostille Required'), type: 'checkbox' },
			{ key: 'pathway_steps', label: t('common.pathway_steps', 'Pathway Steps (JSON)'), type: 'textarea' }
		];
	});

		const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'dest',
				label: t('app.tools.col_destination', 'Destination'),
				type: 'autocomplete',
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			},
			{
				key: 'status',
				label: t('common.status', 'Status'),
				options: [
					{ value: 'draft', label: t('common.status_draft', 'Draft') },
					{ value: 'ai_draft', label: t('common.status_ai_draft', 'AI Draft') },
					{ value: 'pending', label: t('common.status_pending', 'Pending') },
					{ value: 'published', label: t('common.status_published', 'Published') },
					{ value: 'rejected', label: t('common.status_rejected', 'Rejected') }
				]
			},
			{
				// §97 document_type enum — mirrored from the backend's canonical values.
				key: 'doc_type',
				label: t('common.doc_type', 'Document Type'),
				options: [
					{ value: 'birth_certificate', label: t('app.tools.doc_birth_certificate', 'Birth certificate') },
					{ value: 'degree', label: t('app.tools.doc_degree', 'Degree') },
					{ value: 'marriage', label: t('app.tools.doc_marriage', 'Marriage certificate') },
					{ value: 'police_clearance', label: t('app.tools.doc_police_clearance', 'Police clearance') },
					{ value: 'professional_licence', label: t('app.tools.doc_professional_licence', 'Professional licence') },
					{ value: 'other', label: t('common.other', 'Other') }
				]
			},
			{
				key: 'active',
				label: t('common.active', 'Active'),
				options: [
					{ value: 'true', label: t('common.yes', 'Yes') },
					{ value: 'false', label: t('common.no', 'No') }
				]
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.tools.attestation_guide_heading', 'Attestation Guide Review Queue'),
		description: t('app.tools.attestation_guide_desc', 'Review and approve attestation guide entries'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'document_type', label: t('common.doc_type', 'Document Type'), sortable: true },
			{
				key: 'issuing_space_name',
				label: t('common.issuing_country', 'Issuing'),
				sortable: true,
				render: (value: string | null) => value || '—'
			},
			{
				key: 'destination_space_name',
				label: t('common.dest_country', 'Destination'),
				sortable: true,
				render: (value: string | null) => value || '—'
			},
			{
				key: 'status',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'ai_generated',
				label: t('common.ai_generated', 'AI Generated'),
				render: (value: boolean) => (value ? t('common.yes', 'Yes') : t('common.no', 'No'))
			},
			{
				key: 'updated_at',
				label: t('common.updated', 'Updated'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const queryParts: string[] = [];
			// Convention: page_offset is a row offset (api-response-format.md).
			queryParts.push(`page_offset=${params.page * (params.pageSize || 25)}`);
			if (params.pageSize) queryParts.push(`page_size=${params.pageSize}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			// EntityPage passes sort as "key:dir" (e.g. "document_type:asc").
			if (params.sort) {
				const [sortKey, sortDir] = params.sort.split(':');
				if (sortKey) queryParts.push(`sort=${encodeURIComponent(sortKey)}`);
				if (sortDir) queryParts.push(`dir=${encodeURIComponent(sortDir)}`);
			}
			if (params.filter?.dest) queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			if (params.filter?.status)
				queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			if (params.filter?.doc_type)
				queryParts.push(`doc_type=${encodeURIComponent(params.filter.doc_type)}`);
			if (params.filter?.active)
				queryParts.push(`active=${encodeURIComponent(params.filter.active)}`);
			// api helpers prepend '?' themselves — pass a bare query string.
			const result = await attestationGuideAdminApi.list(queryParts.join('&'));
			if (result.ok) {
				const resp = result.data as { data: PathwayItem[]; pagination?: { total?: number } };
				const data = resp.data ?? [];
				const total = resp.pagination?.total ?? data.length;
				const totalPages = Math.ceil(total / (params.pageSize || 25));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: PathwayView
	});
</script>

<svelte:head>
	<title>{t('app.tools.attestation_guide_page_title', 'Attestation Guide Review Queue — {site} Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<!-- Tab switcher -->
<div class="mb-4 flex gap-2 border-b" style="border-color: var(--color-border)">
	<button
		class="px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'pathways'
			? 'color: var(--color-foreground); border-bottom: 2px solid var(--color-primary)'
			: 'color: var(--color-muted); border-bottom: 2px solid transparent'}
		onclick={() => onTabChange('pathways')}
	>
		{t('app.tools.attestation_pathways_tab', 'Pathways')}
	</button>
	<button
		class="px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'updates'
			? 'color: var(--color-foreground); border-bottom: 2px solid var(--color-primary)'
			: 'color: var(--color-muted); border-bottom: 2px solid transparent'}
		onclick={() => onTabChange('updates')}
	>
		{t('app.tools.attestation_updates_tab', 'Community Updates')}
	</button>
</div>

{#if activeTab === 'pathways'}
	{#if canManage}
		<div class="mb-4 flex justify-end">
			<Button onclick={openGenerate}>
				<Sparkles size={16} class="mr-1.5" />
				{t('common.generate', 'Generate')}
			</Button>
		</div>
	{/if}
	<EntityPage {config} bind:controller={entityController} />
{:else}
	<div class="space-y-3">
		<!-- Status filter + page size — server-side filters on /admin/attestation/updates -->
		<div class="mb-2 flex flex-wrap items-center gap-4">
			<div class="flex items-center gap-2">
				<span class="text-sm font-medium" style="color: var(--color-foreground)">
					{t('common.status', 'Status')}:
				</span>
				<Autocomplete
					id="update-status-filter"
					placeholder={t('common.status', 'Status')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={optionFetcher(UPDATE_STATUS_OPTIONS)}
					initialFetcher={optionFetcher(UPDATE_STATUS_OPTIONS)}
					selectedId={updateStatusFilter}
					selectedLabel={optionLabel(UPDATE_STATUS_OPTIONS, updateStatusFilter)}
					onSelect={(item) => { updateStatusFilter = item.id; loadUpdates(1); }}
					onClear={() => { updateStatusFilter = 'all'; loadUpdates(1); }}
				/>
			</div>
			<div class="flex items-center gap-2">
				<span class="text-sm font-medium" style="color: var(--color-foreground)">
					{t('common.page_size', 'Page size')}:
				</span>
				<Autocomplete
					id="update-page-size"
					placeholder={t('common.page_size', 'Page size')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={optionFetcher(PAGE_SIZE_OPTIONS)}
					initialFetcher={optionFetcher(PAGE_SIZE_OPTIONS)}
					selectedId={String(updatesPageSize)}
					selectedLabel={optionLabel(PAGE_SIZE_OPTIONS, String(updatesPageSize))}
					onSelect={(item) => { updatesPageSize = parseInt(item.id, 10) || 25; loadUpdates(1); }}
				/>
			</div>
		</div>
		{#if loadingUpdates}
			<p class="text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
		{:else if updates.length === 0}
			<p class="text-sm" style="color: var(--color-muted)">
				{updateStatusFilter === 'pending'
					? t('app.tools.no_pending_updates', 'No pending community updates.')
					: updateStatusFilter === 'all'
						? t('app.tools.no_updates', 'No community updates.')
						: t('app.tools.no_status_updates', 'No {status} community updates.').replace('{status}', optionLabel(UPDATE_STATUS_OPTIONS, updateStatusFilter).toLowerCase())}
			</p>
		{:else}
			{#each updates as update (update.id)}
				<div
					class="rounded-lg border p-4"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-center justify-between">
						<div>
							<p class="text-sm font-medium" style="color: var(--color-foreground)">
								{t('common.update', 'Update')} #{update.step_index ?? '—'}
								<span class="ml-2 text-xs font-normal" style="color: var(--color-muted)">({update.status})</span>
							</p>
							<p class="text-xs" style="color: var(--color-muted)">
								{update.fee_estimate ?? '—'} · {update.turnaround_days ?? '—'} days
							</p>
						</div>
						{#if update.status === 'pending'}
							<div class="flex gap-2">
								<Button
									variant="ghost"
									onclick={() => reviewUpdate(update.id, true)}
									disabled={acting}
								>
									{t('common.approve', 'Approve')}
								</Button>
								<Button
									variant="ghost"
									onclick={() => reviewUpdate(update.id, false)}
									disabled={acting}
								>
									{t('common.reject', 'Reject')}
								</Button>
							</div>
						{/if}
					</div>
					{#if update.note}
						<p class="mt-2 text-sm" style="color: var(--color-foreground)">{update.note}</p>
					{/if}
				</div>
			{/each}
		{/if}
		<!-- Pagination controls (BUG-005) -->
		{#if updatesTotalPages > 1}
			<div class="mt-4 flex items-center justify-between">
				<p class="text-xs" style="color: var(--color-muted)">
					{t('common.page', 'Page')} {updatesPage} / {updatesTotalPages}
				</p>
				<div class="flex gap-2">
					<Button
						variant="ghost"
						onclick={() => loadUpdates(updatesPage - 1)}
						disabled={updatesPage <= 1 || loadingUpdates}
					>
						{t('common.prev', 'Previous')}
					</Button>
					<Button
						variant="ghost"
						onclick={() => loadUpdates(updatesPage + 1)}
						disabled={updatesPage >= updatesTotalPages || loadingUpdates}
					>
						{t('common.next', 'Next')}
					</Button>
				</div>
			</div>
		{/if}
	</div>
{/if}

<!-- ── Generate modal ────────────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_attestation_guide', 'Generate Attestation Guide')}>
	<div class="space-y-4">
		<div>
			<label for="gen-doc-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.doc_type', 'Document Type')}</label>
			<input
				id="gen-doc-type"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateDocType}
				placeholder={t('app.tools.doc_type_placeholder', 'e.g. birth_certificate')}
			/>
		</div>
		<div>
			<label for="gen-issuing" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.issuing_country', 'Issuing Country')}</label>
			<Autocomplete
				id="gen-issuing"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtils.fetcher}
				initialFetcher={countryUtils.initialFetcher}
				selectedId={generateIssuingId || null}
				selectedLabel={generateIssuingId ? countryUtils.getSelectedLabel(generateIssuingId) : null}
				onSelect={(item) => { generateIssuingId = item.id; }}
				onClear={() => { generateIssuingId = ''; }}
			/>
		</div>
		<div>
			<label for="gen-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.dest_country', 'Destination Country')}</label>
			<Autocomplete
				id="gen-dest"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtils.fetcher}
				initialFetcher={countryUtils.initialFetcher}
				selectedId={generateDestId || null}
				selectedLabel={generateDestId ? countryUtils.getSelectedLabel(generateDestId) : null}
				onSelect={(item) => { generateDestId = item.id; }}
				onClear={() => { generateDestId = ''; }}
			/>
		</div>
		<div>
			<label for="gen-prompt" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.generate_prompt', 'Prompt (optional)')}</label>
			<textarea
				id="gen-prompt"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generatePrompt}
				placeholder={t('app.tools.generate_prompt_placeholder', 'Describe the attestation pathway for the AI to draft')}
				rows="3"
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !generateDocType.trim() || !generateIssuingId.trim() || !generateDestId.trim()}>
			{generating ? t('common.generating', 'Generating...') : t('common.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet PathwayView({ row }: { row: PathwayItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.doc_type', 'Document Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.document_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.issuing_country', 'Issuing')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.issuing_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.dest_country', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.apostille', 'Apostille Required')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.apostille_required ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.ai_generated', 'AI Generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.updated', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
		{#if row.pathway_steps && Array.isArray(row.pathway_steps) && row.pathway_steps.length > 0}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.pathway_steps', 'Pathway Steps')}</p>
				<ol class="mt-1 list-decimal list-inside text-sm space-y-1" style="color: var(--color-foreground)">
					{#each row.pathway_steps as step (step)}
						<li>{JSON.stringify(step)}</li>
					{/each}
				</ol>
			</div>
		{/if}
		{#if row.ai_generated}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.ai_generated', 'AI Generated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t('common.yes', 'Yes')} ({row.ai_model ?? '—'})</p>
			</div>
		{/if}
		{#if row.hitl_reviewed_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.hitl_reviewed_by', 'HITL Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hitl_reviewed_by}</p>
			</div>
		{/if}
	</div>
{/snippet}
