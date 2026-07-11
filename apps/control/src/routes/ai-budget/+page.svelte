<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import { api } from '$lib/api';
	import type { AiBudget } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { DollarSign, RefreshCw, AlertTriangle } from 'lucide-svelte';

	let budget = $state<AiBudget | null>(null);
	let loading = $state(true);

	onMount(load);

	async function load() {
		loading = true;
		const result = await api<AiBudget>('/control/ai-budget');
		if (result.ok) {
			budget = result.data;
		}
		loading = false;
	}

	function formatCost(usd: number): string {
		return '$' + usd.toFixed(2);
	}
</script>

<PageHeader title={t('ai_budget.title')} description={t('ai_budget.description')} />

<div class="mb-4 flex items-center justify-end">
	<button class="btn btn-secondary text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else if budget}
	{#if budget.alert_triggered}
		<div class="mb-4 flex items-center gap-2 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			<AlertTriangle class="h-4 w-4" />
			Budget threshold exceeded! Current spend: {formatCost(budget.month_to_date_cost)} / {formatCost(budget.budget_threshold_usd)}
		</div>
	{/if}

	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
		<MetricCard label="Month-to-Date Cost" value={formatCost(budget.month_to_date_cost)} />
		<MetricCard label="Budget Threshold" value={formatCost(budget.budget_threshold_usd)} />
		<MetricCard label="Budget Used" value="{Math.round((budget.month_to_date_cost / Math.max(budget.budget_threshold_usd, 1)) * 100)}%" />
	</div>

	{#if budget.by_provider.length > 0}
		<div class="mb-6 rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border)">
				<h3 class="text-sm font-semibold">Cost by Provider</h3>
			</div>
			<table class="w-full text-sm">
				<thead>
					<tr style="border-bottom: 1px solid var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Provider</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">Tokens</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">Cost</th>
					</tr>
				</thead>
				<tbody>
					{#each budget.by_provider as p}
						<tr style="border-bottom: 1px solid var(--color-border)">
							<td class="px-4 py-3 font-medium">{p.provider}</td>
							<td class="px-4 py-3 text-right font-mono text-xs">{p.tokens_used.toLocaleString()}</td>
							<td class="px-4 py-3 text-right font-mono text-xs">{formatCost(p.cost_usd)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	{#if budget.by_task_type.length > 0}
		<div class="rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border)">
				<h3 class="text-sm font-semibold">Cost by Task Type</h3>
			</div>
			<table class="w-full text-sm">
				<thead>
					<tr style="border-bottom: 1px solid var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Task Type</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">Calls</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">Tokens</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">Cost</th>
					</tr>
				</thead>
				<tbody>
					{#each budget.by_task_type as t}
						<tr style="border-bottom: 1px solid var(--color-border)">
							<td class="px-4 py-3 font-medium">{t.task_type}</td>
							<td class="px-4 py-3 text-right font-mono text-xs">{t.call_count.toLocaleString()}</td>
							<td class="px-4 py-3 text-right font-mono text-xs">{t.tokens_used.toLocaleString()}</td>
							<td class="px-4 py-3 text-right font-mono text-xs">{formatCost(t.cost_usd)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
{:else}
	<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<DollarSign class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
		<p class="mt-3 text-sm" style="color: var(--color-muted)">No AI budget data available.</p>
	</div>
{/if}
