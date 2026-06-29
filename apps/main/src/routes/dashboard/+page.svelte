<script lang="ts">
	import {
		BarChart3,
		Users,
		FileText,
		Building2,
		TrendingUp,
		Eye,
		UserPlus,
		Loader2
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { getToken } from '$lib/api';
	import { onMount } from 'svelte';

	let { data } = $props();

	let dashboard = $state(data.dashboard);
	let loading = $state(false);
	let errorMessage = $state('');

	interface TopContentItem {
		content_id: string;
		total_views: number;
		total_reactions: number;
		total_comments: number;
		total_unique_viewers: number;
	}

	interface UserDashboard {
		top_content: TopContentItem[];
		user_engagement: Record<string, unknown>[];
		content_over_time: Record<string, unknown>[];
	}

	function authHeaders(extra?: Record<string, string>): Record<string, string> {
		const h: Record<string, string> = { 'Content-Type': 'application/json', ...extra };
		const token = getToken();
		if (token) h['Authorization'] = `Bearer ${token}`;
		return h;
	}

	onMount(async () => {
		if (dashboard) return;
		loading = true;
		try {
			const res = await fetch('/api/v1/analytics/dashboard', { headers: authHeaders() });
			const body = await res.json();
			if (body.data) {
				dashboard = body.data;
			}
		} catch {
			errorMessage = 'Failed to load analytics data.';
		} finally {
			loading = false;
		}
	});

	function formatNumber(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toString();
	}

	const totalViews = $derived(
		dashboard?.top_content?.reduce((sum: number, item: TopContentItem) => sum + item.total_views, 0) ?? 0
	);
	const totalReactions = $derived(
		dashboard?.top_content?.reduce((sum: number, item: TopContentItem) => sum + item.total_reactions, 0) ?? 0
	);
	const totalComments = $derived(
		dashboard?.top_content?.reduce((sum: number, item: TopContentItem) => sum + item.total_comments, 0) ?? 0
	);
	const topContentCount = $derived(dashboard?.top_content?.length ?? 0);

	const stats = $derived(
		dashboard
			? [
					{
						label: 'Top Content',
						value: formatNumber(topContentCount),
						icon: FileText,
						color: 'var(--color-primary)'
					},
					{
						label: 'Total Views',
						value: formatNumber(totalViews),
						icon: Eye,
						color: '#22c55e'
					},
					{
						label: 'Total Reactions',
						value: formatNumber(totalReactions),
						icon: TrendingUp,
						color: '#f59e0b'
					},
					{
						label: 'Total Comments',
						value: formatNumber(totalComments),
						icon: Users,
						color: '#3b82f6'
					}
				]
			: []
	);
</script>

<svelte:head>
	<title>Dashboard | JapaConnect</title>
	<meta name="description" content="Your analytics dashboard on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-5xl px-4 py-8">
	<!-- Header -->
	<header class="mb-8">
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 items-center justify-center rounded-xl"
				style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)"
			>
				<BarChart3 class="h-5 w-5" style="color: var(--color-primary)" />
			</div>
			<div>
				<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
					Dashboard
				</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					Platform overview and key metrics.
				</p>
			</div>
		</div>
	</header>

	{#if !data.isAuthenticated}
		<div
			class="rounded-xl border p-6 text-center"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<BarChart3 class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				Sign in to view your analytics dashboard.
			</p>
			<a
				href="/login"
				class="mt-4 inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background-color: var(--color-primary)"
			>
				Sign in
			</a>
		</div>
	{:else if loading}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _}
				<div
					class="h-28 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if errorMessage}
		<div
			class="rounded-xl border p-6 text-center"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<p class="text-sm" style="color: var(--color-danger)">{errorMessage}</p>
		</div>
	{:else if stats.length === 0}
		<EmptyState
			icon={BarChart3}
			title="No analytics data"
			message="Platform analytics will appear here once there is activity."
		/>
	{:else}
		<!-- Stats Grid -->
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each stats as stat (stat.label)}
				<div
					class="group rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-start justify-between">
						<div>
							<p class="text-xs font-medium uppercase tracking-wide" style="color: var(--color-muted)">
								{stat.label}
							</p>
							<p class="mt-2 text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">
								{stat.value}
							</p>
						</div>
						<div
							class="flex h-9 w-9 items-center justify-center rounded-lg transition-transform group-hover:scale-110"
							style="background: color-mix(in srgb, {stat.color} 12%, transparent)"
						>
							<stat.icon class="h-4.5 w-4.5" style="color: {stat.color}" />
						</div>
					</div>
				</div>
			{/each}
		</div>

		<!-- Top Content Table -->
		{#if dashboard?.top_content?.length}
			<section class="mt-8">
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">Top Content</h2>
				<div
					class="overflow-hidden rounded-xl border"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<table class="w-full text-left text-sm" style="color: var(--color-foreground)">
						<thead>
							<tr style="border-bottom: 1px solid var(--color-border)">
								<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Content</th>
								<th class="px-4 py-3 font-medium text-right" style="color: var(--color-muted)">Views</th>
								<th class="px-4 py-3 font-medium text-right" style="color: var(--color-muted)">Reactions</th>
								<th class="px-4 py-3 font-medium text-right" style="color: var(--color-muted)">Comments</th>
								<th class="px-4 py-3 font-medium text-right" style="color: var(--color-muted)">Unique Viewers</th>
							</tr>
						</thead>
						<tbody>
							{#each dashboard.top_content as item (item.content_id)}
								<tr style="border-bottom: 1px solid var(--color-border)">
									<td class="px-4 py-3 font-mono text-xs">{item.content_id.slice(0, 8)}...</td>
									<td class="px-4 py-3 text-right">{formatNumber(item.total_views)}</td>
									<td class="px-4 py-3 text-right">{formatNumber(item.total_reactions)}</td>
									<td class="px-4 py-3 text-right">{formatNumber(item.total_comments)}</td>
									<td class="px-4 py-3 text-right">{formatNumber(item.total_unique_viewers)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</section>
		{/if}
	{/if}
</main>
