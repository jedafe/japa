<script lang="ts">
	// /tools/tax-calculator — HITL review queue with Approve Guide + AI Generate Guide actions.
	// BUG-001/002/003/011 fix: response shape, field names, generate fields, pagination.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { taxCalculatorAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { useCountries } from '$lib/utils/countries.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type GuideItem = {
		id: string;
		employment_status: string;
		origin_space_name: string | null;
		destination_space_name: string | null;
		residency_trigger: string | null;
		worldwide_income_taxed: boolean | null;
		first_year_obligations: string | null;
		status: string;
		is_active: boolean;
		ai_generated: boolean;
		ai_model: string | null;
		hitl_reviewed_by: string | null;
		hitl_reviewed_at: string | null;
		updated_at: string;
		created_at: string;
	};

	const canManage = $derived(hasPermission('tax_calculator.manage'));
	const canReview = $derived(hasPermission('tax_content.review'));

	// ── State ───────────────────────────────────────────────────────────────
	
	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// Generate modal state
	let showGenerateModal = $state(false);
	let generateOrigin = $state('');
	let generateDest = $state('');
	let generateEmployment = $state('');
	let generatePrompt = $state('');
	let generating = $state(false);

	async function approveGuide(item: GuideItem) {
		// ADR-088 HITL gate: AI-generated content cannot be approved without HITL review.
		if (item.ai_generated && !item.hitl_reviewed_by) {
			pageError = t('app.tax_calculator.hitl_required', 'Cannot approve AI-generated content without HITL review');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await taxCalculatorAdminApi.approveGuide(item.id);
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
		generateOrigin = '';
		generateDest = '';
		generateEmployment = '';
		generatePrompt = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateOrigin.trim() || !generateDest.trim() || !generateEmployment.trim()) return;
		generating = true;
		pageError = '';
		try {
			const result = await taxCalculatorAdminApi.generateGuide({
				origin_space_id: generateOrigin.trim(),
				destination_space_id: generateDest.trim(),
				employment_status: generateEmployment.trim(),
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

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: GuideItem, ctx) => ctx?.openView(_row) }
		];
		if (canReview) {
			acts.push({
				label: t('common.approve_guide', 'Approve Guide'),
				icon: CheckCircle,
				onclick: approveGuide,
				show: (row: GuideItem) => row.status !== 'published'
			});
		}
		if (canManage) {
			acts.push({
				label: t('common.generate_guide', 'Generate Guide'),
				icon: Sparkles,
				onclick: openGenerate,
				show: () => true
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'employment_status', label: t('common.employment_status', 'Employment Status'), type: 'text' },
		{ key: 'residency_trigger', label: t('tax_calculator.residency_trigger', 'Residency Trigger'), type: 'textarea' },
		{ key: 'first_year_obligations', label: t('tax_calculator.first_year_obligations', 'First Year Obligations'), type: 'textarea' }
	];

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
		title: t('app.tools.tax_calculator_heading', 'Tax Calculator Guide Review Queue'),
		description: t('app.tools.tax_calculator_desc', 'Review and approve tax calculator guide entries'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'employment_status',
				label: t('common.employment_status', 'Employment Status'),
				sortable: true
			},
			{
				key: 'origin_space_name',
				label: t('tax_calculator.origin', 'Origin'),
				sortable: true,
				render: (value: string | null) => value ?? '—'
			},
			{
				key: 'destination_space_name',
				label: t('tax_calculator.destination', 'Destination'),
				sortable: true,
				render: (value: string | null) => value ?? '—'
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
			// BUG-011: pass pagination/sort/filter params to the backend.
			const queryParts: string[] = [];
			queryParts.push(`page=${(params.page ?? 0) + 1}`);
			queryParts.push(`limit=${params.pageSize ?? 25}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				queryParts.push(`sort=${encodeURIComponent(params.sort)}`);
				queryParts.push(`order=desc`);
			}
			// Pass filter values as individual query params.
			if (params.filter) {
				for (const [key, value] of Object.entries(params.filter)) {
					if (value != null && value !== '') {
						queryParts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`);
					}
				}
			}
			const query = queryParts.join('&');
			const result = await taxCalculatorAdminApi.list(query);
			if (result.ok && result.data) {
				// Backend returns { data: { guides, treaties }, meta }
				// (geo-aware admin list pattern). Extract guides array
				// and pagination total from meta.
				const guides = (result.data.data?.guides as GuideItem[]) ?? [];
				const total = result.data.meta?.total ?? guides.length;
				const totalPages = result.data.meta?.total_pages ?? Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data: guides, total, totalPages };
			}
			const errMsg = !result.ok ? result.error : t('common.error', 'An error occurred');
			throw new Error(errMsg);
		},
		viewSnippet: GuideView
	});
