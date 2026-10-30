<script lang="ts">
	// /tools/readiness — admin monitoring + management page for the Japa
	// Readiness Assessment (Section 71, ADR-071).
	// Shows score distribution stats, result list with server-side pagination,
	// and a form config editor for scoring_config (weights + thresholds).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Settings, BarChart3 } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Modal } from '$lib/components/ui/modal';
	import { readinessAdminApi } from '$lib/api';
	import { JsonValueEditor } from '@japaconnect/ui';

	type ReadinessItem = {
		id: string;
		user_id: string;
		username: string;
		total_score: number;
		threshold_label: string;
		submission_id: string;
		created_at: string;
	};

	type ScoreDistributionItem = {
		threshold_label: string;
		count: number;
		avg_score: number;
	};

	type CompletionStats = {
		total_submissions: number;
		total_scored: number;
		avg_score: number;
		completion_rate: number;
	};

	type RoadmapStep = {
		title: string;
		description: string;
		content_slug: string;
		dimension: string;
		priority: number;
	};

	type ScoringConfig = {
		max_score: number;
		thresholds: { min: number; max: number; label: string; color: string }[];
		dimension_weights: Record<string, number>;
		curated_roadmap?: Record<string, RoadmapStep[]>;
	};

	const canManage = $derived(hasPermission('assessment.view_results'));
	const canEditForm = $derived(hasPermission('assessment.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let statsError = $state('');
	let configError = $state('');
	let configSaving = $state(false);
	let showConfig = $state(false);
	let scoringConfig = $state<ScoringConfig | null>(null);
	let curatedRoadmapText = $state('{}');
	let stats = $state<{ distribution: ScoreDistributionItem[]; completion: CompletionStats | null }>({
		distribution: [],
		completion: null
	});

	// Score-band filter options derived from the assessment's configured
	// thresholds (Rule 16 — no hardcoded data that changes over time). Falls
	// back to an empty list until the form config loads on mount.
	const scoreBandOptions = $derived(
		(scoringConfig?.thresholds ?? []).map((th) => ({
			value: `${th.min}-${th.max}`,
			label: `${th.min}–${th.max}`
		}))
	);

	// Resolve a threshold color to a CSS value. Threshold colors may be stored
	// as chart theme token names (e.g. "chart-5") or legacy hex values. Token
	// names resolve to var(--chart-N) so the palette adapts to light/dark mode.
	function resolveThresholdColor(color: string): string {
		if (!color) return 'var(--color-muted)';
		if (color.startsWith('#')) return color;
		if (color.startsWith('chart-')) return `var(--${color})`;
		return color;
	}

	// ── Result detail modal (dimension breakdown, roadmap, answers) ────
	type ResultDetail = {
		id: string;
		total_score: number;
		threshold_label: string;
		dimension_scores: Record<string, number>;
		roadmap: { title: string; description: string; content_slug: string; dimension: string; ai_generated?: boolean }[];
		submission_values: Record<string, unknown>;
		created_at: string;
	};
	let detail = $state<ResultDetail | null>(null);
	let detailLoading = $state(false);
	let detailOpen = $state(false);

	async function loadDetail(id: string) {
		detail = null;
		detailLoading = true;
		detailOpen = true;
		const result = await readinessAdminApi.getResult(id);
		detailLoading = false;
		if (result.ok) {
			detail = result.data as ResultDetail;
		} else {
			pageError = result.error ?? t('common.error', 'An error occurred');
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: ReadinessItem) => loadDetail(row.id) }
		];
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'username', label: t('app.tools.col_username', 'Username'), type: 'text' },
		{ key: 'total_score', label: t('app.tools.col_score', 'Score'), type: 'number' },
		{ key: 'threshold_label', label: t('app.tools.col_readiness_level', 'Readiness Level'), type: 'text' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.readiness_heading', 'Readiness Assessment Results'),
		description: t('app.tools.readiness_desc', 'Monitor japa readiness assessment results submitted by users'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'username', label: t('app.tools.col_username', 'Username'), sortable: true },
			{
				key: 'total_score',
				label: t('app.tools.col_score', 'Score'),
				sortable: true,
				render: (value: number) => (value != null ? String(value) : '—')
			},
			{
				key: 'threshold_label',
				label: t('app.tools.col_readiness_level', 'Readiness Level'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.tools.col_completed_at', 'Completed'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		filters: [
			{
				key: 'score_band',
				label: t('app.tools.col_readiness_level', 'Readiness Level'),
				options: scoreBandOptions
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const queryParts: string[] = [];
			queryParts.push(`page=${params.page + 1}`);
			if (params.pageSize) queryParts.push(`limit=${params.pageSize}`);
			// EntityPage passes sort as "key:dir" (e.g. "total_score:desc").
			// The backend accepts separate sort + dir params.
			if (params.sort) {
				const [sortKey, sortDir] = params.sort.split(':');
				queryParts.push(`sort=${encodeURIComponent(sortKey)}`);
				if (sortDir) queryParts.push(`dir=${encodeURIComponent(sortDir)}`);
			}
			// Score-band filter: the filter value is "min-max" (e.g. "40-59").
			// Split into min_score + max_score query params.
			const band = params.filter?.score_band;
			if (band) {
				const [minStr, maxStr] = band.split('-');
				const min = parseInt(minStr, 10);
				const max = parseInt(maxStr, 10);
				if (!isNaN(min)) queryParts.push(`min_score=${min}`);
				if (!isNaN(max)) queryParts.push(`max_score=${max}`);
			}
			const queryString = queryParts.length > 0 ? queryParts.join('&') : '';
			const result = await readinessAdminApi.listResults(queryString);
			if (result.ok) {
				const resp = result.data as { data: ReadinessItem[]; pagination: { page: number; limit: number; total: number } };
				const data = resp.data ?? [];
				const total = resp.pagination?.total ?? data.length;
				const totalPages = Math.ceil(total / (params.pageSize || 25));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
	});

	// ── Stats + form config ─────────────────────────────────────────────────
	async function loadStats() {
		statsError = '';
		const result = await readinessAdminApi.stats();
		if (result.ok) {
			const resp = result.data as {
				score_distribution: ScoreDistributionItem[];
				completion: CompletionStats;
			};
			stats = {
				distribution: resp.score_distribution ?? [],
				completion: resp.completion ?? null
			};
		} else {
			statsError = result.error ?? t('common.error', 'Failed to load stats');
		}
	}

	async function loadFormConfig() {
		configError = '';
		const result = await readinessAdminApi.getForm();
		if (result.ok) {
			const form = result.data as { scoring_config?: ScoringConfig };
			scoringConfig = form.scoring_config ?? null;
			curatedRoadmapText = JSON.stringify(scoringConfig?.curated_roadmap ?? {}, null, 2);
		} else {
			configError = result.error ?? t('common.error', 'Failed to load form config');
		}
	}

	async function saveFormConfig() {
		if (!scoringConfig) return;
		configSaving = true;
		configError = '';
		let curatedRoadmap: Record<string, RoadmapStep[]> | undefined;
		try {
			curatedRoadmap = JSON.parse(curatedRoadmapText) as Record<string, RoadmapStep[]>;
		} catch (e) {
			configSaving = false;
			configError = t('app.tools.roadmap_json_invalid', 'Curated roadmap is not valid JSON');
			return;
		}
		// Validate threshold colors — free-text values render as broken CSS
		// on the result gauge (BUG-UX-012). Allowed: #hex or chart-N token.
		const colorRe = /^(#[0-9a-fA-F]{3,8}|chart-\d+)?$/;
		const badColor = scoringConfig.thresholds.findIndex((th) => !colorRe.test(th.color));
		if (badColor >= 0) {
			configSaving = false;
			configError = t('app.tools.threshold_color_invalid', 'Threshold colors must be #hex or a chart-N token') + ` (#${badColor + 1})`;
			return;
		}
		const payload: ScoringConfig = { ...scoringConfig, curated_roadmap: curatedRoadmap };
		const result = await readinessAdminApi.updateForm({ scoring_config: payload });
		configSaving = false;
		if (!result.ok) {
			configError = result.error ?? t('common.error', 'Failed to save form config');
		} else {
			showConfig = false;
		}
	}

	$effect(() => {
		if (canManage) {
			loadStats();
			// Load the form config on mount so the score-band filter options
			// are driven by the API's configured thresholds (Rule 16).
			loadFormConfig();
		}
	});
</script>

<svelte:head>
	<title>{t('app.tools.readiness_page_title', 'Readiness Assessment Results — {site} Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<!-- Stats dashboard -->
{#if canManage}
	<div class="mb-6 space-y-4">
		<div class="flex items-center justify-between">
			<h2 class="flex items-center gap-2 text-sm font-semibold" style="color: var(--color-muted)">
				<BarChart3 size={16} />
				{t('app.tools.readiness_stats', 'Score Distribution & Completion')}
			</h2>
			{#if canEditForm}
				<button
					type="button"
					onclick={async () => {
						if (!scoringConfig) await loadFormConfig();
						showConfig = true;
					}}
					class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors hover:opacity-80"
					style="background: var(--color-primary); color: var(--color-primary-contrast, white)"
				>
					<Settings size={14} />
					{t('app.tools.readiness_form_config', 'Form Config')}
				</button>
			{/if}
		</div>

		{#if statsError}
			<p class="text-sm" style="color: var(--color-danger)">{statsError}</p>
		{:else if stats.completion}
			<div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
				<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('app.tools.total_submissions', 'Submissions')}
					</p>
					<p class="mt-1 text-xl font-semibold" style="color: var(--color-foreground)">
						{stats.completion.total_submissions}
					</p>
				</div>
				<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('app.tools.total_scored', 'Scored')}
					</p>
					<p class="mt-1 text-xl font-semibold" style="color: var(--color-foreground)">
						{stats.completion.total_scored}
					</p>
				</div>
				<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('app.tools.avg_score', 'Avg Score')}
					</p>
					<p class="mt-1 text-xl font-semibold" style="color: var(--color-foreground)">
						{stats.completion.avg_score}
					</p>
				</div>
				<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('app.tools.completion_rate', 'Completion')}
					</p>
					<p class="mt-1 text-xl font-semibold" style="color: var(--color-foreground)">
						{Math.round(stats.completion.completion_rate)}%
					</p>
				</div>
			</div>

			{#if stats.distribution.length > 0}
				<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
					<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('app.tools.score_distribution', 'Score Distribution by Threshold')}
					</p>
					<div class="space-y-2">
						{#each stats.distribution as d (d.threshold_label)}
							<div class="flex items-center gap-3">
								<span class="w-32 shrink-0 text-sm font-medium" style="color: var(--color-foreground)">
									{d.threshold_label}
								</span>
								<div class="h-2 flex-1 overflow-hidden rounded-full" style="background: var(--color-surface-subtle)">
									<div
										class="h-full transition-all"
										style="width: {Math.min(100, (d.count / Math.max(stats.completion?.total_scored || 1, 1)) * 100)}%; background: var(--color-primary)"
									></div>
								</div>
								<span class="w-16 shrink-0 text-end text-sm" style="color: var(--color-muted)">
									{d.count} ({d.avg_score})
								</span>
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{/if}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Question text/options/per-question weights live in the §06 Form
     Builder — link admins there since this page only edits scoring_config
     (BUG-UX-005). -->
{#if canEditForm}
	<p class="mt-4 text-xs" style="color: var(--color-muted)">
		{t('app.tools.readiness_edit_questions_hint', 'To edit questions, options, or per-question scores, use')}
		<a href="/forms" class="font-medium underline">{t('app.tools.readiness_forms_link', 'Forms')}</a>.
	</p>
{/if}

<!-- ── Result detail modal ─────────────────────────────────────────── -->
<Modal bind:open={detailOpen} title={t('app.tools.readiness_result_detail', 'Result detail')} size="lg">
			{#if detailLoading}
				<p class="text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading…')}</p>
			{:else if detail}
				<div class="space-y-5">
					<div class="flex items-baseline gap-3">
						<span class="text-3xl font-semibold" style="color: var(--color-foreground)">{detail.total_score}</span>
						<span class="text-sm font-medium" style="color: var(--color-primary)">{detail.threshold_label}</span>
						<span class="ms-auto text-xs" style="color: var(--color-muted)">{new Date(detail.created_at).toLocaleString()}</span>
					</div>
					{#if Object.keys(detail.dimension_scores ?? {}).length > 0}
						<div>
							<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.dimensions', 'Dimensions')}</p>
							<div class="space-y-2">
								{#each Object.entries(detail.dimension_scores) as [dim, score] (dim)}
									<div class="flex items-center gap-2 text-sm">
										<span class="w-36 shrink-0 capitalize" style="color: var(--color-foreground)">{dim.replace(/_/g, ' ')}</span>
										<div class="h-2 flex-1 overflow-hidden rounded-full" style="background: var(--color-surface-subtle)">
											<div class="h-full" style="width: {Math.min(100, Math.max(0, score))}%; background: var(--color-primary)"></div>
										</div>
										<span class="w-8 text-end text-xs" style="color: var(--color-muted)">{score}</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
					{#if detail.roadmap?.length > 0}
						<div>
							<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.roadmap', 'Roadmap')}</p>
							<ol class="space-y-2">
								{#each detail.roadmap as step, i (i)}
									<li class="text-sm" style="color: var(--color-foreground)">
										<span class="font-medium">{i + 1}. {step.title}</span>
										{#if step.ai_generated}<span class="ms-1 rounded px-1 text-xs" style="background: var(--color-surface-subtle); color: var(--color-muted)">AI</span>{/if}
										{#if step.description}<p class="text-xs" style="color: var(--color-muted)">{step.description}</p>{/if}
										{#if step.content_slug}<p class="text-xs font-mono" style="color: var(--color-muted)">/{step.content_slug}</p>{/if}
									</li>
								{/each}
							</ol>
						</div>
					{/if}
					{#if Object.keys(detail.submission_values ?? {}).length > 0}
						<div>
							<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.answers', 'Answers')}</p>
							<dl class="space-y-1 text-xs">
								{#each Object.entries(detail.submission_values) as [k, v] (k)}
									<div class="flex gap-2">
										<dt class="w-40 shrink-0 font-mono" style="color: var(--color-muted)">{k}</dt>
										<dd class="min-w-0 break-words" style="color: var(--color-foreground)">{Array.isArray(v) ? v.join(', ') : String(v)}</dd>
									</div>
								{/each}
							</dl>
						</div>
					{/if}
				</div>
			{/if}
</Modal>



<!-- ── Form config modal ────────────────────────────────────────────── -->
{#if showConfig && scoringConfig}
	<div class="fixed inset-0 z-50 flex justify-end">
		<button
			type="button"
			class="absolute inset-0 bg-black/40"
			aria-label={t('common.close', 'Close')}
			onclick={() => (showConfig = false)}
		></button>
		<div class="relative h-full w-full max-w-lg overflow-y-auto border-s p-6" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-semibold" style="color: var(--color-foreground)">
					{t('app.tools.readiness_form_config', 'Assessment Form Config')}
				</h2>
				<button type="button" onclick={() => (showConfig = false)} class="text-sm" style="color: var(--color-muted)">✕</button>
			</div>

			{#if configError}
				<p class="mb-3 text-sm" style="color: var(--color-danger)">{configError}</p>
			{/if}

			<div class="space-y-4">
				<div>
					<label class="block text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)" for="max-score">
						{t('app.tools.max_score', 'Max Score')}
					</label>
					<input
						id="max-score"
						type="number"
						value={scoringConfig!.max_score}
						oninput={(e) => (scoringConfig = { ...scoringConfig!, max_score: parseInt(e.currentTarget.value) || 0 })}
						class="mt-1 w-full rounded-lg border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
					/>
				</div>

				<div>
					<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('app.tools.thresholds', 'Thresholds')}
					</p>
					<div class="space-y-2">
						{#each scoringConfig!.thresholds as t, i (i)}
							<div class="flex items-center gap-2">
								<input
									type="number"
									value={t.min}
									placeholder="min"
									oninput={(e) => {
										const thresholds = [...scoringConfig!.thresholds];
										thresholds[i] = { ...thresholds[i], min: parseInt(e.currentTarget.value) || 0 };
										scoringConfig = { ...scoringConfig!, thresholds };
									}}
									class="w-16 rounded border px-2 py-1 text-xs"
									style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
								/>
								<input
									type="number"
									value={t.max}
									placeholder="max"
									oninput={(e) => {
										const thresholds = [...scoringConfig!.thresholds];
										thresholds[i] = { ...thresholds[i], max: parseInt(e.currentTarget.value) || 0 };
										scoringConfig = { ...scoringConfig!, thresholds };
									}}
									class="w-16 rounded border px-2 py-1 text-xs"
									style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
								/>
								<input
									type="text"
									value={t.label}
									placeholder="label"
									oninput={(e) => {
										const thresholds = [...scoringConfig!.thresholds];
										thresholds[i] = { ...thresholds[i], label: e.currentTarget.value };
										scoringConfig = { ...scoringConfig!, thresholds };
									}}
									class="flex-1 rounded border px-2 py-1 text-xs"
									style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
								/>
								<input
									type="text"
									value={t.color}
									placeholder="color"
									oninput={(e) => {
										const thresholds = [...scoringConfig!.thresholds];
										thresholds[i] = { ...thresholds[i], color: e.currentTarget.value };
										scoringConfig = { ...scoringConfig!, thresholds };
									}}
									class="w-24 rounded border px-2 py-1 text-xs"
									style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); border-inline-start: 4px solid {resolveThresholdColor(t.color)}"
								/>
							</div>
						{/each}
					</div>
				</div>

				<div>
					<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('app.tools.dimension_weights', 'Dimension Weights')}
					</p>
					<div class="space-y-2">
						{#each Object.entries(scoringConfig!.dimension_weights) as [dim, weight] (dim)}
							<div class="flex items-center gap-2">
								<span class="flex-1 text-sm" style="color: var(--color-foreground)">{dim}</span>
								<input
									type="number"
									step="0.05"
									value={weight}
									oninput={(e) => {
										const dw = { ...scoringConfig!.dimension_weights };
										dw[dim] = parseFloat(e.currentTarget.value) || 0;
										scoringConfig = { ...scoringConfig!, dimension_weights: dw };
									}}
									class="w-20 rounded border px-2 py-1 text-xs"
									style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
								/>
							</div>
						{/each}
					</div>
				</div>

				<div>
					<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('app.tools.curated_roadmap', 'Curated Roadmap (JSON)')}
					</p>
					<JsonValueEditor bind:value={curatedRoadmapText} />
					<p class="mt-1 text-xs" style="color: var(--color-muted)">
						{t('app.tools.curated_roadmap_hint', 'Map of dimension -> ordered roadmap steps. Must be valid JSON.')}
					</p>
				</div>

				<button
					type="button"
					onclick={saveFormConfig}
					disabled={configSaving}
					class="w-full rounded-lg px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					style="background: var(--color-primary); color: var(--color-primary-contrast, white)"
				>
					{configSaving ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
				</button>
			</div>
		</div>
	</div>
{/if}
