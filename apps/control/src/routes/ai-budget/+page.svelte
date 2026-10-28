<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import Tabs from '$lib/components/ui/Tabs.svelte';
	import TabsContent from '$lib/components/ui/TabsContent.svelte';
	import { api } from '$lib/api';
	import type { AiBudget } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { DollarSign, RefreshCw, AlertTriangle, Pencil } from 'lucide-svelte';
	import { BarChart, LineChart } from 'layerchart';
	import { curveMonotoneX } from 'd3-shape';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';

	let {} = $props();

	// Backend routes gated by ai.budget.view (GET) and ai.budget.manage
	// (PATCH) — migration 000452, server.go:6089-6093. The frontend must
	// match so users without view access see an access-denied message
	// instead of triggering 403 API calls.
	const canView = $derived(hasAnyPermission(['ai.budget.view', 'ai.budget.manage']));
	const canManage = $derived(hasAnyPermission(['ai.budget.manage']));

	let budget = $state<AiBudget | null>(null);
	let loading = $state(true);
	let loadError = $state('');
	let activeTab = $state('overview');
	let editModalOpen = $state(false);
	let editThreshold = $state('');
	let saving = $state(false);
	let editError = $state('');

	async function load(quiet = false) {
		if (!canView) {
			loading = false;
			return;
		}
		if (!quiet) loading = true;
		const result = await api<AiBudget>('/control/ai-budget');
		if (result.ok) {
			budget = result.data;
			loadError = '';
		} else {
			loadError = result.error;
		}
		loading = false;
	}

	$effect(() => {
		if (!canView) {
			loading = false;
			return;
		}
		load();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') load(true);
		}, 30_000);
		return () => clearInterval(interval);
	});

	function formatCost(usd: number): string {
		return '$' + (usd ?? 0).toFixed(2);
	}

	function budgetPercent(): number {
		if (!budget || !budget.budget_threshold_usd || budget.budget_threshold_usd <= 0) return 0;
		return Math.round((budget.month_to_date_cost / budget.budget_threshold_usd) * 100);
	}

	// M8: Chart data derived from the budget response
	const providerBarData = $derived.by(() => {
		if (!budget?.by_provider) return [];
		return budget.by_provider.map((p) => ({
			key: p.provider,
			label: p.provider,
			value: p.cost_usd
		}));
	});

	const taskBarData = $derived.by(() => {
		if (!budget?.by_task_type) return [];
		return budget.by_task_type.map((t) => ({
			key: t.task_type,
			label: t.task_type,
			value: t.cost_usd
		}));
	});

	// Cumulative spend spline — approximated from the by_provider data
	// sorted by cost descending, showing cumulative accumulation.
	const cumulativeData = $derived.by(() => {
		if (!budget?.by_provider) return [];
		const sorted = [...budget.by_provider].sort((a, b) => b.cost_usd - a.cost_usd);
		let cumulative = 0;
		return sorted.map((p, i) => {
			cumulative += p.cost_usd;
			return { x: i, y: cumulative, label: p.provider };
		});
	});

	function openEditModal() {
		editThreshold = budget ? String(budget.budget_threshold_usd ?? 0) : '0';
		editError = '';
		editModalOpen = true;
	}

	async function saveThreshold() {
		if (!String(editThreshold).trim()) {
			editError = t('ai_budget.invalid_threshold', 'Threshold must be a non-negative number.');
			return;
		}
		const val = Number(editThreshold);
		if (Number.isNaN(val) || val < 0) {
			editError = t('ai_budget.invalid_threshold', 'Threshold must be a non-negative number.');
			return;
		}
		saving = true;
		editError = '';
		try {
			const result = await api<AiBudget>('/control/ai-budget', {
				method: 'PATCH',
				body: { budget_threshold_usd: val }
			});
			if (result.ok) {
				budget = result.data;
				editModalOpen = false;
			} else {
				editError = result.error || t('ai_budget.save_error', 'Failed to update budget threshold.');
			}
		} catch {
			editError = t('ai_budget.save_error', 'Failed to update budget threshold.');
		} finally {
			saving = false;
		}
	}
</script>

<PageHeader title={t('ai_budget.title', 'AI Budget')} description={t('ai_budget.description', 'Monitor AI costs and budget limits')} />

