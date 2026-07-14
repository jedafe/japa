<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import { Tabs } from 'bits-ui';
	import { api } from '$lib/api';
	import type { AiBudget } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { DollarSign, RefreshCw, AlertTriangle } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let budget = $state<AiBudget | null>(data.budget ?? null);
	let loading = $state(false);
	let activeTab = $state('overview');

	async function load() {
		loading = true;
		const result = await api<AiBudget>('/control/ai-budget');
		if (result.ok) {
			budget = result.data;
		}
		loading = false;
	}

	function formatCost(usd: number): string {
		return '$' + (usd ?? 0).toFixed(2);
	}

	function budgetPercent(): number {
		if (!budget) return 0;
		return Math.round((budget.month_to_date_cost / Math.max(budget.budget_threshold_usd, 1)) * 100);
	}
</script>

<PageHeader title={t('ai_budget.title', 'AI Budget')} description={t('ai_budget.description', 'Monitor AI costs and budget limits')} />

<div class="mb-4 flex items-center justify-end">
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors" onclick={load}>
		<RefreshCw class="size-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="text-muted-foreground py-8 text-center text-sm">Loading...</p>
{:else if budget}
	{#if budget.alert_triggered}
		<div class="border-destructive bg-destructive/10 text-destructive mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
			<AlertTriangle class="size-4" />
			Budget threshold exceeded! Current spend: {formatCost(budget.month_to_date_cost)} / {formatCost(budget.budget_threshold_usd)}
		</div>
	{/if}

	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		<MetricCard label="Month-to-Date Cost" value={formatCost(budget.month_to_date_cost)} />
		<MetricCard label="Budget Threshold" value={formatCost(budget.budget_threshold_usd)} />
		<MetricCard label="Budget Used" value="{budgetPercent()}%" trend={budgetPercent() > 80 ? 'down' : 'neutral'} />
		<div class="border-border-card bg-background-alt shadow-card rounded-card p-4">
			<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">Budget Progress</p>
			<div class="bg-muted mt-3 h-3 w-full rounded-full">
				<div
					class="h-full rounded-full transition-all"
					class:bg-green-500={budgetPercent() < 60}
					class:bg-tertiary={budgetPercent() >= 60 && budgetPercent() < 80}
					class:bg-destructive={budgetPercent() >= 80}
					style="width: {Math.min(budgetPercent(), 100)}%"
				></div>
			</div>
			<p class="text-muted-foreground mt-1 text-[10px]">{budgetPercent()}% of threshold</p>
		</div>
	</div>

	<Tabs.Root bind:value={activeTab}>
		<Tabs.List class="bg-muted inline-flex gap-1 rounded-9px p-1">
			<Tabs.Trigger value="overview" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
				Overview
			</Tabs.Trigger>
			<Tabs.Trigger value="by-provider" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
				By Provider ({(budget.by_provider ?? []).length})
			</Tabs.Trigger>
			<Tabs.Trigger value="by-task" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
				By Task Type ({(budget.by_task_type ?? []).length})
			</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="overview" class="mt-4">
			<div class="border-border-card bg-background-alt shadow-card rounded-xl p-6">
				<h3 class="text-foreground text-sm font-semibold">Budget Summary</h3>
				<div class="mt-4 space-y-3">
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground text-sm">Total Spend</span>
						<span class="text-foreground font-mono text-sm font-medium">{formatCost(budget.month_to_date_cost)}</span>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground text-sm">Threshold</span>
						<span class="text-foreground font-mono text-sm font-medium">{formatCost(budget.budget_threshold_usd)}</span>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground text-sm">Remaining</span>
						<span class="font-mono text-sm font-medium" class:text-destructive={budget.month_to_date_cost >= budget.budget_threshold_usd} class:text-green-500={budget.month_to_date_cost < budget.budget_threshold_usd}>
							{formatCost(Math.max(0, budget.budget_threshold_usd - budget.month_to_date_cost))}
						</span>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground text-sm">Alert Status</span>
						<span class="rounded-full px-2 py-0.5 text-xs font-medium {budget.alert_triggered ? 'bg-destructive/15 text-destructive' : 'bg-green-500/15 text-green-600'}">
							{budget.alert_triggered ? 'Alert Triggered' : 'Within Budget'}
						</span>
					</div>
				</div>
			</div>
		</Tabs.Content>

		<Tabs.Content value="by-provider" class="mt-4">
			{#if budget.by_provider && budget.by_provider.length > 0}
				<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
					<div class="border-border-card border-b px-4 py-3">
						<h3 class="text-foreground text-sm font-semibold">Cost by Provider</h3>
					</div>
					<table class="w-full text-sm">
						<thead>
							<tr class="border-border-card border-b">
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">Provider</th>
								<th class="text-muted-foreground px-4 py-3 text-right font-medium">Tokens</th>
								<th class="text-muted-foreground px-4 py-3 text-right font-medium">Cost</th>
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">Share</th>
							</tr>
						</thead>
						<tbody>
							{#each budget.by_provider as p}
								{@const totalTokens = budget.by_provider.reduce((sum, x) => sum + x.tokens_used, 0)}
								<tr class="border-border-card border-t">
									<td class="px-4 py-3 font-medium">{p.provider}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{p.tokens_used.toLocaleString()}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{formatCost(p.cost_usd)}</td>
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
				<p class="text-muted-foreground py-8 text-center text-sm">No provider data available.</p>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="by-task" class="mt-4">
			{#if budget.by_task_type && budget.by_task_type.length > 0}
				<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
					<div class="border-border-card border-b px-4 py-3">
						<h3 class="text-foreground text-sm font-semibold">Cost by Task Type</h3>
					</div>
					<table class="w-full text-sm">
						<thead>
							<tr class="border-border-card border-b">
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">Task Type</th>
								<th class="text-muted-foreground px-4 py-3 text-right font-medium">Calls</th>
								<th class="text-muted-foreground px-4 py-3 text-right font-medium">Tokens</th>
								<th class="text-muted-foreground px-4 py-3 text-right font-medium">Cost</th>
							</tr>
						</thead>
						<tbody>
							{#each budget.by_task_type as task}
								<tr class="border-border-card border-t">
									<td class="px-4 py-3 font-medium">{task.task_type}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{task.call_count.toLocaleString()}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{task.tokens_used.toLocaleString()}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{formatCost(task.cost_usd)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{:else}
				<p class="text-muted-foreground py-8 text-center text-sm">No task type data available.</p>
			{/if}
		</Tabs.Content>
	</Tabs.Root>
{:else}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<DollarSign class="text-muted-foreground mx-auto size-8" />
		<p class="text-muted-foreground mt-3 text-sm">No AI budget data available.</p>
	</div>
{/if}
