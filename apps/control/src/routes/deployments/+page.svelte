<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api } from '$lib/api';
	import type { Deployment } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { Rocket, RefreshCw } from 'lucide-svelte';

	let deployments = $state<Deployment[]>([]);
	let loading = $state(true);
	let apiAvailable = $state(true);

	async function load() {
		loading = true;
		try {
			const result = await api<Deployment[]>('/control/deployments');
			if (result.ok) {
				deployments = Array.isArray(result.data) ? result.data : [];
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') load();
		}, 30_000);
		return () => clearInterval(interval);
	});

	function formatDate(iso: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return iso;
		return d.toLocaleString();
	}

	const statusClasses: Record<string, string> = {
		active: 'bg-emerald-500/15 text-emerald-600',
		rolled_back: 'bg-amber-500/15 text-amber-500',
		failed: 'bg-destructive/15 text-destructive'
	};
</script>

<PageHeader title={t('deployments.title', 'Deployments')} description={t('deployments.description', 'Current deployment version and history')} />

<div class="mb-4 flex items-center justify-end">
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors" onclick={load}>
		<RefreshCw class="size-3" />
		{t('common.refresh')}
	</button>
</div>

{#if !apiAvailable}
	<div class="border-tertiary bg-tertiary/10 text-tertiary mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
		{t('deployments.unavailable', 'Deployment information is currently unavailable.')}
	</div>
{/if}

{#if loading}
	<p class="text-muted-foreground py-8 text-center text-sm">{t('common.loading', 'Loading…')}</p>
{:else if deployments.length === 0}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<Rocket class="text-muted-foreground mx-auto size-8" />
		<p class="text-muted-foreground mt-3 text-sm">{t('deployments.no_data', 'No deployment records found.')}</p>
	</div>
{:else}
	<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-border-card border-b">
					<th class="text-muted-foreground px-4 py-3 text-left font-medium">{t('deployments.version', 'Version')}</th>
					<th class="text-muted-foreground px-4 py-3 text-left font-medium">{t('common.status', 'Status')}</th>
					<th class="text-muted-foreground px-4 py-3 text-left font-medium">{t('deployments.deployed_at', 'Deployed At')}</th>
				</tr>
			</thead>
			<tbody>
				{#each deployments as dep}
					<tr class="border-border-card border-t">
						<td class="px-4 py-3 font-mono text-xs font-medium">{dep.version}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusClasses[dep.status] ?? 'bg-muted text-foreground'}">{dep.status}</span>
						</td>
						<td class="text-muted-foreground px-4 py-3 font-mono text-xs">{formatDate(dep.deployed_at)}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
