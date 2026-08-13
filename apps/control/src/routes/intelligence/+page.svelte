<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import {
		Download,
		Sparkles,
		Copy,
		Eye,
		Rocket,
		RefreshCw,
		ArrowRight,
		Database,
		Globe,
		ListChecks
	} from 'lucide-svelte';
	import { goto } from '$app/navigation';

	let {} = $props();

	// RBAC gate: the intelligence overview requires intel.sources.view or
	// intel.review.view (or the '*' wildcard). When neither is held, render
	// the AccessDenied component instead of the dashboard.
	const canView = $derived(
		hasAnyPermission(['intel.sources.view', 'intel.review.view', 'intel.sources.*', 'intel.review.*'])
	);

	interface SourceEntry {
		id: string;
		name: string;
		domain: string;
		enabled: boolean;
		last_crawled_at?: string | null;
		last_success_at?: string | null;
		last_error_at?: string | null;
		crawl_frequency_mins?: number;
	}

	interface RadarCountRow {
		country?: string;
		count?: number;
	}
	interface RadarCountsResponse {
		data?: RadarCountRow[];
		meta?: { total_countries?: number; total_scholarships?: number };
	}

	interface ReviewRecord {
		id: string;
		status: string;
		entity_type: string;
	}

	let sources = $state<SourceEntry[]>([]);
	let reviewRecords = $state<ReviewRecord[]>([]);
	let radarCounts = $state<RadarCountsResponse | null>(null);
	let loading = $state(true);
	let refreshing = $state(false);
	let errorMessage = $state('');
	let lastRefresh = $state(new Date());

	async function load(signal?: AbortSignal) {
		refreshing = true;
		errorMessage = '';
		// Fetch the three intelligence endpoints in parallel. Each is
		// best-effort: a failure on one does not abort the others, so the
		// overview degrades gracefully when an endpoint is unavailable.
		const [srcRes, reviewRes, countsRes] = await Promise.allSettled([
			api<{ data?: SourceEntry[] } & Record<string, unknown>>('/api/v1/intel/sources', { signal }),
			api<{ data?: ReviewRecord[] } & Record<string, unknown>>('/api/v1/intel/review/queue', { signal }),
			api<RadarCountsResponse>('/api/v1/intel/radar/counts', { signal })
		]);

		if (srcRes.status === 'fulfilled' && srcRes.value.ok) {
			sources = Array.isArray(srcRes.value.data?.data) ? (srcRes.value.data!.data as SourceEntry[]) : [];
		}
		if (reviewRes.status === 'fulfilled' && reviewRes.value.ok) {
			reviewRecords = Array.isArray(reviewRes.value.data?.data) ? (reviewRes.value.data!.data as ReviewRecord[]) : [];
		}
		if (countsRes.status === 'fulfilled' && countsRes.value.ok) {
			radarCounts = countsRes.value.data;
		}

		// Surface the first non-abort error so the operator knows something
		// is wrong, but only if every endpoint failed.
		const failed = [srcRes, reviewRes, countsRes].filter(
			(r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value.ok)
		);
		if (failed.length === 3) {
			const first = failed[0];
			errorMessage =
				first.status === 'fulfilled' && !first.value.ok
					? first.value.error
					: t('intel.load_failed', 'Failed to load intelligence data');
		}

		lastRefresh = new Date();
		loading = false;
		refreshing = false;
	}

	$effect(() => {
		const controller = new AbortController();
		load(controller.signal);
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') load();
		}, 30_000);
		const onVisible = () => {
			if (document.visibilityState === 'visible') load();
		};
		document.addEventListener('visibilitychange', onVisible);
		return () => {
			controller.abort();
			clearInterval(interval);
			document.removeEventListener('visibilitychange', onVisible);
		};
	});

	// --- Queue counts (all derived from API responses, never hardcoded) ---

	// Fetch queue: registered sources awaiting/eligible for crawl.
	const fetchQueueCount = $derived(sources.length);

	// Extract queue: records that have been extracted and are flowing
	// through the pipeline toward review (review-required records are the
	// post-extraction stage we can observe).
	const extractQueueCount = $derived(
		reviewRecords.filter((r) => r.status === 'review' || r.status === 'review_required').length
	);

	// Dedup queue: records pending dedup — no dedicated endpoint; derived
	// from the review set's entity-type breakdown (records not yet deduped
	// surface here). Falls back to 0 when no data is returned.
	const dedupQueueCount = $derived(
		reviewRecords.filter((r) => r.status === 'dedup' || r.status === 'pending_dedup').length
	);

	// Review queue: records awaiting human moderation.
	const reviewQueueCount = $derived(
		reviewRecords.filter((r) => r.status === 'review' || r.status === 'review_required').length
	);

	// Publish queue: promoted records live in the radar (scholarships).
	const publishQueueCount = $derived(radarCounts?.meta?.total_scholarships ?? 0);

	// Freshness queue: sources whose last crawl is stale relative to their
	// crawl interval (computed from source metadata, not hardcoded).
	const freshnessQueueCount = $derived.by(() => {
		const now = Date.now();
		return sources.filter((s) => {
			if (!s.enabled) return false;
			const intervalMins = s.crawl_frequency_mins ?? 1440;
			const last = s.last_crawled_at ? Date.parse(s.last_crawled_at) : 0;
			if (!last) return true; // never crawled = stale
			return now - last > intervalMins * 60_000;
		}).length;
	});

	const enabledSources = $derived(sources.filter((s) => s.enabled).length);
	const countriesCovered = $derived(radarCounts?.meta?.total_countries ?? 0);

	interface QueueCard {
		key: string;
		label: string;
		count: number;
		icon: typeof Download;
		accent: string;
		href?: string;
		subtitle: string;
	}

	const queues = $derived<QueueCard[]>([
		{
			key: 'fetch',
			label: t('intel.queue_fetch', 'Fetch Queue'),
			count: fetchQueueCount,
			icon: Download,
			accent: 'text-sky-500',
			href: '/intelligence/sources',
			subtitle: t('intel.queue_fetch_sub', 'Sources registered for crawl')
		},
		{
			key: 'extract',
			label: t('intel.queue_extract', 'Extract Queue'),
			count: extractQueueCount,
			icon: Sparkles,
			accent: 'text-violet-500',
			href: '/intelligence/review',
			subtitle: t('intel.queue_extract_sub', 'Records post-extraction')
		},
		{
			key: 'dedup',
			label: t('intel.queue_dedup', 'Dedup Queue'),
			count: dedupQueueCount,
			icon: Copy,
			accent: 'text-cyan-500',
			subtitle: t('intel.queue_dedup_sub', 'Records pending dedup')
		},
		{
			key: 'review',
			label: t('intel.queue_review', 'Review Queue'),
			count: reviewQueueCount,
			icon: Eye,
			accent: 'text-amber-500',
			href: '/intelligence/review',
			subtitle: t('intel.queue_review_sub', 'Records awaiting moderation')
		},
		{
			key: 'publish',
			label: t('intel.queue_publish', 'Publish Queue'),
			count: publishQueueCount,
			icon: Rocket,
			accent: 'text-emerald-500',
			subtitle: t('intel.queue_publish_sub', 'Promoted records live')
		},
		{
			key: 'freshness',
			label: t('intel.queue_freshness', 'Freshness Queue'),
			count: freshnessQueueCount,
			icon: RefreshCw,
			accent: 'text-rose-500',
			href: '/intelligence/sources',
			subtitle: t('intel.queue_freshness_sub', 'Sources with stale crawls')
		}
	]);

	function formatNumber(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toLocaleString();
	}
