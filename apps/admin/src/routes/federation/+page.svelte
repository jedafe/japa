<script lang="ts">
	import { onMount } from 'svelte';
	import { siteName } from '#lib/stores/site.svelte.js';
	// /federation — §126 ops visibility: /admin/federation/stats counters.
	import StatCard from '#lib/components/ui/StatCard.svelte';
	import { federationAdminApi } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';

	type FederationStats = {
		local_actors: number;
		remote_actors: number;
		remote_follows: number;
		deliveries_pending: number;
		deliveries_delivered: number;
		deliveries_dead: number;
		instance_blocks: number;
	};

	let stats = $state<FederationStats | null>(null);
	let loadError = $state('');

	onMount(async () => {
		const res = await federationAdminApi.stats();
		if (res.ok) {
			stats = res.data as FederationStats;
		} else {
			loadError = res.error ?? t('common.error', 'Failed to load federation stats');
		}
	});
</script>

<svelte:head>
	<title>{t('federation.overview.title', 'Federation')} — {siteName()} Admin</title>
</svelte:head>

<div class="mb-6">
	<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('federation.overview.title', 'Federation')}</h1>
	<p class="mt-1 text-sm" style="color: var(--color-muted)">
		{t('federation.overview.description', 'ActivityPub federation counters — actors, follows, delivery ledger health, and instance blocks.')}
	</p>
</div>

{#if loadError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{loadError}
	</div>
{/if}

{#if stats}
	<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
		<StatCard label={t('federation.overview.local_actors', 'Local actors')} value={stats.local_actors} />
		<StatCard label={t('federation.overview.remote_actors', 'Remote actors')} value={stats.remote_actors} />
		<StatCard label={t('federation.overview.remote_follows', 'Remote follows')} value={stats.remote_follows} />
		<StatCard label={t('federation.overview.instance_blocks', 'Instance blocks')} value={stats.instance_blocks} />
	</div>
	<div class="mt-4 grid grid-cols-3 gap-4">
		<StatCard label={t('federation.overview.deliveries_pending', 'Deliveries pending')} value={stats.deliveries_pending} />
		<StatCard label={t('federation.overview.deliveries_delivered', 'Deliveries delivered')} value={stats.deliveries_delivered} />
		<StatCard label={t('federation.overview.deliveries_dead', 'Deliveries dead')} value={stats.deliveries_dead} />
	</div>
{/if}
