<script lang="ts">
	// /tools/visa-checklist — read-only monitoring page for visa document
	// checklist user progress. Displays raw per-item progress rows from
	// GET /admin/checklist/progress (§70, ADR-070).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { Eye } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '#lib/components/ui/data-table/index.js';
	import { visaChecklistAdminApi } from '#lib/api.js';

	type ChecklistProgressRow = {
		id: string;
		user_id: string;
		guide_id: string;
		guide_title: string;
		item_key: string;
		completed: boolean;
		completed_at: string;
		notes: string;
		updated_at: string;
		username: string;
	};

	type CompletionStat = {
		guide_id: string;
		title: string;
		guide_deleted?: boolean;
		total_items: number;
		total_users: number;
		users_with_any_completion: number;
		users_fully_completed: number;
		avg_completion_pct: number;
	};

	const canManage = $derived(hasPermission('checklist.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let stats = $state<CompletionStat[]>([]);
	let statsTotal = $state(0);
	let statsPage = $state(0);
	const statsPageSize = 10;
	let statsLoading = $state(false);
	let statsError = $state('');

	// Load completion stats (§70: GET /admin/checklist/completion-stats) —
	// server-paginated; rows carry real completion rates computed against
	// each guide's live item set.
	$effect(() => {
		if (!canManage) return;
		const page = statsPage;
		statsLoading = true;
		statsError = '';
		visaChecklistAdminApi
			.completionStats(`page=${page + 1}&limit=${statsPageSize}`)
			.then((resp: unknown) => {
				const r = resp as { data?: CompletionStat[]; pagination?: { total?: number } };
				stats = r.data ?? [];
				statsTotal = r.pagination?.total ?? stats.length;
			})
			.catch((e: unknown) => {
				statsError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
			})
			.finally(() => {
				statsLoading = false;
			});
	});

	const statsTotalPages = $derived(Math.max(1, Math.ceil(statsTotal / statsPageSize)));

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

	// §70 admin filters — guide_id/user_id are exact-match UUID filters the
	// backend supports; 'text' FilterDef applies them on Apply.
	const filters = $derived.by<FilterDef[]>(() => [
		{
			key: 'guide_id',
			label: t('app.tools.col_guide_id', 'Guide ID'),
			type: 'text',
			placeholder: t('app.tools.filter_guide_id_ph', 'Filter by guide UUID…')
		},
		{
			key: 'user_id',
			label: t('app.tools.col_user_id', 'User ID'),
			type: 'text',
			placeholder: t('app.tools.filter_user_id_ph', 'Filter by user UUID…')
		}
	]);

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.visa_checklist_heading', 'Visa Checklist Progress'),
		description: t('app.tools.visa_checklist_desc', 'Monitor user progress on visa document checklists'),
		readonly: true,
		exportable: true,
		exportFilename: 'visa-checklist-progress',
		pageSize: 25,
		filters,
		columns: [
			{ key: 'username', label: t('app.tools.col_username', 'Username'), sortable: true },
			{
				key: 'guide_title',
				label: t('app.tools.col_guide', 'Guide'),
				sortable: true,
				sortKey: 'guide',
				render: (value: string, row: ChecklistProgressRow) =>
					value || (row.guide_id ? `${row.guide_id.slice(0, 8)}…` : '—')
			},
			{ key: 'item_key', label: t('app.tools.col_item_key', 'Item'), sortable: true },
			{
				key: 'completed',
				label: t('app.tools.col_completed', 'Completed'),
				sortable: true,
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
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// Server-side pagination — pass page/limit/q/sort/dir + filters to
			// the backend. EntityPage encodes sort as "column:dir".
			// Ref: geo-aware-tool-pattern.md (admin list must filter+sort+paginate).
			const queryParts: string[] = [];
			queryParts.push(`page=${params.page + 1}`);
			if (params.pageSize) queryParts.push(`limit=${params.pageSize}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				const [sortCol, sortDir] = params.sort.split(':');
				if (sortCol) queryParts.push(`sort=${encodeURIComponent(sortCol)}`);
				if (sortDir) queryParts.push(`dir=${encodeURIComponent(sortDir)}`);
			}
			if (params.filter?.guide_id) {
				queryParts.push(`guide_id=${encodeURIComponent(params.filter.guide_id)}`);
			}
			if (params.filter?.user_id) {
				queryParts.push(`user_id=${encodeURIComponent(params.filter.user_id)}`);
			}
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
			<div class="mt-3 overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr style="color: var(--color-muted)">
							<th class="pb-2 text-start font-medium">{t('app.tools.col_guide', 'Guide')}</th>
							<th class="pb-2 text-end font-medium">{t('app.tools.visa_checklist_total_items', 'Items')}</th>
							<th class="pb-2 text-end font-medium">{t('app.tools.visa_checklist_total_users', 'Users w/ Progress')}</th>
							<th class="pb-2 text-end font-medium">{t('app.tools.visa_checklist_users_completed', 'Any Completed')}</th>
							<th class="pb-2 text-end font-medium">{t('app.tools.visa_checklist_users_full', 'Fully Completed')}</th>
							<th class="pb-2 text-end font-medium">{t('app.tools.visa_checklist_avg_pct', 'Avg. Completion')}</th>
						</tr>
					</thead>
					<tbody>
						{#each stats as s (s.guide_id)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="py-2" style="color: var(--color-foreground)">
									{#if s.title}
										{s.title}
									{:else}
										<span class="font-mono text-xs">{s.guide_id.slice(0, 8)}…</span>
									{/if}
									{#if s.guide_deleted}
										<span class="ml-2 rounded px-1.5 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-danger) 12%, transparent); color: var(--color-danger)">
											{t('app.tools.visa_checklist_guide_deleted', 'guide removed')}
										</span>
									{/if}
								</td>
								<td class="py-2 text-end">{s.total_items}</td>
								<td class="py-2 text-end">{s.total_users}</td>
								<td class="py-2 text-end">{s.users_with_any_completion}</td>
								<td class="py-2 text-end">{s.users_fully_completed}</td>
								<td class="py-2 text-end">{Math.round(s.avg_completion_pct)}%</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			{#if statsTotalPages > 1}
				<div class="mt-3 flex items-center justify-end gap-2 text-sm">
					<button
						type="button"
						disabled={statsPage <= 0 || statsLoading}
						onclick={() => (statsPage -= 1)}
						class="rounded px-2 py-1 disabled:opacity-40"
						style="border: 1px solid var(--color-border); color: var(--color-foreground)"
					>
						{t('common.prev', 'Previous')}
					</button>
					<span style="color: var(--color-muted)">{statsPage + 1} / {statsTotalPages}</span>
					<button
						type="button"
						disabled={statsPage + 1 >= statsTotalPages || statsLoading}
						onclick={() => (statsPage += 1)}
						class="rounded px-2 py-1 disabled:opacity-40"
						style="border: 1px solid var(--color-border); color: var(--color-foreground)"
					>
						{t('common.next', 'Next')}
					</button>
				</div>
			{/if}
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
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_guide', 'Guide')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.guide_title || '—'}
				<span class="block font-mono text-xs" style="color: var(--color-muted)">{row.guide_id || ''}</span>
			</p>
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
