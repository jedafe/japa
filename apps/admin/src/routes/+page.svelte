<script lang="ts">
	import { onMount } from 'svelte';
	import StatsCard from '$lib/components/StatsCard.svelte';
	import { api } from '$lib/api';

	type DashboardData = {
		signups_today: number;
		dau: number;
		mau: number;
		content_volume: number;
		top_spaces: { name: string; members: number }[];
		geo_distribution: { country: string; users: number }[];
		moderation_pending: number;
		moderation_resolved_today: number;
	};

	let data = $state<DashboardData>({
		signups_today: 0,
		dau: 0,
		mau: 0,
		content_volume: 0,
		top_spaces: [],
		geo_distribution: [],
		moderation_pending: 0,
		moderation_resolved_today: 0
	});
	let loading = $state(true);

	async function load() {
		try {
			data = await api<DashboardData>('/admin/dashboard');
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

<h1 class="mb-1 text-2xl font-bold tracking-tight">Dashboard</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">Community health overview</p>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<!-- Key metrics -->
	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		<StatsCard label="Signups Today" value={data.signups_today} icon="👤" />
		<StatsCard label="DAU" value={data.dau} subtitle="Daily Active Users" />
		<StatsCard label="MAU" value={data.mau} subtitle="Monthly Active Users" />
		<StatsCard label="Content Volume" value={data.content_volume} subtitle="posts this week" />
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<!-- Top Spaces -->
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Top Spaces</h2>
			{#if data.top_spaces.length === 0}
				<p class="text-sm" style="color: var(--color-muted)">No data yet</p>
			{:else}
				<div class="space-y-2">
					{#each data.top_spaces as space}
						<div class="flex items-center justify-between text-sm">
							<span>{space.name}</span>
							<span style="color: var(--color-muted)">{space.members} members</span>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Geo Distribution -->
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Geo Distribution</h2>
			{#if data.geo_distribution.length === 0}
				<p class="text-sm" style="color: var(--color-muted)">No data yet</p>
			{:else}
				<div class="space-y-2">
					{#each data.geo_distribution as geo}
						<div class="flex items-center justify-between text-sm">
							<span>{geo.country}</span>
							<span style="color: var(--color-muted)">{geo.users}</span>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Moderation Summary -->
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Moderation</h2>
			<div class="space-y-3">
				<div class="flex items-center justify-between text-sm">
					<span>Pending reviews</span>
					<span class="font-semibold" style="color: var(--color-warning)">{data.moderation_pending}</span>
				</div>
				<div class="flex items-center justify-between text-sm">
					<span>Resolved today</span>
					<span class="font-semibold" style="color: var(--color-success)">{data.moderation_resolved_today}</span>
				</div>
			</div>
		</div>
	</div>
{/if}
