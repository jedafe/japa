<script lang="ts">
	import { onMount } from 'svelte';
	import StatsCard from '$lib/components/StatsCard.svelte';
	import { api } from '$lib/api';

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

<h1 class="mb-1 text-2xl font-bold tracking-tight">Dashboard</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">Community health overview</p>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<!-- Key metrics -->
	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		<StatsCard label="Total Users" value={data.total_users} icon="👤" />
		<StatsCard label="New Today" value={data.new_users_today} subtitle="signups" />
		<StatsCard label="Total Content" value={data.total_content} subtitle="items" />
		<StatsCard label="Open Tickets" value={data.pending_tickets} subtitle="awaiting response" />
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<!-- Platform Summary -->
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Platform Overview</h2>
			<div class="space-y-2 text-sm">
				<div class="flex items-center justify-between">
					<span>Total Users</span>
					<span class="font-semibold">{data.total_users}</span>
				</div>
				<div class="flex items-center justify-between">
					<span>New Users Today</span>
					<span class="font-semibold">{data.new_users_today}</span>
				</div>
				<div class="flex items-center justify-between">
					<span>Total Content</span>
					<span class="font-semibold">{data.total_content}</span>
				</div>
			</div>
		</div>

		<!-- Support Summary -->
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Support Queue</h2>
			<div class="space-y-3">
				<div class="flex items-center justify-between text-sm">
					<span>Pending tickets</span>
					<span class="font-semibold" style="color: var(--color-warning)">{data.pending_tickets}</span>
				</div>
			</div>
		</div>
	</div>
{/if}
