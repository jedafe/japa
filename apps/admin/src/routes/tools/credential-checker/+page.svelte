<script lang="ts">
	// /tools/credential-checker — admin HITL review queue for AI-generated credential checker guides.
	// Fields map to the backend PathwayDTO contract (§94, ADR-102, ADR-088).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { credentialCheckerAdminApi, apiFetch } from '$lib/api';

	type CredentialCheckerItem = {
		id: string;
		profession_name: string;
		profession_slug: string;
		destination_space_name: string;
		issuing_space_name: string;
		regulatory_body: string;
		pathway_type: string;
		pathway_description: string;
		status: string;
		ai_generated: boolean;
		is_active: boolean;
		updated_at?: string;
		created_at?: string;
	};

	type ProfessionOption = { id: string; slug: string; name: string; category?: string };
	type GeoSpaceOption = { id: string; name: string; geo_code?: string };

	const canManage = $derived(hasPermission('credential_checker.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// ── Generate modal state ────────────────────────────────────────────────
	let showGenerateModal = $state(false);
	let generateProfessionId = $state('');
	let generateIssuingSpaceId = $state('');
	let generateDestSpaceId = $state('');
	let generating = $state(false);

	// ── Reference data for the generate modal (professions + countries) ─────
	let professions = $state<ProfessionOption[]>([]);
	let destinations = $state<GeoSpaceOption[]>([]);

	async function loadReferenceData() {
		try {
			const profRes = await apiFetch<ProfessionOption[]>('/credentials/professions');
			if (profRes.data) professions = profRes.data;
		} catch {
			professions = [];
		}
		try {
			const destRes = await apiFetch<GeoSpaceOption[]>('/spaces?level=country');
			if (destRes.data) destinations = destRes.data;
		} catch {
			destinations = [];
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

	// ── Status filter options (HITL review queue) ───────────────────────────
	const statusFilter: FilterDef = {
		key: 'status',
		label: t('common.status', 'Status'),
		options: [
			{ value: '', label: t('common.all', 'All') },
			{ value: 'ai_draft', label: t('common.ai_draft', 'AI Draft') },
			{ value: 'pending', label: t('common.pending', 'Pending') },
			{ value: 'published', label: t('common.published', 'Published') },
			{ value: 'rejected', label: t('common.rejected', 'Rejected') },
			{ value: 'draft', label: t('common.draft', 'Draft') }
		]
	};

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: CredentialCheckerItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveItem,
				show: (row: CredentialCheckerItem) => row.status !== 'published'
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'profession_name', label: t('app.tools.col_profession', 'Profession'), type: 'text' },
		{ key: 'destination_space_name', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'text' },
		{ key: 'issuing_space_name', label: t('app.tools.col_issuing_country', 'Issuing Country'), type: 'text' },
		{ key: 'pathway_description', label: t('app.tools.col_summary', 'Summary'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.credential_checker_heading', 'Credential Checker Review Queue'),
		description: t('app.tools.credential_checker_desc', 'Review and approve AI-generated credential checker guides'),
		readonly: true,
		pageSize: 25,
		filters: [statusFilter],
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
			queryParts.push(`page=${params.page}`);
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
		viewSnippet: CredentialCheckerView
	});
</script>

<svelte:head>
	<title>{t('app.tools.credential_checker_page_title', 'Credential Checker Review Queue — JapaConnect Admin')}</title>
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
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_credential_guide', 'Generate Credential Guide')}>
	<div class="space-y-4">
		<div>
			<label for="gen-profession" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_profession', 'Profession')}</label>
			<select
				id="gen-profession"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateProfessionId}
			>
				<option value="">{t('credential_checker.select_profession', 'Select a profession…')}</option>
				{#each professions as prof (prof.id)}
					<option value={prof.id}>{prof.name}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="gen-issuing-country" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_issuing_country', 'Issuing Country')}</label>
			<select
				id="gen-issuing-country"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateIssuingSpaceId}
			>
				<option value="">{t('geo.select_destination', 'Select a country…')}</option>
				{#each destinations as dest (dest.id)}
					<option value={dest.id}>{dest.name}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="gen-dest-country" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_dest_country', 'Destination Country')}</label>
			<select
				id="gen-dest-country"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateDestSpaceId}
			>
				<option value="">{t('geo.select_destination', 'Select a country…')}</option>
				{#each destinations as dest (dest.id)}
					<option value={dest.id}>{dest.name}</option>
				{/each}
			</select>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !generateProfessionId || !generateIssuingSpaceId || !generateDestSpaceId}>
			{generating ? t('app.tools.generating', 'Generating...') : t('app.tools.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
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
	</div>
{/snippet}
