<script lang="ts">
	// /tools/attestation-guide — HITL review queue with Approve, Generate, and
	// community update review actions. Aligned with backend PathwayDTO fields.
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { attestationGuideAdminApi , spacesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
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
	let activeTab = $state<'pathways' | 'updates'>('pathways');

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

	async function loadUpdates() {
		loadingUpdates = true;
		try {
			const result = await attestationGuideAdminApi.listUpdates('?status=pending');
			if (result.ok) {
				const resp = result.data as { data?: CommunityUpdateItem[] } | undefined;
				updates = resp?.data ?? [];
			} else {
				updates = [];
			}
		} catch {
			updates = [];
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
				updates = updates.filter((u) => u.id !== id);
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
		if (tab === 'updates') loadUpdates();
	}

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
			acts.push({
				label: t('common.generate', 'Generate'),
				icon: Sparkles,
				onclick: openGenerate,
				show: () => true
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'document_type', label: t('common.doc_type', 'Document Type'), type: 'text' },
		{ key: 'issuing_space_id', label: t('common.issuing_country', 'Issuing Country ID'), type: 'text' },
		{ key: 'destination_space_id', label: t('common.dest_country', 'Destination Country ID'), type: 'text' },
		{ key: 'apostille_required', label: t('common.apostille', 'Apostille Required'), type: 'checkbox' },
		{ key: 'pathway_steps', label: t('common.pathway_steps', 'Pathway Steps (JSON)'), type: 'textarea' }
	];

		const filters = $derived<FilterDef[]>([
		{ key: 'dest', label: t('app.tools.col_destination', 'Destination'), options: destOptions.map((s) => ({ value: s.id, label: s.name })) }
	]);

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
			if (params.page) queryParts.push(`page_offset=${params.page}`);
			if (params.pageSize) queryParts.push(`page_size=${params.pageSize}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.filter?.dest) queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			const queryString = queryParts.length > 0 ? `?${queryParts.join('&')}` : '';
			const result = await attestationGuideAdminApi.list(queryString);
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
	<title>{t('app.tools.attestation_guide_page_title', 'Attestation Guide Review Queue — JapaConnect Admin')}</title>
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
	<EntityPage {config} bind:controller={entityController} />
{:else}
	<div class="space-y-3">
		{#if loadingUpdates}
			<p class="text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
		{:else if updates.length === 0}
			<p class="text-sm" style="color: var(--color-muted)">
				{t('app.tools.no_pending_updates', 'No pending community updates.')}
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
							</p>
							<p class="text-xs" style="color: var(--color-muted)">
								{update.fee_estimate ?? '—'} · {update.turnaround_days ?? '—'} days
							</p>
						</div>
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
					</div>
					{#if update.note}
						<p class="mt-2 text-sm" style="color: var(--color-foreground)">{update.note}</p>
					{/if}
				</div>
			{/each}
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
			<label for="gen-issuing" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.issuing_country', 'Issuing Country (Space ID)')}</label>
			<input
				id="gen-issuing"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateIssuingId}
				placeholder="UUID of the issuing country space"
			/>
		</div>
		<div>
			<label for="gen-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.dest_country', 'Destination Country (Space ID)')}</label>
			<input
				id="gen-dest"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateDestId}
				placeholder="UUID of the destination country space"
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
