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
		active_users_7d?: number;
		pending_reports?: number;
		spam_queue_count?: number;
		total_generations_today?: number;
		hitl_pending_count?: number;
		recent_signups?: Array<{ id: string; username: string; avatar_url?: string; created_at: string }>;
		[key: string]: unknown;
	};

	const periods = [
		{ key: 'today', label: 'dashboard.period_today' },
		{ key: '7d', label: 'dashboard.period_7d' },
		{ key: '30d', label: 'dashboard.period_30d' },
		{ key: '90d', label: 'dashboard.period_90d' }
	] as const;

	let data = $state<OverviewData>({
		total_users: 0,
		new_users_today: 0,
		total_content: 0,
		pending_tickets: 0
	});
	let loading = $state(true);
	let selectedPeriod = $state<string>('7d');

	async function load() {
		loading = true;
		try {
			const params = new URLSearchParams({ period: selectedPeriod });
			const result = await api<OverviewData>(`/admin/analytics/overview?${params}`);
			if (result.ok) data = result.data;
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function setPeriod(p: string) {
		selectedPeriod = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Dashboard — JapaConnect Admin</title>
</svelte:head>

<div class="animate-fade-in">
	<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('dashboard.title')}</h1>
	<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('dashboard.description')}</p>

	<!-- Period Selector -->
	<div class="mb-6 flex gap-1 rounded-lg p-1" style="background: var(--color-surface)">
		{#each periods as period}
			<button
				class="rounded-md px-3 py-1.5 text-sm font-medium transition-all"
				style="background: {selectedPeriod === period.key ? 'var(--color-primary)' : 'transparent'}; color: {selectedPeriod === period.key ? '#fff' : 'var(--color-muted)'}"
				onclick={() => setPeriod(period.key)}
			>
				{t(period.label)}
			</button>
		{/each}
	</div>

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
			<StatsCard label={t('dashboard.total_users')} value={data.total_users} icon="&#x1f465;" />
			<StatsCard label={t('dashboard.new_today')} value={data.new_users_today} subtitle={t('dashboard.signups')} />
			<StatsCard label={t('dashboard.total_content')} value={data.total_content} subtitle={t('dashboard.items')} />
			<StatsCard label={t('dashboard.open_tickets')} value={data.pending_tickets} subtitle={t('dashboard.awaiting')} />
		</div>

		<!-- Community Health -->
		<div class="mb-6 rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
			<h2 class="mb-4 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.community_health')}</h2>
			<div class="grid gap-4 sm:grid-cols-3">
				<div class="flex items-center gap-3">
					<div class="flex h-10 w-10 items-center justify-center rounded-xl text-lg" style="background: color-mix(in srgb, var(--color-success) 12%, transparent)">
						&#x1f331;
					</div>
					<div>
						<p class="text-2xl font-bold">{data.new_users_today}</p>
						<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.new_users_today')}</p>
					</div>
				</div>
				<div class="flex items-center gap-3">
					<div class="flex h-10 w-10 items-center justify-center rounded-xl text-lg" style="background: color-mix(in srgb, var(--color-info) 12%, transparent)">
						&#x1f4ca;
					</div>
					<div>
						<p class="text-2xl font-bold">{data.total_content}</p>
						<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.total_content')}</p>
					</div>
				</div>
				<div class="flex items-center gap-3">
					<div class="flex h-10 w-10 items-center justify-center rounded-xl text-lg" style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)">
						&#x1f451;
					</div>
					<div>
						<p class="text-2xl font-bold">{data.active_users_7d ?? '—'}</p>
						<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.active_users_7d')}</p>
					</div>
				</div>
			</div>
		</div>

		<!-- Moderation Snapshot -->
		<div class="mb-6 rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
			<h2 class="mb-4 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.moderation_snapshot')}</h2>
			<div class="grid gap-4 sm:grid-cols-3">
				<div class="flex items-center justify-between rounded-xl border px-4 py-3" style="border-color: var(--color-border)">
					<div>
						<p class="text-lg font-bold">{data.pending_reports ?? 0}</p>
						<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.pending_reports')}</p>
					</div>
					<span class="rounded-full px-2 py-0.5 text-xs font-bold" style="background: color-mix(in srgb, var(--color-warning) 12%, transparent); color: var(--color-warning)">
						{(data.pending_reports ?? 0) > 0 ? '!' : '✓'}
					</span>
				</div>
				<div class="flex items-center justify-between rounded-xl border px-4 py-3" style="border-color: var(--color-border)">
					<div>
						<p class="text-lg font-bold">{data.pending_tickets}</p>
						<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.pending_tickets')}</p>
					</div>
					<span class="rounded-full px-2 py-0.5 text-xs font-bold" style="background: color-mix(in srgb, var(--color-info) 12%, transparent); color: var(--color-info)">
						{data.pending_tickets > 0 ? '!' : '✓'}
					</span>
				</div>
				<div class="flex items-center justify-between rounded-xl border px-4 py-3" style="border-color: var(--color-border)">
					<div>
						<p class="text-lg font-bold">{data.spam_queue_count ?? 0}</p>
						<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.spam_queue')}</p>
					</div>
					<span class="rounded-full px-2 py-0.5 text-xs font-bold" style="background: color-mix(in srgb, var(--color-danger) 12%, transparent); color: var(--color-danger)">
						{(data.spam_queue_count ?? 0) > 0 ? '!' : '✓'}
					</span>
				</div>
			</div>
		</div>

		<!-- Bottom Grid: Recent Signups + AI Usage -->
		<div class="grid gap-4 lg:grid-cols-2">
			<!-- Recent Signups Feed -->
			<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.recent_signups')}</h2>
					<a href="/users" class="text-xs font-medium transition-colors hover:underline" style="color: var(--color-primary)">{t('dashboard.view_all')}</a>
				</div>
				{#if data.recent_signups && data.recent_signups.length > 0}
					<div class="space-y-2">
						{#each data.recent_signups as user}
							<a href="/users/{user.id}" class="flex items-center gap-3 rounded-xl px-3 py-2 transition-colors hover:bg-black/5 dark:hover:bg-white/5">
								<div class="flex h-8 w-8 items-center justify-center rounded-full text-sm font-bold text-white" style="background: var(--gradient-primary)">
									{user.username.charAt(0).toUpperCase()}
								</div>
								<div class="flex-1 min-w-0">
									<p class="text-sm font-medium truncate">{user.username}</p>
									<p class="text-xs" style="color: var(--color-muted)">{user.created_at}</p>
								</div>
							</a>
						{/each}
					</div>
				{:else}
					<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('dashboard.no_signups')}</p>
				{/if}
			</div>

			<!-- AI Usage Widget -->
			<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.ai_usage')}</h2>
					<a href="/ai" class="text-xs font-medium transition-colors hover:underline" style="color: var(--color-primary)">{t('dashboard.view_all')}</a>
				</div>
				<div class="space-y-4">
					<div class="flex items-center justify-between rounded-xl border px-4 py-3" style="border-color: var(--color-border)">
						<div>
							<p class="text-2xl font-bold">{data.total_generations_today ?? 0}</p>
							<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.generations_today')}</p>
						</div>
						<div class="flex h-10 w-10 items-center justify-center rounded-xl text-lg" style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)">
							&#x2699;
						</div>
					</div>
					<div class="flex items-center justify-between rounded-xl border px-4 py-3" style="border-color: var(--color-border)">
						<div>
							<p class="text-2xl font-bold">{data.hitl_pending_count ?? 0}</p>
							<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.hitl_pending')}</p>
						</div>
						<div class="flex h-10 w-10 items-center justify-center rounded-xl text-lg" style="background: color-mix(in srgb, var(--color-warning) 12%, transparent)">
							&#x23f3;
						</div>
					</div>
				</div>
			</div>
		</div>
	{/if}
</div>
