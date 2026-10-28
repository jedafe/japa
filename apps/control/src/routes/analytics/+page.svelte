<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	// Native Umami dashboard — the Go API proxies /api/v1/analytics/umami/*
	// to the self-hosted Umami instance (service creds stay server-side).

	interface Website { id: string; name: string; domain: string }
	interface Metric { value: number; change: number }
	interface Stats { pageviews: Metric; visitors: Metric; visits: Metric; bounces: Metric; totaltime: Metric }
	interface Point { x: string; y: number }
	interface MetricRow { x: string; y: number }

	const DAY = 86400000;
	let websites = $state<Website[]>([]);
	let siteId = $state('');
	let days = $state(7);
	let stats = $state<Stats | null>(null);
	let series = $state<Point[]>([]);
	let topPages = $state<MetricRow[]>([]);
	let loading = $state(false);
	let error = $state('');

	function data<T>(res: any): T | null {
		if (!res) return null;
		if ('data' in res) return res.data as T;
		return res as T;
	}

	async function load() {
		if (!siteId) return;
		loading = true;
		error = '';
		const startAt = Date.now() - days * DAY;
		const endAt = Date.now();
		const q = `startAt=${startAt}&endAt=${endAt}&timezone=UTC`;
		const [s, pv, m] = await Promise.all([
			api(`/analytics/umami/websites/${siteId}/stats?${q}`),
			api(`/analytics/umami/websites/${siteId}/pageviews?${q}&unit=day`),
			api(`/analytics/umami/websites/${siteId}/metrics?${q}&type=url`)
		]);
		stats = data<Stats>(s);
		const pvd = data<{ pageviews: Point[] }>(pv);
		series = pvd?.pageviews ?? [];
		topPages = (data<MetricRow[]>(m) ?? []).slice(0, 10);
		if (!stats && !pvd) error = 'Analytics unavailable — check the Umami proxy configuration.';
		loading = false;
	}

	onMount(async () => {
		const res = await api('/analytics/umami/websites?pageSize=50');
		const list = data<{ data: Website[] }>(res);
		websites = list?.data ?? [];
		siteId = websites.find((w) => w.domain === 'japa.to')?.id ?? websites[0]?.id ?? '';
		await load();
	});

	const maxY = $derived(Math.max(1, ...series.map((p) => p.y)));
	const points = $derived(
		series.map((p, i) => `${(i / Math.max(1, series.length - 1)) * 100},${40 - (p.y / maxY) * 38}`).join(' ')
	);
	const fmt = (n: number) => (n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(1)}k` : `${Math.round(n)}`);
</script>

<PageHeader title={t('analytics.title', 'Analytics')} description={t('analytics.description', 'Page-view analytics from the self-hosted Umami instance.')} />

<div class="space-y-4">
	<div class="flex flex-wrap items-center gap-3">
		<AutocompleteSelect
			bind:value={siteId}
			onValueChange={() => load()}
			options={websites.map((w) => ({ value: w.id, label: `${w.name} — ${w.domain}` }))}
		/>
		<AutocompleteSelect
			value={String(days)}
			onValueChange={(v) => { days = Number(v); load(); }}
			options={[1, 7, 30, 90].map((d) => ({ value: String(d), label: d === 1 ? '24h' : `${d}d` }))}
		/>
		{#if loading}<span class="text-muted-foreground text-sm">Loading…</span>{/if}
	</div>

	{#if error}
		<div class="border-border-card rounded-lg border p-6"><p class="text-muted-foreground text-sm">{error}</p></div>
	{:else if stats}
		<div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
			{#each ([
				['Pageviews', stats.pageviews], ['Visitors', stats.visitors], ['Visits', stats.visits],
				['Bounce rate', stats.bounces], ['Avg. time', stats.totaltime]
			] as [string, Metric][]) as [label, m]}
				<div class="border-border-card rounded-lg border p-4">
					<p class="text-muted-foreground text-xs">{label}</p>
					<p class="mt-1 text-xl font-semibold">
						{label === 'Bounce rate' ? `${Math.round(m.value)}%` : label === 'Avg. time' ? `${Math.round(m.value)}s` : fmt(m.value)}
					</p>
					{#if m.change !== 0}<p class="text-xs" class:text-green-500={m.change > 0} class:text-red-500={m.change < 0}>{m.change > 0 ? '+' : ''}{m.change} prev</p>{/if}
				</div>
			{/each}
		</div>

		<div class="border-border-card rounded-lg border p-4">
			<p class="text-muted-foreground mb-2 text-sm">Pageviews</p>
			{#if series.length}
				<svg viewBox="0 0 100 42" preserveAspectRatio="none" class="h-40 w-full">
					<polyline points={points} fill="none" stroke="var(--color-accent, #6366f1)" stroke-width="0.7" vector-effect="non-scaling-stroke" />
				</svg>
				<div class="text-muted-foreground flex justify-between text-xs">
					<span>{new Date(series[0].x).toLocaleDateString()}</span>
					<span>{new Date(series[series.length - 1].x).toLocaleDateString()}</span>
				</div>
			{/if}
		</div>

		{#if topPages.length}
			<div class="border-border-card rounded-lg border p-4">
				<p class="text-muted-foreground mb-2 text-sm">Top pages</p>
				<table class="w-full text-sm">
					<tbody>
						{#each topPages as r}
							<tr class="border-border-card border-t"><td class="truncate py-1.5 pr-4 font-mono text-xs">{r.x}</td><td class="py-1.5 text-right tabular-nums">{r.y}</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</div>
