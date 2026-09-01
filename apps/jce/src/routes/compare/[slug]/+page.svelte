<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		deleteComparison,
		getFreshness,
		promoteComparison,
		scoreComparison
	} from '$lib/api/jce';
	import { t } from '$lib/i18n';
	import type {
		ComparisonProfile,
		FreshnessCell,
		FreshnessSummary,
		JCEDimension,
		JapaScore,
		MatrixDimension,
		MatrixItem,
		ResolvedItem,
		ScoreResult
	} from '$lib/types/jce';
	import type { PageData } from './$types';
	import ScoreBadge from '$lib/components/compare/ScoreBadge.svelte';
	import ScoreExplanation from '$lib/components/compare/ScoreExplanation.svelte';
	import RecommendationPanel from '$lib/components/compare/RecommendationPanel.svelte';
	import ProfileDrawer from '$lib/components/compare/ProfileDrawer.svelte';
	import SmartToggle from '$lib/components/compare/SmartToggle.svelte';
	import FreshnessChip from '$lib/components/compare/FreshnessChip.svelte';
	import RefreshButton from '$lib/components/compare/RefreshButton.svelte';
	import AIAdvisorDrawer from '$lib/components/compare/AIAdvisorDrawer.svelte';
	import { ArrowLeft, Pencil, Sparkles, Trash2, ExternalLink, RefreshCw, User } from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	let comparison = $derived(data.comparison);
	let matrix = $derived(data.matrix ?? []);
	let resolvedItems = $derived(data.resolvedItems ?? []);
	let isOwner = $derived(data.isOwner ?? false);
	let mainOrigin = $derived(data.mainOrigin ?? 'http://localhost:3000');
	let typeSlug = $derived(data.typeSlug ?? '');
	let user = $derived(data.user ?? null);
	let profiles = $derived(data.profiles ?? []);
	let features = $derived((data.features as Record<string, boolean> | undefined) ?? {});

	let freshness = $state<FreshnessSummary | null>(null);
	let advisorOpen = $state(false);

	let smartMode = $state('none');
	$effect.pre(() => {
		smartMode = data.comparison.metadata.smart_mode ?? 'none';
	});
	$effect(() => {
		const mode = scoreResult?.smart_mode;
		if (mode && mode !== smartMode) {
			smartMode = mode;
		}
	});
	let activeProfile = $state<ComparisonProfile | null>(null);
	let profileOpen = $state(false);
	let scoring = $state(false);

	$effect.pre(() => {
		if (activeProfile === null) {
			activeProfile = data.defaultProfile ?? null;
		}
	});

	$effect(() => {
		if (comparison?.content?.id && user && features['feature.jce']) {
			getFreshness(comparison.content.id).then((f) => {
				freshness = f;
			});
		}
	});

	let refreshedScore = $state<ScoreResult | null>(null);

	$effect.pre(() => {
		// Reset any client-side refresh when the server data changes (e.g. navigation).
		data.comparison.content.id;
		refreshedScore = null;
	});

	let scoreResult = $derived(refreshedScore ?? data.scoreResult ?? null);
	let recommendations = $derived(
		refreshedScore ? (refreshedScore.recommendations ?? []) : (data.recommendations ?? [])
	);

	let bestItemId = $derived.by(() => {
		if (!scoreResult || scoreResult.scores.length === 0) return null;
		const eligible = scoreResult.scores.filter((s) => s.eligible_status !== 'ineligible');
		if (eligible.length === 0) return null;
		const top = scoreResult.recommendation_summary?.top_item_id;
		if (top) {
			const topScore = eligible.find((s) => s.item_id === top);
			if (topScore) return top;
		}
		return eligible.reduce((best, s) => (s.japa_score > best.japa_score ? s : best), eligible[0]).item_id;
	});

	let sortedItems = $derived(
		scoreResult
			? matrix.slice().sort((a, b) => {
					const scoreA = scoreResult?.scores.find((s) => s.item_id === a.id);
					const scoreB = scoreResult?.scores.find((s) => s.item_id === b.id);
					const ineligibleA = scoreA?.eligible_status === 'ineligible' ? 1 : 0;
					const ineligibleB = scoreB?.eligible_status === 'ineligible' ? 1 : 0;
					if (ineligibleA !== ineligibleB) return ineligibleA - ineligibleB;
					const sa = scoreA?.japa_score ?? 0;
					const sb = scoreB?.japa_score ?? 0;
					return sb - sa;
				})
			: matrix
	);

	let deleting = $state(false);
	let promoting = $state(false);
	let error = $state('');

	async function remove() {
		if (!isOwner) return;
		deleting = true;
		error = '';
		try {
			await deleteComparison(comparison.content.id);
			goto('/compare');
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.compare.errors.delete_failed');
		} finally {
			deleting = false;
		}
	}

	async function promote() {
		if (!isOwner) return;
		promoting = true;
		error = '';
		try {
			const promoted = await promoteComparison(comparison.content.id);
			if (promoted?.id) {
				// Ref: sections/30-generic-comparison-system.md (JCE promotion to detailed composer)
				await goto(`${mainOrigin}/create?type=comparison&draft=${promoted.id}`);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.compare.errors.promote_failed');
		} finally {
			promoting = false;
		}
	}

	async function refreshScore(mode?: string, force = false) {
		if (!user) return;
		scoring = true;
		error = '';
		const res = await scoreComparison(comparison.content.id, activeProfile?.id, mode, force);
		if (res) {
			refreshedScore = res;
		} else {
			const unknownError = t('jce.compare.errors.unknown');
			error = t('jce.compare.errors.load_score', unknownError, { message: unknownError });
		}
		scoring = false;
	}

	function edit() {
		const ids = resolvedItems.map((i: ResolvedItem) => i.id).join(',');
		const url = new URL('/compare', page.url);
		if (typeSlug) url.searchParams.set('type', typeSlug);
		url.searchParams.set('preselect', ids);
		goto(url);
	}

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}

	function findCell(itemId: string, dimId: string): FreshnessCell | undefined {
		const item = freshness?.by_item.find((i) => i.item_id === itemId);
		return item?.cells.find((c) => c.dimension_id === dimId);
	}

	function formatValue(cell?: MatrixDimension) {
		if (!cell) return '—';
		if (cell.value !== undefined && cell.value !== null) return cell.value;
		if (cell.value_rating !== undefined && cell.value_rating !== null) return cell.value_rating;
		if (cell.value_boolean !== undefined && cell.value_boolean !== null) return cell.value_boolean ? t('jce.common.yes') : t('jce.common.no');
		return '—';
	}

	function scoreFor(item: MatrixItem): JapaScore | undefined {
		return scoreResult?.scores.find((s) => s.item_id === item.id);
	}