{#if !canView}
	<div class="flex items-center gap-2 rounded-lg border p-4 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
		<AlertTriangle class="size-4" />
		{t('ai_budget.no_permission', 'You do not have permission to view the AI budget dashboard.')}
	</div>
{:else}
<div class="mb-4 flex items-center justify-end gap-2">
	{#if canManage}
		<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors" onclick={openEditModal}>
			<Pencil class="size-3" />
			{t('ai_budget.edit_threshold', 'Edit Threshold')}
		</button>
	{/if}
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors" onclick={() => load()}>
		<RefreshCw class="size-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="text-muted-foreground py-8 text-center text-sm">{t('ai_budget.loading', 'Loading...')}</p>
{:else if loadError}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
		<AlertTriangle class="size-4" />
		{loadError}
	</div>
{:else if budget}
		{#if budget.alert_triggered}
		<div class="border-destructive bg-destructive/10 text-destructive mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
			<AlertTriangle class="size-4" />
			{t('ai_budget.threshold_exceeded', 'Budget threshold exceeded! Current spend:')} {formatCost(budget.month_to_date_cost)} / {formatCost(budget.budget_threshold_usd)}
		</div>
	{/if}

	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		<MetricCard label={t('ai_budget.month_to_date')} value={formatCost(budget.month_to_date_cost)} />
		<MetricCard label={t('ai_budget.budget_threshold')} value={formatCost(budget.budget_threshold_usd)} />
		<MetricCard label={t('ai_budget.budget_used')} value="{budgetPercent()}%" trend={budgetPercent() > 80 ? 'down' : 'neutral'} />
		<div class="border-border-card bg-background-alt shadow-card rounded-card p-4">
			<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">{t('ai_budget.budget_progress')}</p>
			<div class="bg-muted mt-3 h-3 w-full rounded-full">
				<div
					class="h-full rounded-full transition-all"
					class:bg-emerald-500={budgetPercent() < 60}
					class:bg-tertiary={budgetPercent() >= 60 && budgetPercent() < 80}
					class:bg-destructive={budgetPercent() >= 80}
					style="width: {Math.min(budgetPercent(), 100)}%"
				></div>
			</div>
			<p class="text-muted-foreground mt-1 text-[10px]">{budgetPercent()}% {t('ai_budget.percent_of_threshold')}</p>
		</div>
	</div>

	<Tabs
		bind:value={activeTab}
		tabs={[
			{ value: 'overview', label: t('ai_budget.tab_overview') },
			{ value: 'by-provider', label: `${t('ai_budget.cost_by_provider')} (${(budget.by_provider ?? []).length})` },
			{ value: 'by-task', label: `${t('ai_budget.cost_by_task')} (${(budget.by_task_type ?? []).length})` }
		]}
	>
		<TabsContent value="overview">
			<!-- M8: Daily spend BarChart + cumulative Spline -->
			{#if providerBarData.length > 0}
				<div class="grid gap-4 lg:grid-cols-2 mb-4">
					<div class="border-border-card bg-background-alt shadow-card rounded-xl p-5">
						<h3 class="text-muted-foreground mb-4 text-xs font-semibold uppercase tracking-wider">{t('ai_budget.spend_by_provider', 'Spend by Provider')}</h3>
						<div class="h-[200px]">
							<BarChart
								data={providerBarData}
								x="value"
								y="label"
								orientation="horizontal"
								grid={false}
								xPadding={[0.2, 0.2]}
								cRange={['var(--color-accent-foreground)']}
							/>
						</div>
					</div>
					<div class="border-border-card bg-background-alt shadow-card rounded-xl p-5">
						<h3 class="text-muted-foreground mb-4 text-xs font-semibold uppercase tracking-wider">{t('ai_budget.cumulative_spend', 'Cumulative Spend')}</h3>
						<div class="h-[200px]">
							<LineChart
								data={cumulativeData}
								x="x"
								y="y"
								props={{ spline: { curve: curveMonotoneX, stroke: 'var(--color-accent-foreground)' } }}
								axis={false}
								grid={false}
								rule={false}
							/>
						</div>
					</div>
				</div>
			{/if}
			<div class="border-border-card bg-background-alt shadow-card rounded-xl p-6">
				<h3 class="text-foreground text-sm font-semibold">{t('ai_budget.budget_summary')}</h3>
				<div class="mt-4 space-y-3">
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground text-sm">{t('ai_budget.total_spend')}</span>
						<span class="text-foreground font-mono text-sm font-medium">{formatCost(budget.month_to_date_cost)}</span>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground text-sm">{t('ai_budget.budget_threshold')}</span>
						<span class="text-foreground font-mono text-sm font-medium">{formatCost(budget.budget_threshold_usd)}</span>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground text-sm">{t('ai_budget.remaining')}</span>
						<span class="font-mono text-sm font-medium" class:text-destructive={budget.month_to_date_cost >= budget.budget_threshold_usd} class:text-emerald-500={budget.month_to_date_cost < budget.budget_threshold_usd}>
							{formatCost(Math.max(0, (budget.budget_threshold_usd ?? 0) - (budget.month_to_date_cost ?? 0)))}
						</span>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground text-sm">{t('ai_budget.alert_status')}</span>
						<span class="rounded-full px-2 py-0.5 text-xs font-medium {budget.alert_triggered ? 'bg-destructive/15 text-destructive' : 'bg-emerald-500/15 text-emerald-600'}">
							{budget.alert_triggered ? t('ai_budget.alert_triggered') : t('ai_budget.within_budget')}
						</span>
					</div>
				</div>
			</div>
		</TabsContent>

		<TabsContent value="by-provider">
			{#if budget.by_provider && budget.by_provider.length > 0}
				<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
					<div class="border-border-card border-b px-4 py-3">
						<h3 class="text-foreground text-sm font-semibold">{t('ai_budget.cost_by_provider')}</h3>
					</div>
					<table class="w-full text-sm">
						<thead>
							<tr class="border-border-card border-b">
								<th class="text-muted-foreground px-4 py-3 text-start font-medium">{t('ai_budget.provider')}</th>
								<th class="text-muted-foreground px-4 py-3 text-end font-medium">{t('ai_budget.tokens')}</th>
								<th class="text-muted-foreground px-4 py-3 text-end font-medium">{t('ai_budget.cost')}</th>
								<th class="text-muted-foreground px-4 py-3 text-start font-medium">{t('ai_budget.share')}</th>
							</tr>
						</thead>
						<tbody>
							{#each budget.by_provider as p}
								{@const totalTokens = budget.by_provider.reduce((sum, x) => sum + x.tokens_used, 0)}
								<tr class="border-border-card border-t">
									<td class="px-4 py-3 font-medium">{p.provider}</td>
									<td class="px-4 py-3 text-end font-mono text-xs">{p.tokens_used.toLocaleString()}</td>
									<td class="px-4 py-3 text-end font-mono text-xs">{formatCost(p.cost_usd)}</td>
									<td class="px-4 py-3">
										<div class="bg-muted h-2 w-full max-w-[100px] rounded-full">
											<div class="bg-foreground h-full rounded-full" style="width: {totalTokens > 0 ? (p.tokens_used / totalTokens) * 100 : 0}%"></div>
										</div>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{:else}
				<p class="text-muted-foreground py-8 text-center text-sm">{t('ai_budget.no_provider_data')}</p>
			{/if}
		</TabsContent>

		<TabsContent value="by-task">
			{#if budget.by_task_type && budget.by_task_type.length > 0}
				<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
					<div class="border-border-card border-b px-4 py-3">
						<h3 class="text-foreground text-sm font-semibold">{t('ai_budget.cost_by_task')}</h3>
					</div>
					<table class="w-full text-sm">
						<thead>
							<tr class="border-border-card border-b">
								<th class="text-muted-foreground px-4 py-3 text-start font-medium">{t('ai_budget.task_type')}</th>
								<th class="text-muted-foreground px-4 py-3 text-end font-medium">{t('ai_budget.calls')}</th>
								<th class="text-muted-foreground px-4 py-3 text-end font-medium">{t('ai_budget.tokens')}</th>
								<th class="text-muted-foreground px-4 py-3 text-end font-medium">{t('ai_budget.cost')}</th>
							</tr>
						</thead>
						<tbody>
							{#each budget.by_task_type as task}
								<tr class="border-border-card border-t">
									<td class="px-4 py-3 font-medium">{task.task_type}</td>
									<td class="px-4 py-3 text-end font-mono text-xs">{task.call_count.toLocaleString()}</td>
									<td class="px-4 py-3 text-end font-mono text-xs">{task.tokens_used.toLocaleString()}</td>
									<td class="px-4 py-3 text-end font-mono text-xs">{formatCost(task.cost_usd)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{:else}
				<p class="text-muted-foreground py-8 text-center text-sm">{t('ai_budget.no_task_data')}</p>
			{/if}
		</TabsContent>
	</Tabs>
{:else}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<DollarSign class="text-muted-foreground mx-auto size-8" />
		<p class="text-muted-foreground mt-3 text-sm">{t('ai_budget.no_data')}</p>
	</div>
{/if}

<Modal bind:open={editModalOpen} title={t('ai_budget.edit_threshold', 'Edit Budget Threshold')} description={t('ai_budget.edit_threshold_desc', 'Set the monthly AI spend alert threshold in USD.')}>
			<div class="mt-4">
				<label for="threshold-input" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_budget.budget_threshold')}</label>
				<p class="text-muted-foreground mt-1 text-xs">{t('ai_budget.threshold_hint', 'Monthly AI spend cap — usage alerts fire near it.')}</p>
				<input
					id="threshold-input"
					type="number"
					min="0"
					step="0.01"
					bind:value={editThreshold}
					class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
					placeholder="0.00"
				/>
				{#if editError}
					<p class="text-destructive mt-2 text-xs">{editError}</p>
				{/if}
			</div>

			<div class="mt-6 flex justify-end gap-2">
				<button
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={() => (editModalOpen = false)}
				>
					{t('common.cancel')}
				</button>
				<button
					class="bg-foreground text-background hover:bg-foreground/90 rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={saving}
					onclick={saveThreshold}
				>
					{saving ? t('common.saving') : t('common.save')}
				</button>
			</div>
</Modal>
{/if}
