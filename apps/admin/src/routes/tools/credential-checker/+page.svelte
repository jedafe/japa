<script lang="ts">
	// /tools/credential-checker — admin HITL review queue for AI-generated credential checker guides.
	// Fields map to the backend PathwayDTO contract (§94, ADR-102, ADR-088).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import Autocomplete from '#lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/ui/Autocomplete.svelte';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { Eye, CheckCircle, Sparkles, Pencil, XCircle } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '#lib/components/ui/data-table/index.js';
	import { credentialCheckerAdminApi, apiFetch, adminCatalogApi, type CatalogOption } from '#lib/api.js';
	import { useCountries } from '#lib/utils/countries.svelte.js';
	import AutocompleteSelect from '#lib/components/ui/AutocompleteSelect.svelte';

	type CredentialCheckerItem = {
		id: string;
		profession_id?: string;
		profession_name: string;
		profession_slug: string;
		destination_space_id?: string;
		destination_space_name: string;
		issuing_space_id?: string;
		issuing_space_name: string;
		regulatory_body: string;
		regulatory_body_url?: string;
		pathway_type: string;
		timeline_estimate?: string;
		cost_estimate_usd?: string;
		pathway_description: string;
		official_application_url?: string;
		status: string;
		ai_generated: boolean;
		ai_model?: string | null;
		hitl_reviewed_by?: string | null;
		hitl_reviewed_by_name?: string | null;
		ai_generated_at?: string | null;
		is_active: boolean;
		updated_at?: string;
		created_at?: string;
	};

	type CommunityNote = {
		id: string;
		pathway_id: string;
		user_name?: string;
		note: string;
		status: string;
		created_at?: string;
		profession_name?: string;
		destination_space_name?: string;
	};

	type ProfessionOption = { id: string; slug: string; name: string; category?: string };

	const canManage = $derived(hasPermission('credential_checker.manage'));

	// ── Country selectors (250 primary geo space countries) ──────────────────
	// Filter, generate modal, and the edit form all use geo space *id*.
	const countryUtilsById = useCountries('id');
	$effect(() => {
		countryUtilsById.load();
	});

	// ── State ───────────────────────────────────────────────────────────────

	let entityController = $state<EntityPageController | undefined>(undefined);

	let credentialCheckerStatusOptions = $state<CatalogOption[]>([
		{ value: 'ai_draft', label: t('common.ai_draft', 'AI Draft') },
		{ value: 'pending', label: t('common.pending', 'Pending') },
		{ value: 'published', label: t('common.published', 'Published') },
		{ value: 'rejected', label: t('common.rejected', 'Rejected') },
		{ value: 'draft', label: t('common.draft', 'Draft') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('tools.credential_checker.status');
			if (res.ok && res.data?.length) credentialCheckerStatusOptions = res.data;
		})();
	});

	let pageError = $state('');
	let acting = $state(false);

	// ── Community note moderation queue ─────────────────────────────────
	let pendingNotes = $state<CommunityNote[]>([]);
	let notesLoaded = $state(false);
	let moderatingNote = $state<string | null>(null);

	async function loadPendingNotes() {
		const res = await credentialCheckerAdminApi.listNotes('status=pending&limit=50');
		if (res.ok && res.data) {
			pendingNotes = (res.data.data as CommunityNote[]) ?? [];
			notesLoaded = true;
		}
	}

	async function moderateNote(note: CommunityNote, status: 'published' | 'rejected') {
		moderatingNote = note.id;
		pageError = '';
		const res = await credentialCheckerAdminApi.moderateNote(note.id, status);
		moderatingNote = null;
		if (res.ok) {
			pendingNotes = pendingNotes.filter((n) => n.id !== note.id);
		} else {
			pageError = res.error ?? t('common.error', 'An error occurred');
		}
	}

	$effect(() => {
		if (canManage && !notesLoaded) loadPendingNotes();
	});

	// ── Generate modal state ────────────────────────────────────────────────
	let showGenerateModal = $state(false);
	let generateProfessionId = $state('');
	let generateIssuingSpaceId = $state('');
	let generateDestSpaceId = $state('');
	let generating = $state(false);

	// ── Reference data for the generate modal (professions) ─────────────────
	let professions = $state<ProfessionOption[]>([]);

	async function loadReferenceData() {
		try {
			const profRes = await apiFetch<ProfessionOption[]>('/credentials/professions');
			if (profRes.data) professions = profRes.data;
		} catch {
			professions = [];
		}
	}

	$effect(() => {
		if (showGenerateModal && professions.length === 0) {
			loadReferenceData();
		}
	});

	function openGenerate() {
		generateProfessionId = '';
		generateIssuingSpaceId = '';
		generateDestSpaceId = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateProfessionId || !generateDestSpaceId || !generateIssuingSpaceId) return;
		generating = true;
		pageError = '';
		try {
			const result = await credentialCheckerAdminApi.generate({
				profession_id: generateProfessionId,
				issuing_space_id: generateIssuingSpaceId,
				destination_space_id: generateDestSpaceId
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

	async function approveItem(item: CredentialCheckerItem) {
		acting = true;
		pageError = '';
		try {
			const result = await credentialCheckerAdminApi.approve(item.id);
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

	async function rejectItem(item: CredentialCheckerItem) {
		acting = true;
		pageError = '';
		try {
			const result = await credentialCheckerAdminApi.reject(item.id);
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

	// ── Status filter options (HITL review queue) ───────────────────────────
	const statusFilter = $derived<FilterDef>({
		key: 'status',
		label: t('common.status', 'Status'),
		options: [
			{ value: '', label: t('common.all', 'All') },
			...credentialCheckerStatusOptions
		]
	});

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: CredentialCheckerItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push(
				{
					label: t('common.edit', 'Edit'),
					icon: Pencil,
					onclick: (row: CredentialCheckerItem, ctx) => ctx?.openEdit(row)
				},
				{
					label: t('common.approve', 'Approve'),
					icon: CheckCircle,
					onclick: approveItem,
					show: (row: CredentialCheckerItem) => row.status !== 'published'
				},
				{
					label: t('common.reject', 'Reject'),
					icon: XCircle,
					variant: 'destructive',
					onclick: rejectItem,
					show: (row: CredentialCheckerItem) => ['draft', 'ai_draft', 'pending'].includes(row.status)
				}
			);
		}
		return acts;
	});

	const countryByIdReady = $derived(countryUtilsById.ready);

	// Edit form — reviewers correct draft fields before approving (CC-18).
	// Keys match UpdatePathwayRequest; profession/status stay immutable here
	// (a wrong-profession draft should be rejected + regenerated, and only
	// the approve endpoint may publish — the backend enforces both).
	const fields = $derived.by<FieldDef[]>(() => {
		countryByIdReady; // re-derive once the 250 primary geo space countries load
		return [
			{ key: 'destination_space_id', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'autocomplete', required: true, placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: countryUtilsById.fetcher, initialFetcher: countryUtilsById.initialFetcher, getSelectedLabel: countryUtilsById.getSelectedLabel },
			{ key: 'issuing_space_id', label: t('app.tools.col_issuing_country', 'Issuing Country'), type: 'autocomplete', placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: countryUtilsById.fetcher, initialFetcher: countryUtilsById.initialFetcher, getSelectedLabel: countryUtilsById.getSelectedLabel },
			{ key: 'regulatory_body', label: t('app.tools.col_regulatory_body', 'Regulatory Body'), type: 'text' },
			{ key: 'regulatory_body_url', label: t('credential_checker.regulatory_url', 'Regulatory body URL'), type: 'url' },
			{ key: 'pathway_type', label: t('app.tools.col_pathway_type', 'Pathway Type'), type: 'text' },
			{ key: 'timeline_estimate', label: t('credential_checker.timeline', 'Timeline estimate'), type: 'text' },
			{ key: 'cost_estimate_usd', label: t('credential_checker.cost_usd', 'Cost estimate (USD)'), type: 'text' },
			{ key: 'pathway_description', label: t('app.tools.col_summary', 'Summary'), type: 'textarea' },
			{ key: 'official_application_url', label: t('credential_checker.application_url', 'Official application URL'), type: 'url' }
		];
	});

	const config = $derived.by<EntityPageConfig>(() => {
		countryByIdReady; // re-derive once the 250 primary geo space countries load
		return {
			title: t('app.tools.credential_checker_heading', 'Credential Checker Review Queue'),
			description: t('app.tools.credential_checker_desc', 'Review and approve AI-generated credential checker guides'),
			hideCreate: true,
			editPermission: 'credential_checker.manage',
			pageSize: 25,
			filters: [statusFilter,
				{
					key: 'dest',
					label: t('app.tools.col_destination', 'Destination'),
					type: 'autocomplete',
					placeholder: t('app.tools.country_placeholder', 'Search country…'),
					emptyMessage: t('common.no_results', 'No results found'),
					fetcher: countryUtilsById.fetcher,
					initialFetcher: countryUtilsById.initialFetcher,
					getSelectedLabel: countryUtilsById.getSelectedLabel
				}
			],
		columns: [
			{ key: 'profession_name', label: t('app.tools.col_profession', 'Profession'), sortable: true },
			{ key: 'destination_space_name', label: t('app.tools.col_dest_country', 'Destination'), sortable: true },
			{ key: 'issuing_space_name', label: t('app.tools.col_issuing_country', 'Issuing Country'), sortable: true },
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
			const queryParts: string[] = [];
			queryParts.push(`page=${params.page + 1}`);
			queryParts.push(`limit=${params.pageSize}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				queryParts.push(`sort=${params.sort}`);
			}
			if (params.filter?.dest) {
				queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			}
			if (params.filter?.status) {
				queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			}
			const result = await credentialCheckerAdminApi.list(queryParts.join('&'));
			if (result.ok) {
				const body = result.data;
				const data = (body.data as CredentialCheckerItem[]) ?? [];
				const total = body.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / params.pageSize));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		update: async (id, body) => {
			const res = await credentialCheckerAdminApi.update(id, body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: CredentialCheckerView
	};
});
</script>

<svelte:head>
	<title>{t('app.tools.credential_checker_page_title', 'Credential Checker Review Queue — {site} Admin')}</title>
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

<!-- ── Community note moderation queue ─────────────────────────────────── -->
{#if canManage && pendingNotes.length > 0}
	<section class="mt-8">
		<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide" style="color: var(--color-muted)">
			{t('app.tools.notes_queue', 'Community notes pending review')} ({pendingNotes.length})
		</h2>
		<div class="space-y-2">
			{#each pendingNotes as note (note.id)}
				<div class="rounded-lg border p-3" style="border-color: var(--color-border); background: var(--color-surface)">
					<p class="text-sm" style="color: var(--color-foreground)">{note.note}</p>
					<div class="mt-1 text-xs" style="color: var(--color-muted)">
						{note.profession_name ?? '—'}
						{#if note.destination_space_name}· {note.destination_space_name}{/if}
						{#if note.user_name}· {note.user_name}{/if}
						{#if note.created_at}· {new Date(note.created_at).toLocaleDateString()}{/if}
					</div>
					<div class="mt-2 flex gap-2">
						<Button size="sm" disabled={moderatingNote === note.id} onclick={() => moderateNote(note, 'published')}>
							{t('common.approve', 'Approve')}
						</Button>
						<Button size="sm" variant="ghost" disabled={moderatingNote === note.id} onclick={() => moderateNote(note, 'rejected')}>
							{t('common.reject', 'Reject')}
						</Button>
					</div>
				</div>
			{/each}
		</div>
	</section>
{/if}

<!-- ── Generate modal ────────────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_credential_guide', 'Generate Credential Guide')}>
	<div class="space-y-4">
		<div>
			<label for="gen-profession" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_profession', 'Profession')}</label>
			<AutocompleteSelect
				id="gen-profession"
				bind:value={generateProfessionId}
				options={[{ value: '', label: t('credential_checker.select_profession', 'Select a profession…') }, ...professions.map((prof) => ({ value: prof.id, label: prof.name }))]}
				class="w-full"
			/>
		</div>
		<div>
			<label for="gen-issuing-country" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_issuing_country', 'Issuing Country')}</label>
			<Autocomplete
				id="gen-issuing-country"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtilsById.fetcher}
				initialFetcher={countryUtilsById.initialFetcher}
				selectedId={generateIssuingSpaceId || null}
				selectedLabel={generateIssuingSpaceId ? countryUtilsById.getSelectedLabel(generateIssuingSpaceId) : null}
				onSelect={(item: AutocompleteItem) => { generateIssuingSpaceId = item.id; }}
				onClear={() => { generateIssuingSpaceId = ''; }}
			/>
		</div>
		<div>
			<label for="gen-dest-country" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_dest_country', 'Destination Country')}</label>
			<Autocomplete
				id="gen-dest-country"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtilsById.fetcher}
				initialFetcher={countryUtilsById.initialFetcher}
				selectedId={generateDestSpaceId || null}
				selectedLabel={generateDestSpaceId ? countryUtilsById.getSelectedLabel(generateDestSpaceId) : null}
				onSelect={(item: AutocompleteItem) => { generateDestSpaceId = item.id; }}
				onClear={() => { generateDestSpaceId = ''; }}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !generateProfessionId || !generateIssuingSpaceId || !generateDestSpaceId}>
			{generating ? t('app.tools.generating', 'Generating...') : t('app.tools.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View modal body ──────────────────────────────────────────────── -->
{#snippet CredentialCheckerView({ row }: { row: CredentialCheckerItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_profession', 'Profession')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.profession_name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_issuing_country', 'Issuing Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.issuing_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_regulatory_body', 'Regulatory Body')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.regulatory_body || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_pathway_type', 'Pathway Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.pathway_type || '—'}</p>
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
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_summary', 'Pathway Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.pathway_description || '—'}</p>
		</div>
		{#if row.ai_generated}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.ai_generated', 'AI Generated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t('common.yes', 'Yes')} ({row.ai_model ?? '—'})</p>
			</div>
		{/if}
		{#if row.hitl_reviewed_by || row.hitl_reviewed_by_name}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.hitl_reviewed_by', 'HITL Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hitl_reviewed_by_name ?? row.hitl_reviewed_by}</p>
			</div>
		{/if}
	</div>
{/snippet}
