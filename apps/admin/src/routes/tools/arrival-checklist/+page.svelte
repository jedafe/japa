<script lang="ts">
	// /tools/arrival-checklist — admin HITL review queue for AI-generated arrival checklists.
	// Ref: §92 (Arrival Checklist Generator), ADR-100, ADR-088 (Universal AI content generation).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { arrivalChecklistAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	// GuideDTO — matches the backend response shape (internal/arrivalchecklist/service.go).
	type ArrivalChecklistGuide = {
		id: string;
		destination_space_id: string;
		destination_space_name?: string;
		destination_space_code?: string;
		visa_category?: string;
		applicant_type: string;
		version: number;
		status: string;
		is_active: boolean;
		ai_generated: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		hitl_reviewed_by_name?: string;
		hitl_reviewed_at?: string;
		hitl_review_notes?: string;
		is_japabot_indexed: boolean;
		created_at: string;
		updated_at: string;
	};

	const canManage = $derived(hasPermission('arrival_checklist.manage'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	const countryUtilsName = useCountries('name');
	$effect(() => {
		countryUtils.load();
	});

	let entityController = $state<EntityPageController | undefined>(undefined);

	let arrivalChecklistStatusOptions = $state<CatalogOption[]>([
		{ value: 'draft', label: t('app.tools.status_draft', 'Draft') },
		{ value: 'ai_draft', label: t('app.tools.status_ai_draft', 'AI Draft') },
		{ value: 'pending', label: t('app.tools.status_pending', 'Pending') },
		{ value: 'published', label: t('app.tools.status_published', 'Published') },
		{ value: 'rejected', label: t('app.tools.status_rejected', 'Rejected') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('tools.arrival_checklist.status');
			if (res.ok && res.data?.length) arrivalChecklistStatusOptions = res.data;
		})();
	});

	let pageError = $state('');
	let acting = $state(false);

	// ── Generate modal state ────────────────────────────────────────────────
	let showGenerateModal = $state(false);
	let generateDestSpaceId = $state('');
	let generateVisaCategory = $state('');
	let generateApplicantType = $state('individual');

	async function openGenerate() {
		generateDestSpaceId = '';
		generateVisaCategory = '';
		generateApplicantType = 'individual';
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
			const result = await arrivalChecklistAdminApi.generate({
				destination_space_id: generateDestSpaceId,
				visa_category: generateVisaCategory || undefined,
				applicant_type: generateApplicantType
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

	async function approveItem(item: ArrivalChecklistGuide) {
		acting = true;
		pageError = '';
		try {
			const result = await arrivalChecklistAdminApi.approve(item.id);
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

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ArrivalChecklistGuide, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveItem,
				show: (row: ArrivalChecklistGuide) => row.status !== 'published'
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields = $derived.by<FieldDef[]>(() => {
		countryUtilsName.ready;
		return [
			{ key: 'destination_space_name', label: t('app.tools.col_dest_country', 'Destination'), type: 'autocomplete', help: t('app.tools.destination_space_name_hint', 'The destination space this is linked to.'), placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: countryUtilsName.fetcher, initialFetcher: countryUtilsName.initialFetcher, getSelectedLabel: countryUtilsName.getSelectedLabel },
			{ key: 'visa_category', label: t('app.tools.col_visa_category', 'Visa Category'), type: 'text', help: t('app.tools.visa_category_hint', 'The visa category for this record.') },
			{ key: 'applicant_type', label: t('app.tools.col_applicant_type', 'Applicant Type'), type: 'text', help: t('app.tools.applicant_type_hint', 'Who this applies to — e.g. student, worker.') }
		];
	});

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'status',
				label: t('app.tools.col_status', 'Status'),
				options: [
					{ value: '', label: t('common.all', 'All') },
					...arrivalChecklistStatusOptions
				]
			},
			{
				key: 'dest',
				label: t('app.tools.col_destination', 'Destination'),
				type: 'autocomplete',
			help: t('app.tools.dest_hint', 'The destination country this applies to.'),
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.arrival_checklist_heading', 'Arrival Checklist Review Queue'),
		description: t('app.tools.arrival_checklist_desc', 'Review and approve AI-generated arrival checklists'),
		readonly: true,
		pageSize: 25,
		filters,
		columns: [
			{
				key: 'destination_space_name',
				label: t('app.tools.col_dest_country', 'Destination'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'visa_category',
				label: t('app.tools.col_visa_category', 'Visa Category'),
				sortable: true,
				render: (value: string) => value || t('common.all', 'All')
			},
			{
				key: 'applicant_type',
				label: t('app.tools.col_applicant_type', 'Applicant Type'),
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
				key: 'updated_at',
				label: t('app.tools.col_updated_at', 'Updated'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// Server-side pagination — pass page/limit/q/sort/order to the backend.
			// EntityPage encodes sort as "column:dir" (e.g. "updated_at:desc").
			// Ref: geo-aware-tool-pattern.md (admin list must filter+sort+paginate).
			const queryParts: string[] = [];
			queryParts.push(`page=${(params.page ?? 0) + 1}`);
			queryParts.push(`limit=${params.pageSize ?? 25}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				const [sortCol, sortDir] = params.sort.split(':');
				if (sortCol) queryParts.push(`sort=${encodeURIComponent(sortCol)}`);
				if (sortDir) queryParts.push(`order=${encodeURIComponent(sortDir)}`);
			}
			if (params.filter?.dest) {
				queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			}
			if (params.filter?.status) {
				queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			}
			const queryString = queryParts.join('&');

			const result = await arrivalChecklistAdminApi.listRaw(queryString);
			if (result.ok) {
				const data = (result.data.data as ArrivalChecklistGuide[]) ?? [];
				const meta = result.data.meta ?? { total: 0, per_page: 25 };
				const total = meta.total ?? data.length;
				const totalPages = meta.total_pages ?? Math.max(1, Math.ceil(total / (meta.per_page ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ArrivalChecklistView
	});
</script>

<svelte:head>
	<title>{t('app.tools.arrival_checklist_page_title', 'Arrival Checklist Review Queue — {site} Admin')}</title>
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
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_arrival_checklist', 'Generate Arrival Checklist')}>
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
		<div>
			<label for="gen-app-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_applicant_type', 'Applicant Type')}</label>
			<AutocompleteSelect
				id="gen-app-type"
				bind:value={generateApplicantType}
				options={[{ value: 'individual', label: t('app.tools.applicant_individual', 'Individual') }, { value: 'couple', label: t('app.tools.applicant_couple', 'Couple') }, { value: 'family', label: t('app.tools.applicant_family', 'Family') }]}
				class="w-full"
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

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet ArrivalChecklistView({ row }: { row: ArrivalChecklistGuide })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || row.destination_space_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_visa_category', 'Visa Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visa_category || t('common.all', 'All')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_applicant_type', 'Applicant Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.applicant_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_version', 'Version')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.version}</p>
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
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_hitl_reviewed_by', 'HITL Reviewed By')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hitl_reviewed_by_name || '—'}</p>
		</div>
		{#if row.hitl_reviewed_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_hitl_reviewed_at', 'Reviewed At')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.hitl_reviewed_at).toLocaleString()}</p>
			</div>
		{/if}
		{#if row.hitl_review_notes}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_hitl_review_notes', 'Review Notes')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.hitl_review_notes}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
