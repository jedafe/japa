<script lang="ts">
	import { onMount } from 'svelte';
	import StatsCard from '$lib/components/StatsCard.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type OverviewData = {
		total_users: number;
		new_users_today: number;
		total_content: number;
		pending_tickets: number;
		[key: string]: unknown;
	};

	let data = $state<OverviewData>({
		total_users: 0,
		new_users_today: 0,
		total_content: 0,
		pending_tickets: 0
	});
	let loading = $state(true);

	async function load() {
		try {
			const result = await api<OverviewData>('/admin/analytics/overview');
			if (result.ok) data = result.data;
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>Dashboard — JapaConnect Admin</title>
</svelte:head>

<div class="animate-fade-in">
	<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('dashboard.title')}</h1>
	<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('dashboard.description')}</p>

	{#if loading}
		<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
			{#each Array(4) as _}
				<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--color-surface)">
					<div class="skeleton mb-2 h-4 w-20 rounded-lg"></div>
					<div class="skeleton h-8 w-16 rounded-lg"></div>
				</div>
			{/each}
		</div>
	{:else}
		<!-- Key metrics -->
		<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
			<StatsCard label={t('dashboard.total_users')} value={data.total_users} icon="👤" />
			<StatsCard label={t('dashboard.new_today')} value={data.new_users_today} subtitle={t('dashboard.signups')} />
			<StatsCard label={t('dashboard.total_content')} value={data.total_content} subtitle={t('dashboard.items')} />
			<StatsCard label={t('dashboard.open_tickets')} value={data.pending_tickets} subtitle={t('dashboard.awaiting')} />
		</div>

		<div class="grid gap-4 lg:grid-cols-2">
			<!-- Platform Summary -->
			<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
				<h2 class="mb-4 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.platform_overview')}</h2>
				<div class="space-y-3 text-sm">
					<div class="flex items-center justify-between">
						<span style="color: var(--color-foreground)">{t('dashboard.total_users')}</span>
						<span class="font-bold">{data.total_users}</span>
					</div>
					<div class="flex items-center justify-between">
						<span style="color: var(--color-foreground)">{t('dashboard.new_today')}</span>
						<span class="font-bold">{data.new_users_today}</span>
					</div>
					<div class="flex items-center justify-between">
						<span style="color: var(--color-foreground)">{t('dashboard.total_content')}</span>
						<span class="font-bold">{data.total_content}</span>
					</div>
				</div>
			</div>

			<!-- Support Summary -->
			<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
				<h2 class="mb-4 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.support_queue')}</h2>
				<div class="space-y-3">
					<div class="flex items-center justify-between text-sm">
						<span style="color: var(--color-foreground)">{t('dashboard.pending_tickets')}</span>
						<span
							class="rounded-full px-2.5 py-0.5 text-sm font-bold"
							style="background: color-mix(in srgb, var(--color-warning) 12%, transparent); color: var(--color-warning)"
						>
							{data.pending_tickets}
						</span>
					</div>
				</div>
			</div>
		</div>
	{/if}
</div>
