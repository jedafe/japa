<script lang="ts">
	// /tools/language-test-hub — HITL review queue with Approve, AI Generate,
	// and Create actions. Fields map to the backend RequirementDTO contract.
	// Ref: §99 (Language Test Hub), ADR-107, ADR-088 (HITL review).
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { languageTestHubAdminApi , spacesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles, Plus } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type DestinationOption = { id: string; name: string; slug: string };
	type TestOption = { id: string; slug: string; name: string };

	type RequirementItem = {
		id: string;
		test_name: string;
		test_slug: string;
		destination_space_name: string;
		destination_space_id: string;
		visa_category: string;
		minimum_score: string;
		notes: string;
		source_url: string;
		status: string;
		ai_generated: boolean;
		ai_model?: string | null;
		ai_generated_at?: string | null;
		is_active: boolean;
		updated_at: string;
		created_at: string;
		hitl_reviewed_by: string;
	};

	const canManage = $derived(hasPermission('language_test_hub.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	
// Destination Spaces for the dest filter (geo-aware pattern §90-101).
type SpaceOption = { id: string; name: string };
let destOptions = $state<SpaceOption[]>([]);
onMount(async () => {
	try {
		const res = await spacesAdminApi.list('?level=country&limit=200');
		if (res.ok && Array.isArray(res.data)) {
			destOptions = (res.data as Array<{ id: string; name: string }>).map((s) => ({
				id: s.id, name: s.name
			}));
		}
	} catch { /* Non-fatal */ }
});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// Generate modal state
	let showGenerateModal = $state(false);
	let generateDestSpaceId = $state('');
	let generateVisaCategory = $state('');
	let generateTestSlug = $state('');
	let generating = $state(false);

	// Create modal state
	let showCreateModal = $state(false);
	let createDestSpaceId = $state('');
	let createVisaCategory = $state('');
	let createTestSlug = $state('');
	let createMinimumScore = $state('');
	let createNotes = $state('');
	let createSourceUrl = $state('');
	let creating = $state(false);

	// Geo-aware selector sources (ADR-088 geo-aware-tool-pattern).
	// Destinations and tests are fetched from the API so the admin never
	// types a raw UUID — the destination selector is the first input.
	let destinations = $state<DestinationOption[]>([]);
	let tests = $state<TestOption[]>([]);

	onMount(async () => {
		try {
			const destResult = await languageTestHubAdminApi.listDestinations();
			if (destResult.ok && Array.isArray(destResult.data)) {
				destinations = (destResult.data as DestinationOption[]).map((d) => ({
					id: d.id,
					name: d.name,
					slug: d.slug
				}));
			}
		} catch {
			destinations = [];
		}
		try {
			const testResult = await languageTestHubAdminApi.listTests();
			if (testResult.ok && Array.isArray(testResult.data)) {
				tests = (testResult.data as TestOption[]).map((tt) => ({
					id: tt.id,
					slug: tt.slug,
					name: tt.name
				}));
			}
		} catch {
			tests = [];
		}
	});

	async function approveRequirement(item: RequirementItem) {
		// ADR-088 HITL gate: AI-generated content cannot be approved without HITL review.
		if (item.ai_generated && !item.hitl_reviewed_by) {
			pageError = t('app.language_test_hub.hitl_required', 'Cannot approve AI-generated content without HITL review');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await languageTestHubAdminApi.approve(item.id);
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
		generateDestSpaceId = '';
		generateVisaCategory = '';
		generateTestSlug = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateDestSpaceId.trim() || !generateTestSlug.trim()) return;
		generating = true;
		pageError = '';
		try {
			const result = await languageTestHubAdminApi.generate({
				destination_space_id: generateDestSpaceId.trim(),
				visa_category: generateVisaCategory.trim() || undefined,
				test_slug: generateTestSlug.trim()
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

	function openCreate() {
		createDestSpaceId = '';
		createVisaCategory = '';
		createTestSlug = '';
		createMinimumScore = '';
		createNotes = '';
		createSourceUrl = '';
		showCreateModal = true;
	}

	async function confirmCreate() {
		if (!createDestSpaceId.trim() || !createTestSlug.trim() || !createMinimumScore.trim()) return;
		creating = true;
		pageError = '';
		try {
			const result = await languageTestHubAdminApi.create({
				destination_space_id: createDestSpaceId.trim(),
				visa_category: createVisaCategory.trim() || undefined,
				test_slug: createTestSlug.trim(),
				minimum_score: createMinimumScore.trim(),
				notes: createNotes.trim() || undefined,
				source_url: createSourceUrl.trim() || undefined
			});
			if (result.ok) {
				showCreateModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			creating = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: RequirementItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveRequirement,
				show: (row: RequirementItem) => row.status !== 'published'
			});
			acts.push({
				label: t('common.generate', 'Generate'),
				icon: Sparkles,
				onclick: openGenerate,
				show: () => true
			});
		}
		return acts;
	});

	// ── Country autocomplete helpers (filter destinations client-side) ─────
	// Sourced from the spaces country table (destinations, loaded from
	// /spaces?level=country via listDestinations) — never hardcoded (Rule 16).
	function fetchCountryName(q: string): Promise<{ id: string; label: string; sublabel?: string }[]> {
		const needle = q.toLowerCase().trim();
		const filtered = needle
			? destinations.filter((d) => d.name.toLowerCase().includes(needle))
			: destinations;
		return Promise.resolve(filtered.slice(0, 50).map((d) => ({ id: d.name, label: d.name })));
	}

	function fetchCountryNameInitial(): Promise<{ id: string; label: string; sublabel?: string }[]> {
		return Promise.resolve(destinations.slice(0, 8).map((d) => ({ id: d.name, label: d.name })));
	}

	function getCountryNameLabel(id: string): string {
		return destinations.find((d) => d.name === id)?.name ?? id;
	}

	const fields: FieldDef[] = [
		{ key: 'test_name', label: t('common.test_name', 'Test Name'), type: 'text' },
		{ key: 'destination_space_name', label: t('common.country', 'Country'), type: 'autocomplete', placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: fetchCountryName, initialFetcher: fetchCountryNameInitial, getSelectedLabel: getCountryNameLabel },
		{ key: 'visa_category', label: t('common.visa_category', 'Visa Category'), type: 'text' },
		{ key: 'minimum_score', label: t('common.minimum_score', 'Minimum Score'), type: 'text' },
		{ key: 'notes', label: t('common.notes', 'Notes'), type: 'textarea' },
		{ key: 'source_url', label: t('common.source_url', 'Source URL'), type: 'url' }
	];

		const filters = $derived<FilterDef[]>([
		{ key: 'dest', label: t('app.tools.col_destination', 'Destination'), options: destOptions.map((s) => ({ value: s.id, label: s.name })) }
	]);

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.tools.language_test_hub_heading', 'Language Test Hub Review Queue'),
		description: t('app.tools.language_test_hub_desc', 'Review and approve language test requirement entries'),
		readonly: false,
		createLabel: t('common.create_requirement', 'Create Requirement'),
		createPermission: 'language_test_hub.manage',
		editPermission: 'language_test_hub.manage',
		pageSize: 25,
		columns: [
			{ key: 'test_name', label: t('common.test_name', 'Test Name'), sortable: true },
			{ key: 'destination_space_name', label: t('common.country', 'Country'), sortable: true },
			{ key: 'visa_category', label: t('common.visa_category', 'Visa Category'), sortable: true },
			{
				key: 'minimum_score',
				label: t('common.minimum_score', 'Minimum Score'),
				sortable: true,
				render: (value: string) => value || '—'
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
			queryParts.push(`page=${params.page}`);
			queryParts.push(`limit=${params.pageSize}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				queryParts.push(`sort=${params.sort}`);
			}
			if (params.filter?.dest) {
				queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			}
			const result = await languageTestHubAdminApi.list(queryParts.join('&'));
			if (result.ok) {
				const body = result.data;
				const data = (body.data as RequirementItem[]) ?? [];
				const total = body.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / params.pageSize));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: RequirementView
	});
</script>

<svelte:head>
	<title>{t('app.tools.language_test_hub_page_title', 'Language Test Hub Review Queue — JapaConnect Admin')}</title>
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

{#if canManage}
	<div class="mt-4 flex gap-2">
		<Button onclick={openCreate} disabled={acting}>
			<Plus size={16} />
			{t('common.create_requirement', 'Create Requirement')}
		</Button>
	</div>
{/if}

<!-- ── Generate modal ────────────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_language_test', 'Generate Language Test Requirement')}>
	<div class="space-y-4">
		<div>
			<label for="gen-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('language_test.destination', 'Destination')}</label>
			<select
				id="gen-dest"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateDestSpaceId}
			>
				<option value="">{t('language_test.select_destination', 'Select destination...')}</option>
				{#each destinations as d (d.id)}
					<option value={d.id}>{d.name}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="gen-test" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.test', 'Test')}</label>
			<select
				id="gen-test"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateTestSlug}
			>
				<option value="">{t('common.select_test', 'Select test...')}</option>
				{#each tests as tt (tt.id)}
					<option value={tt.slug}>{tt.name}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="gen-visa" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.visa_category', 'Visa Category')}</label>
			<input
				id="gen-visa"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateVisaCategory}
				placeholder={t('common.visa_category_placeholder', 'e.g. Skilled Worker (optional)')}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !generateDestSpaceId.trim() || !generateTestSlug.trim()}>
			{generating ? t('common.generating', 'Generating...') : t('common.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── Create modal ──────────────────────────────────────────────────────── -->
<Modal bind:open={showCreateModal} title={t('common.create_requirement', 'Create Requirement')}>
	<div class="space-y-4">
		<div>
			<label for="cr-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('language_test.destination', 'Destination')} *</label>
			<select
				id="cr-dest"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={createDestSpaceId}
			>
				<option value="">{t('language_test.select_destination', 'Select destination...')}</option>
				{#each destinations as d (d.id)}
					<option value={d.id}>{d.name}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="cr-test" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.test', 'Test')} *</label>
			<select
				id="cr-test"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={createTestSlug}
			>
				<option value="">{t('common.select_test', 'Select test...')}</option>
				{#each tests as tt (tt.id)}
					<option value={tt.slug}>{tt.name}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="cr-score" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.minimum_score', 'Minimum Score')} *</label>
			<input
				id="cr-score"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={createMinimumScore}
				placeholder={t('common.minimum_score_ph', 'e.g. 7.0 overall')}
			/>
		</div>
		<div>
			<label for="cr-visa" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.visa_category', 'Visa Category')}</label>
			<input
				id="cr-visa"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={createVisaCategory}
				placeholder={t('common.visa_category_placeholder', 'e.g. Skilled Worker (optional)')}
			/>
		</div>
		<div>
			<label for="cr-notes" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.notes', 'Notes')}</label>
			<textarea
				id="cr-notes"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={createNotes}
				placeholder={t('common.notes_ph', 'Additional notes about this requirement...')}
			></textarea>
		</div>
		<div>
			<label for="cr-url" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.source_url', 'Source URL')}</label>
			<input
				id="cr-url"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={createSourceUrl}
				placeholder={t('common.source_url_ph', 'https://...')}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showCreateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmCreate} disabled={creating || !createDestSpaceId.trim() || !createTestSlug.trim() || !createMinimumScore.trim()}>
			{creating ? t('common.creating', 'Creating...') : t('common.create', 'Create')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet RequirementView({ row }: { row: RequirementItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.test_name', 'Test Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.test_name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.country', 'Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.visa_category', 'Visa Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visa_category || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.minimum_score', 'Minimum Score')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.minimum_score || '—'}</p>
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
		{#if row.notes}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.notes', 'Notes')}</p>
				<p class="mt-1 text-sm whitespace-pre-wrap" style="color: var(--color-foreground)">{row.notes}</p>
			</div>
		{/if}
		{#if row.source_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.source_url', 'Source URL')}</p>
				<a href={row.source_url} target="_blank" rel="noopener noreferrer" class="mt-1 text-sm text-[var(--color-primary)] hover:underline">{row.source_url}</a>
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
