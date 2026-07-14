<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import type { PageData } from './$types';

	type ServiceStatus = {
		name: string;
		status: 'healthy' | 'degraded' | 'down' | 'unknown';
		latency_ms: number;
		message?: string;
	};

	type HealthResponse = {
		infrastructure: ServiceStatus[];
		application: ServiceStatus[];
		background: ServiceStatus[];
		realtime: ServiceStatus[];
	};

	let { data = {} as PageData } = $props();

	function statusToHealth(s: ServiceStatus['status']): 'healthy' | 'warning' | 'critical' | 'unknown' {
		if (s === 'degraded') return 'warning';
		if (s === 'down') return 'critical';
		return s === 'healthy' ? 'healthy' : 'unknown';
	}

	function applyOverview(health: HealthResponse) {
		const mapGroup = (label: string, items: ServiceStatus[]) => ({
			name: label,
			items: items.map((s) => ({ name: s.name, status: statusToHealth(s.status), latency: s.latency_ms, message: s.message }))
		});
		groups = [
			mapGroup('dashboard.groups.infra', health.infrastructure),
			mapGroup('dashboard.groups.app', health.application),
			mapGroup('dashboard.groups.background', health.background),
			mapGroup('dashboard.groups.realtime', health.realtime)
		];
		lastRefresh = new Date().toLocaleTimeString();
		apiAvailable = true;
	}

	const groupFallbacks: Record<string, string> = {
		'dashboard.groups.infra': 'Infrastructure',
		'dashboard.groups.app': 'Application',
		'dashboard.groups.background': 'Background',
		'dashboard.groups.realtime': 'Realtime'
	};

	let groups = $state<{ name: string; items: { name: string; status: 'healthy' | 'warning' | 'critical' | 'unknown'; latency?: number; message?: string }[] }[]>([
		{
			name: 'dashboard.groups.infra',
			items: [
				{ name: 'PostgreSQL', status: 'unknown' },
				{ name: 'Valkey', status: 'unknown' },
				{ name: 'Meilisearch', status: 'unknown' },
				{ name: 'SeaweedFS', status: 'unknown' }
			]
		},
		{
			name: 'dashboard.groups.app',
			items: [
				{ name: 'API Server', status: 'unknown' },
				{ name: 'imgproxy', status: 'unknown' },
				{ name: 'Nginx', status: 'unknown' }
			]
		},
		{
			name: 'dashboard.groups.background',
			items: [
				{ name: 'Asynq Workers', status: 'unknown' },
				{ name: 'Queue Depth', status: 'unknown' }
			]
		},
		{
			name: 'dashboard.groups.realtime',
			items: [
				{ name: 'WebSocket Hub', status: 'unknown' },
				{ name: 'SSE Connections', status: 'unknown' }
			]
		}
	]);

	let displayGroups = $derived(
		groups.map((group) => ({
			...group,
			name: t(group.name, groupFallbacks[group.name] ?? group.name)
		}))
	);

	let lastRefresh = $state(new Date().toLocaleTimeString());
	let apiAvailable = $state(true);

	// svelte-ignore state_referenced_locally
	if (data.initial) {
		if (data.initial.infrastructure) {
			applyOverview(data.initial);
		}
	}

	async function refresh() {
		try {
			const result = await api<HealthResponse>('/control/health');
			if (result.ok) {
				applyOverview(result.data);
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		}
	}

	let interval: ReturnType<typeof setInterval>;
	onMount(() => {
		interval = setInterval(refresh, 30_000);
		return () => clearInterval(interval);
	});
</script>

<PageHeader title={t('dashboard.title')} description={t('dashboard.description')} />

{#if !apiAvailable}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 flex items-center gap-2 rounded-xl border px-4 py-3 text-sm">
		{t('dashboard.analytics_warning')}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<p class="text-muted-foreground text-xs font-medium">{t('dashboard.last_refresh').replace('{time}', lastRefresh)}</p>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors"
		onclick={refresh}
	>
		{t('common.refresh')}
	</button>
</div>

<div class="grid gap-4 sm:grid-cols-2">
	{#each displayGroups as group, i}
		<div
			class="border-border-card bg-background-alt shadow-card rounded-card-lg p-5"
			style="animation: fade-in 0.3s ease-out {i * 50}ms both"
		>
			<h3 class="text-muted-foreground mb-4 text-xs font-semibold uppercase tracking-wider">{group.name}</h3>
			<div class="space-y-3">
				{#each group.items as item}
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-2">
							<span class="text-foreground text-sm font-medium" title="{item.latency !== undefined ? `Latency: ${item.latency}ms` : ''}{item.message ? ` - ${item.message}` : ''}">{item.name}</span>
						</div>
						<StatusBadge status={item.status} />
					</div>
				{/each}
			</div>
		</div>
	{/each}
</div>
