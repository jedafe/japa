<script lang="ts">
	// /tools/visa-checklist — read-only monitoring page for visa document
	// checklist user progress. Displays raw per-item progress rows from
	// GET /admin/checklist/progress (§70, ADR-070).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { visaChecklistAdminApi } from '$lib/api';

	type ChecklistProgressRow = {
		id: string;
		user_id: string;
		guide_id: string;
		item_key: string;
		completed: boolean;
		completed_at: string;
		notes: string;
		updated_at: string;
		username: string;
	};

	type CompletionStat = {
		guide_id: string;
		total_users: number;
		users_with_any_completion: number;
	};

	const canManage = $derived(hasPermission('checklist.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let stats = $state<CompletionStat[]>([]);
	let statsLoading = $state(false);
	let statsError = $state('');

	// Load completion stats (§70: GET /admin/checklist/completion-stats).
	// The route + API method exist but were previously unused.
	$effect(() => {
		if (!canManage) return;
		statsLoading = true;
		statsError = '';
		visaChecklistAdminApi
			.completionStats()
			.then((resp: unknown) => {
				const r = resp as { data?: CompletionStat[] };
				stats = r.data ?? [];
			})
			.catch((e: unknown) => {
				statsError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
			})
			.finally(() => {
				statsLoading = false;
			});
	});

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ChecklistProgressRow, ctx) => ctx?.openView(_row) }
		];
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is
	// never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'username', label: t('app.tools.col_username', 'Username'), type: 'text' },
		{ key: 'guide_id', label: t('app.tools.col_guide_id', 'Guide ID'), type: 'text' },
		{ key: 'item_key', label: t('app.tools.col_item_key', 'Item'), type: 'text' },
		{ key: 'completed', label: t('app.tools.col_completed', 'Completed'), type: 'checkbox' },
		{ key: 'notes', label: t('app.tools.col_notes', 'Notes'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.visa_checklist_heading', 'Visa Checklist Progress'),
		description: t('app.tools.visa_checklist_desc', 'Monitor user progress on visa document checklists'),
		readonly: true,
		exportable: true,
		exportFilename: 'visa-checklist-progress',
		pageSize: 25,
		columns: [
			{ key: 'username', label: t('app.tools.col_username', 'Username') },
			{ key: 'guide_id', label: t('app.tools.col_guide_id', 'Guide') },
			{ key: 'item_key', label: t('app.tools.col_item_key', 'Item') },
			{
				key: 'completed',
				label: t('app.tools.col_completed', 'Completed'),
				render: (value: boolean) => (value ? '✓' : '—')
			},
			{
				key: 'notes',
				label: t('app.tools.col_notes', 'Notes'),
				sortable: false,
				render: (value: string) => (value ? value : '—')
			},
			{
				key: 'updated_at',
				label: t('app.tools.col_updated_at', 'Updated'),
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const queryParts: string[] = [];
			queryParts.push(`page=${params.page + 1}`);
			if (params.pageSize) queryParts.push(`limit=${params.pageSize}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			const queryString = queryParts.join('&');
			const result = await visaChecklistAdminApi.listProgress(queryString || undefined);
			if (result.ok) {
				const resp = result.data as { data?: ChecklistProgressRow[]; pagination?: { total?: number } };
				const data = resp.data ?? [];
				const total = resp.pagination?.total ?? data.length;
				const totalPages = Math.ceil(total / (params.pageSize || 25));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ChecklistView
	});
</script>

<svelte:head>
	<title>{t('app.tools.visa_checklist_page_title', 'Visa Checklist Progress — {site} Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<!-- ── Completion stats (§70: GET /admin/checklist/completion-stats) ─────── -->
{#if canManage}
	<div class="mb-6 rounded-lg border p-4" style="border-color: var(--color-border)">
		<h2 class="text-sm font-semibold uppercase tracking-wide" style="color: var(--color-muted)">
			{t('app.tools.visa_checklist_stats_heading', 'Completion Rates by Guide')}
		</h2>
		{#if statsLoading}
			<p class="mt-2 text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading…')}</p>
		{:else if statsError}
			<p class="mt-2 text-sm" style="color: var(--color-danger)">{statsError}</p>
		{:else if stats.length === 0}
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('app.tools.visa_checklist_stats_empty', 'No completion data yet.')}
			</p>
		{:else}
			<table class="mt-3 w-full text-sm">
				<thead>
					<tr style="color: var(--color-muted)">
						<th class="pb-2 text-start font-medium">{t('app.tools.col_guide_id', 'Guide')}</th>
						<th class="pb-2 text-end font-medium">{t('app.tools.visa_checklist_total_users', 'Total Users')}</th>
						<th class="pb-2 text-end font-medium">{t('app.tools.visa_checklist_users_completed', 'Users w/ Any Completion')}</th>
					</tr>
				</thead>
				<tbody>
					{#each stats as s (s.guide_id)}
						<tr class="border-t" style="border-color: var(--color-border)">
							<td class="py-2 font-mono text-xs" style="color: var(--color-foreground)">{s.guide_id}</td>
							<td class="py-2 text-end">{s.total_users}</td>
							<td class="py-2 text-end">{s.users_with_any_completion}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── View modal body ──────────────────────────────────────────────── -->
{#snippet ChecklistView({ row }: { row: ChecklistProgressRow })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_username', 'Username')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.username || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_guide_id', 'Guide ID')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.guide_id || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_item_key', 'Item')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.item_key || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_completed', 'Completed')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.completed ? '✓' : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_notes', 'Notes')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.notes || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
