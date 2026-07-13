<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import type { PageData } from './$types';

	type ServiceStatus = {
		name: string;
		status: 'healthy' | 'degraded' | 'down';
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
			items: items.map((s) => ({ name: s.name, status: statusToHealth(s.status) }))
		});
		groups = [
			mapGroup(t('dashboard.groups.infra'), health.infrastructure),
			mapGroup(t('dashboard.groups.app'), health.application),
			mapGroup(t('dashboard.groups.background'), health.background),
			mapGroup(t('dashboard.groups.realtime'), health.realtime)
		];
		lastRefresh = new Date().toLocaleTimeString();
		apiAvailable = true;
	}

	let groups = $state<{ name: string; items: { name: string; status: 'healthy' | 'warning' | 'critical' | 'unknown' }[] }[]>([
		{
			name: t('dashboard.groups.infra'),
			items: [
				{ name: 'PostgreSQL', status: 'unknown' },
				{ name: 'Valkey', status: 'unknown' },
				{ name: 'Meilisearch', status: 'unknown' },
				{ name: 'SeaweedFS', status: 'unknown' }
			]
		},
		{
			name: t('dashboard.groups.app'),
			items: [
				{ name: 'API Server', status: 'unknown' },
				{ name: 'imgproxy', status: 'unknown' },
				{ name: 'Nginx', status: 'unknown' }
			]
		},
		{
			name: t('dashboard.groups.background'),
			items: [
				{ name: 'Asynq Workers', status: 'unknown' },
				{ name: 'Queue Depth', status: 'unknown' }
			]
		},
		{
			name: t('dashboard.groups.realtime'),
			items: [
				{ name: 'WebSocket Hub', status: 'unknown' },
				{ name: 'SSE Connections', status: 'unknown' }
			]
		}
	]);

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
	<div class="animate-fade-in mb-4 rounded-xl border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		{t('dashboard.analytics_warning')}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<p class="text-xs font-medium" style="color: var(--color-muted)">{t('dashboard.last_refresh').replace('{time}', lastRefresh)}</p>
	<button
		class="btn btn-secondary text-xs"
		onclick={refresh}
	>
		{t('common.refresh')}
	</button>
</div>

<!-- Health groups -->
<div class="grid gap-4 sm:grid-cols-2">
	{#each groups as group, i}
		<div
			class="rounded-2xl border p-5"
			style="border-color: var(--color-border); background: var(--gradient-surface); animation: slide-up 0.3s ease-out {i * 50}ms both"
		>
			<h3 class="mb-4 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{group.name}</h3>
			<div class="space-y-2.5">
				{#each group.items as item}
					<div class="flex items-center justify-between">
						<span class="text-sm font-medium" style="color: var(--color-foreground)">{item.name}</span>
						<StatusBadge status={item.status} />
					</div>
				{/each}
			</div>
		</div>
	{/each}
</div>