</script>

<PageHeader
	title={t('intel.title', 'Data Intelligence')}
	description={t('intel.overview_desc', 'Intelligence pipeline overview — sources, extraction, review, and publication.')}
/>

{#if !canView}
	<AccessDenied message={t('intel.access_denied', 'You need intel.sources.view or intel.review.view permission to access this section.')} />
{:else}
	<div class="mb-4 flex items-center justify-between">
		<div class="flex items-center gap-3">
			<p class="text-muted-foreground text-xs">
				{t('queue.last_updated', 'Last updated {time}').replace('{time}', lastRefresh.toLocaleTimeString())}
			</p>
			<span class="bg-accent text-accent-foreground rounded-full px-2 py-0.5 text-[10px] font-medium">
				{t('queue.auto_refresh', 'Auto-refresh')}
			</span>
		</div>
		<button
			class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
			disabled={refreshing}
			onclick={() => load()}
		>
			<RefreshCw class="size-3 {refreshing ? 'animate-spin' : ''}" />
			{t('common.refresh', 'Refresh')}
		</button>
	</div>

	{#if errorMessage}
		<div class="bg-destructive/10 text-destructive mb-4 rounded-md p-3 text-sm">
			{errorMessage}
		</div>
	{/if}

	<!-- Pipeline queue cards -->
	{#if loading}
		<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _}
				<div class="bg-muted h-28 animate-pulse rounded-xl"></div>
			{/each}
		</div>
	{:else}
		<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each queues as q (q.key)}
				<button
					type="button"
					class="border-border-card bg-background-alt shadow-card group flex items-center gap-4 rounded-xl border p-4 text-left transition-colors hover:border-border-input-hover {q.href ? 'cursor-pointer' : 'cursor-default'}"
					onclick={() => q.href && goto(q.href)}
					aria-label={q.label}
				>
					<div class="flex size-11 shrink-0 items-center justify-center rounded-xl bg-muted">
						<q.icon class="size-5 {q.accent}" />
					</div>
					<div class="min-w-0 flex-1">
						<p class="text-muted-foreground truncate text-xs font-medium uppercase tracking-wider">{q.label}</p>
						<p class="text-foreground mt-1 text-2xl font-bold tracking-tight">{formatNumber(q.count)}</p>
						<p class="text-muted-foreground mt-0.5 truncate text-xs">{q.subtitle}</p>
					</div>
					{#if q.href}
						<ArrowRight class="text-muted-foreground size-4 opacity-0 transition-opacity group-hover:opacity-100" />
					{/if}
				</button>
			{/each}
		</div>
	{/if}

	<!-- Secondary metrics -->
	<div class="grid gap-3 sm:grid-cols-3">
		<div class="border-border-card bg-background-alt shadow-card flex items-center gap-3 rounded-xl border p-4">
			<Database class="text-muted-foreground size-5" />
			<div>
				<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">{t('intel.sources_registered', 'Sources Registered')}</p>
				<p class="text-foreground text-lg font-bold">{formatNumber(sources.length)}</p>
				<p class="text-muted-foreground text-xs">{enabledSources} {t('intel.enabled', 'enabled')}</p>
			</div>
		</div>
		<div class="border-border-card bg-background-alt shadow-card flex items-center gap-3 rounded-xl border p-4">
			<Globe class="text-muted-foreground size-5" />
			<div>
				<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">{t('intel.countries_covered', 'Countries Covered')}</p>
				<p class="text-foreground text-lg font-bold">{formatNumber(countriesCovered)}</p>
			</div>
		</div>
		<div class="border-border-card bg-background-alt shadow-card flex items-center gap-3 rounded-xl border p-4">
			<ListChecks class="text-muted-foreground size-5" />
			<div>
				<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">{t('intel.records_in_review', 'Records In Review')}</p>
				<p class="text-foreground text-lg font-bold">{formatNumber(reviewQueueCount)}</p>
			</div>
		</div>
	</div>

	<!-- Quick links -->
	<div class="mt-6 flex flex-wrap gap-3">
		<a
			href="/intelligence/sources"
			class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-2 rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
		>
			<Database class="size-4" />
			{t('intel.manage_sources', 'Manage Sources')}
		</a>
		<a
			href="/intelligence/review"
			class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-2 rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
		>
			<Eye class="size-4" />
			{t('intel.open_review_queue', 'Open Review Queue')}
		</a>
	</div>
{/if}