</script>

<div class="max-w-5xl mx-auto p-4 sm:p-6 space-y-6">
	<div class="flex items-start justify-between gap-4 flex-wrap">
		<div class="space-y-1">
			<a
				href="/compare"
				class="inline-flex items-center gap-1 text-sm transition-colors hover:opacity-80"
				style="color: var(--color-muted)"
			>
				<ArrowLeft size={14} /> {t('jce.compare.back')}
			</a>
			<h1 class="text-2xl sm:text-3xl font-bold" style="color: var(--color-text)">{comparison.content.title}</h1>
			<p class="text-sm capitalize" style="color: var(--color-muted)">
				{comparison.metadata.depth} · {comparison.content.status} · {comparison.content.visibility}
			</p>
		</div>
		{#if isOwner}
			<div class="flex flex-wrap gap-2">
				<button
					type="button"
					class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium transition-colors"
					style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
					onclick={edit}
				>
					<Pencil size={12} /> {t('jce.compare.edit_selection')}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium transition-colors disabled:opacity-50"
					style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
					disabled={promoting}
					onclick={promote}
				>
					<Sparkles size={12} /> {promoting ? t('jce.common.promoting') : t('jce.compare.promote_to_detailed')}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium transition-colors disabled:opacity-50"
					style="background-color: rgba(239,68,68,0.1); color: #ef4444"
					disabled={deleting}
					onclick={remove}
				>
					<Trash2 size={12} /> {deleting ? t('jce.common.deleting') : t('jce.common.delete')}
				</button>
			</div>
		{/if}
	</div>

	{#if error}
		<div class="rounded-[var(--radius-button)] p-4 text-sm" role="alert" style="background-color: rgba(239,68,68,0.1); color: #ef4444; border: 1px solid rgba(239,68,68,0.2)">
			{t(error)}
		</div>
	{/if}

	<section>
		<h2 class="text-lg font-semibold mb-3" style="color: var(--color-text)">{t('jce.compare.items_heading')}</h2>
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each resolvedItems as item (item.id)}
				<div
					class="rounded-[var(--radius-card)] border p-4 space-y-2"
					style="background-color: var(--color-surface); border-color: var(--color-border)"
				>
					<h3 class="font-medium" style="color: var(--color-text)">{item.title}</h3>
					{#if item.description}
						<p class="text-sm" style="color: var(--color-muted)">{item.description}</p>
					{/if}
					{#if item.url}
						<a
							href={item.url}
							target="_blank"
							class="inline-flex items-center gap-1 text-xs underline"
							style="color: var(--color-text)"
						>
							<ExternalLink size={12} /> {t('jce.common.view')}
						</a>
					{/if}
				</div>
			{/each}
		</div>
	</section>

	{#if user}
		<section>
			<div class="flex flex-wrap items-center gap-3 mb-3">
				<SmartToggle bind:mode={smartMode} modes={data.smartModes} onchange={(m) => refreshScore(m, true)} />

				<button
					type="button"
					class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium transition-colors"
					style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
					onclick={() => (profileOpen = true)}
				>
					<User size={12} />
					{activeProfile?.title ?? t('jce.profile.select')}
				</button>

				{#if smartMode !== 'none'}
					<button
						type="button"
						class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium transition-colors disabled:opacity-50"
						style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
						disabled={scoring}
						onclick={() => refreshScore(smartMode, true)}
					>
						<RefreshCw size={12} />
						{scoring ? t('jce.compare.score.loading') : t('jce.compare.score.refresh')}
					</button>
				{/if}
			</div>

			<ProfileDrawer
				bind:open={profileOpen}
				{profiles}
				bind:activeProfile
				scoreFactors={data.scoreFactors ?? []}
			allowedGoals={data.allowedGoals ?? []}
				onsave={() => refreshScore(smartMode, true)}
			/>
		</section>
	{/if}

	{#if sortedItems.length > 0}
		<section>
			<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
				<h2 class="text-lg font-semibold" style="color: var(--color-text)">{t('jce.compare.matrix_heading')}</h2>
				<div class="flex flex-wrap items-center gap-2">
					{#if user && features['feature.jce.refresh']}
						<RefreshButton comparisonID={comparison.content.id} />
					{/if}
					{#if user && features['feature.jce.ai_advisor']}
						<button
							type="button"
							class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
							style="background-color: var(--color-primary, #3b82f6); color: white"
							onclick={() => (advisorOpen = true)}
						>
							{t('jce.advisor.open', 'Ask AI Advisor')}
						</button>
					{/if}
					{#if freshness}
						<FreshnessChip summary={freshness} />
					{/if}
				</div>
			</div>
			<div class="overflow-x-auto rounded-[var(--radius-card)] border" style="border-color: var(--color-border)">
				<table class="min-w-full text-sm">
					<thead>
						<tr style="background-color: var(--color-bg); border-bottom: 1px solid var(--color-border)">
							<th class="p-3 text-left font-medium" style="color: var(--color-text)">{t('jce.compare.matrix_dimension')}</th>
							{#each sortedItems as item (item.id)}
								{@const score = scoreFor(item)}
								<th
									class="p-3 text-left font-medium"
									style="color: var(--color-text); {score?.eligible_status === 'ineligible' ? 'opacity: 0.5' : ''}"
								>
									<div class="space-y-1">
										<div>{item.title}</div>
										{#if scoreResult}
											{#if score}
												<ScoreBadge {score} best={bestItemId === item.id} />
												<ScoreExplanation {score} />
											{:else}
												<span class="text-xs" style="color: var(--color-muted)">{t('jce.score.no_data')}</span>
											{/if}
										{/if}
									</div>
								</th>
							{/each}
						</tr>
					</thead>
					<tbody class="divide-y" style="border-color: var(--color-border)">
						{#each comparison.dimensions as dim (dim.id)}
							<tr style="background-color: var(--color-surface)">
								<td class="p-3 font-medium" style="color: var(--color-text)">{dim.label}</td>
								{#each sortedItems as item (item.id)}
									{@const score = scoreFor(item)}
									<td
										class="p-3 {score?.eligible_status === 'ineligible' ? 'opacity-50' : ''}"
										style="color: var(--color-muted)"
									>
										<div class="space-y-1">
											<div>{formatValue(cellFor(item, dim))}</div>
											{#if freshness}
												<FreshnessChip cell={findCell(item.id, dim.id)} />
											{/if}
										</div>
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}

	{#if scoreResult || recommendations.length > 0}
		<RecommendationPanel {recommendations} />
	{/if}

	{#if comparison.metadata.depth === 'quick'}
		<div class="rounded-[var(--radius-card)] border p-4 text-sm" style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-muted)">
			{@html t('jce.compare.quick_comparison_notice')
				.replace('{promote}', '<strong style="color: var(--color-text)">' + t('jce.compare.promote_to_detailed') + '</strong>')}
		</div>
	{/if}

	{#if user && features['feature.jce.ai_advisor']}
		<AIAdvisorDrawer
			bind:open={advisorOpen}
			comparisonID={comparison.content.id}
			profileID={activeProfile?.id}
			items={comparison.items ?? []}
		/>
	{/if}
</div>
