<script lang="ts">
	// /tools/housing-guide — admin HITL review queue for AI-generated housing guides.
	// Ref: §96 (Housing Guide), ADR-088 (universal AI content generation).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { housingGuideAdminApi } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	// GuideDTO fields from internal/housingguide/service.go (GuideDTO).
	type HousingGuideItem = {
		id: string;
		destination_space_id: string;
		destination_space_name?: string | null;
		city_space_name?: string | null;
		accommodation_type?: string | null;
		status: string;
		is_active: boolean;
		ai_generated: boolean;
		ai_model?: string | null;
		hitl_reviewed_by?: string | null;
		ai_generated_at?: string | null;
		last_verified_at?: string | null;
		hitl_reviewed_at?: string | null;
		reviewer_name?: string | null;
		created_at: string;
		updated_at: string;
	};

	const canManage = $derived(hasPermission('housing_guide.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	
	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// ── Generate modal state ────────────────────────────────────────────────
	let showGenerateModal = $state(false);
	let generateDestSpaceId = $state('');
	let generateAccommodationType = $state('');

	const accommodationTypes = [
		{ value: 'private_rental', label: 'Private rental' },
		{ value: 'flatshare', label: 'Flatshare' },
		{ value: 'student', label: 'Student' },
		{ value: 'employer_provided', label: 'Employer-provided' },
		{ value: 'all', label: 'All' }
	];

	function openGenerate() {
		generateDestSpaceId = '';
		generateAccommodationType = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateDestSpaceId) {
			pageError = t('app.tools.dest_country_placeholder', 'Select a destination country');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await housingGuideAdminApi.generate({
				destination_space_id: generateDestSpaceId,
				accommodation_type: generateAccommodationType
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

	async function approveItem(item: HousingGuideItem) {
		// ADR-088 HITL gate: AI-generated content cannot be approved without HITL review.
		if (item.ai_generated && !item.hitl_reviewed_by) {
			pageError = t('app.housing_guide.hitl_required', 'Cannot approve AI-generated content without HITL review');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await housingGuideAdminApi.approve(item.id);
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

	async function rejectItem(item: HousingGuideItem) {
		acting = true;
		pageError = '';
		try {
			const result = await housingGuideAdminApi.reject(item.id);
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: HousingGuideItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveItem,
				show: (row: HousingGuideItem) => row.status !== 'published'
			});
			acts.push({
				label: t('common.reject', 'Reject'),
				icon: XCircle,
				onclick: rejectItem,
				variant: 'destructive',
				show: (row: HousingGuideItem) => row.status !== 'published' && row.status !== 'rejected'
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields = $derived.by<FieldDef[]>(() => {
		countryUtils.ready;
		return [
			{ key: 'destination_space_id', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'autocomplete', placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: countryUtils.fetcher, initialFetcher: countryUtils.initialFetcher, getSelectedLabel: countryUtils.getSelectedLabel },
			{ key: 'accommodation_type', label: t('app.tools.col_accommodation_type', 'Accommodation Type'), type: 'text' }
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
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.tools.housing_guide_heading', 'Housing Guide Review Queue'),
		description: t('app.tools.housing_guide_desc', 'Review and approve AI-generated housing guides'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'destination_space_name',
				label: t('app.tools.col_dest_country', 'Destination'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{ key: 'city_space_name', label: t('app.tools.col_city', 'City'), render: (value: string) => value || '—' },
			{
				key: 'accommodation_type',
				label: t('app.tools.col_accommodation_type', 'Type'),
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
				key: 'last_verified_at',
				label: t('app.tools.col_last_verified', 'Last verified'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
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
			// Forward all DataTable state to the backend so server-side
			// pagination/sort/search/filter works (geo-aware-tool-pattern.md
			// "Tool-Specific Admin List Pattern": filter+sort+paginate, no
			// infinite scroll). The backend HandleAdminList supports
			// ?page=&limit=&status=&type=&dest=&city=&q=&active=.
			const qs = new URLSearchParams();
			qs.set('page', String(params.page + 1)); // backend is 1-based
			qs.set('limit', String(params.pageSize));
			if (params.q) qs.set('q', params.q);
			if (params.sort) {
				qs.set('sort', params.sort);
				// The backend orders by created_at/updated_at desc; the
				// direction is implicit. We still forward the key.
			}
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			if (params.filter?.status) qs.set('status', params.filter.status);
			if (params.filter?.type) qs.set('type', params.filter.type);
			const result = await housingGuideAdminApi.list(`?${qs.toString()}`);
			if (result.ok) {
				const data = (result.data.data as HousingGuideItem[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / params.pageSize));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: HousingGuideView
	});
</script>

<svelte:head>
	<title>{t('app.tools.housing_guide_page_title', 'Housing Guide Review Queue — {site} Admin')}</title>
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
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_housing_guide', 'Generate Housing Guide')}>
	<div class="space-y-4">
		<div>
			<label for="gen-dest-country" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_dest_country', 'Destination Country')}</label>
			<Autocomplete
				id="gen-dest-country"
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
			<label for="gen-accommodation-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_accommodation_type', 'Accommodation Type')}</label>
			<AutocompleteSelect
				id="gen-accommodation-type"
				bind:value={generateAccommodationType}
				options={[{ value: '', label: t('app.tools.all_types', 'All types') }, ...accommodationTypes]}
				class="w-full"
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={acting}>
			{acting ? t('app.tools.generating', 'Generating...') : t('app.tools.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View modal body ──────────────────────────────────────────────── -->
{#snippet HousingGuideView({ row }: { row: HousingGuideItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_city', 'City')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.city_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_accommodation_type', 'Accommodation Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.accommodation_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_ai_generated', 'AI Generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_last_verified', 'Last verified')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.last_verified_at ? new Date(row.last_verified_at).toLocaleDateString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
		{#if row.reviewer_name}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_reviewer', 'Reviewer')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.reviewer_name}</p>
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
