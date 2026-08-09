<script lang="ts">
	// /tools/readiness — read-only monitoring page for japa readiness assessment results.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { readinessAdminApi } from '$lib/api';

	type ReadinessItem = {
		id: string;
		user_email: string;
		score: number;
		readiness_level: string;
		completed_at?: string;
	};

	const canManage = $derived(hasPermission('assessment.view_results'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ReadinessItem, ctx) => ctx?.openView(_row) }
		];
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'user_email', label: t('app.tools.col_user_email', 'User Email'), type: 'email' },
		{ key: 'score', label: t('app.tools.col_score', 'Score'), type: 'number' },
		{ key: 'readiness_level', label: t('app.tools.col_readiness_level', 'Readiness Level'), type: 'text' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.readiness_heading', 'Readiness Assessment Results'),
		description: t('app.tools.readiness_desc', 'Monitor japa readiness assessment results submitted by users'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'user_email', label: t('app.tools.col_user_email', 'User Email'), sortable: true },
			{
				key: 'score',
				label: t('app.tools.col_score', 'Score'),
				sortable: true,
				render: (value: number) => (value != null ? String(value) : '—')
			},
			{
				key: 'readiness_level',
				label: t('app.tools.col_readiness_level', 'Readiness Level'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'completed_at',
				label: t('app.tools.col_completed_at', 'Completed'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await readinessAdminApi.listResults();
			if (result.ok) {
				const data = (result.data as ReadinessItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ReadinessView
	});
</script>

<svelte:head>
	<title>{t('app.tools.readiness_page_title', 'Readiness Assessment Results — JapaConnect Admin')}</title>
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

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet ReadinessView({ row }: { row: ReadinessItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_user_email', 'User Email')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.user_email || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_score', 'Score')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.score != null ? String(row.score) : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_readiness_level', 'Readiness Level')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.readiness_level || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_completed_at', 'Completed')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.completed_at ? new Date(row.completed_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