</script>

<svelte:head>
	<title>{t('app.tools.tax_calculator_page_title', 'Tax Calculator Guide Review Queue — {site} Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Generate modal ────────────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_tax_guide', 'Generate Tax Calculator Guide')}>
	<div class="space-y-4">
		<!-- Destination selector (first — geo-aware pattern) -->
		<div>
			<label for="gen-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('tax_calculator.destination', 'Destination')}</label>
			<Autocomplete
				id="gen-dest"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtils.fetcher}
				initialFetcher={countryUtils.initialFetcher}
				selectedId={generateDest || null}
				selectedLabel={generateDest ? countryUtils.getSelectedLabel(generateDest) : null}
				onSelect={(item: AutocompleteItem) => { generateDest = item.id; }}
				onClear={() => { generateDest = ''; }}
			/>
		</div>
		<!-- Origin selector (secondary — geo-aware pattern) -->
		<div>
			<label for="gen-origin" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('tax_calculator.origin', 'Origin')}</label>
			<Autocomplete
				id="gen-origin"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtils.fetcher}
				initialFetcher={countryUtils.initialFetcher}
				selectedId={generateOrigin || null}
				selectedLabel={generateOrigin ? countryUtils.getSelectedLabel(generateOrigin) : null}
				onSelect={(item: AutocompleteItem) => { generateOrigin = item.id; }}
				onClear={() => { generateOrigin = ''; }}
			/>
		</div>
		<div>
			<label for="gen-emp" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.employment_status', 'Employment Status')}</label>
			<AutocompleteSelect
				id="gen-emp"
				bind:value={generateEmployment}
				options={[{ value: '', label: t('common.select', 'Select…') }, { value: 'employed', label: t('tax_calculator.employed', 'Employed') }, { value: 'self_employed', label: t('tax_calculator.self_employed', 'Self-employed') }, { value: 'contractor', label: t('tax_calculator.contractor', 'Contractor') }, { value: 'unemployed', label: t('tax_calculator.unemployed', 'Unemployed') }]}
				class="w-full"
			/>
		</div>
		<div>
			<label for="gen-prompt" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.topic_prompt', 'Prompt')}</label>
			<textarea
				id="gen-prompt"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={generatePrompt}
				placeholder={t('app.tools.topic_placeholder', 'Describe the tax guide to generate...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !generateOrigin.trim() || !generateDest.trim() || !generateEmployment.trim()}>
			{generating ? t('common.generating', 'Generating...') : t('common.generate_guide', 'Generate Guide')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet GuideView({ row }: { row: GuideItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.employment_status', 'Employment Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.employment_status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tax_calculator.origin', 'Origin')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.origin_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tax_calculator.destination', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.ai_generated', 'AI Generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		{#if row.residency_trigger}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tax_calculator.residency_trigger', 'Residency Trigger')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.residency_trigger}</p>
			</div>
		{/if}
		{#if row.first_year_obligations}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tax_calculator.first_year_obligations', 'First Year Obligations')}</p>
				<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.first_year_obligations}</p>
			</div>
		{/if}
		{#if row.worldwide_income_taxed !== null && row.worldwide_income_taxed !== undefined}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tax_calculator.worldwide_income_taxed', 'Worldwide Income Taxed')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.worldwide_income_taxed ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.updated', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
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
