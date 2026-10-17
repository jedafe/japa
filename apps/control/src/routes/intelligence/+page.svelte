<script lang="ts">
	import { api, apiList } from '$lib/api';
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
		Search,
		ArrowRight,
		Database,
		Globe,
		ListChecks,
		CheckCircle2,
		ExternalLink
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
		status?: string | null;
		last_crawled_at?: string | null;
		last_success_at?: string | null;
		last_error_at?: string | null;
		crawl_frequency_mins?: number;
	}

	interface RadarCountsData {
		total_new?: number;
		by_type?: Record<string, number>;
		by_country?: Record<string, number>;
	}

	interface ReviewRecord {
		id: string;
		status: string;
		entity_type: string;
	}

	interface IntelRecord {
		id: string;
		status: string;
		entity_type: string;
		source_id: string;
		extracted?: Record<string, unknown> | string;
		confidence_overall?: number;
		promoted_record_id?: string | null;
		extracted_at?: string | null;
		last_verified_at?: string | null;
	}

	let sources = $state<SourceEntry[]>([]);
	let reviewTotal = $state(0);
	let approvedTotal = $state(0);
	let publishedTotal = $state(0);
	let proposedTotal = $state(0);
	let sourcesTotal = $state(0);
	let enabledTotal = $state(0);
	let radarCounts = $state<RadarCountsData | null>(null);
	let promotedRecords = $state<IntelRecord[]>([]);
	let loading = $state(true);
	let refreshing = $state(false);
	let errorMessage = $state('');
	let lastRefresh = $state(new Date());

	async function load(signal?: AbortSignal) {
		refreshing = true;
		errorMessage = '';
		// Fetch the intelligence endpoints in parallel. Each is best-effort:
		// a failure on one does not abort the others, so the overview
		// degrades gracefully when an endpoint is unavailable. Queue cards
		// use meta.total so the counts reflect the true pipeline depth
		// rather than just the rows fetched for display.
		const [srcRes, reviewRes, countsRes, promotedRes, approvedRes, proposedRes, enabledRes] = await Promise.allSettled([
			apiList<SourceEntry[]>('/api/v1/intel/sources?page_size=100&include_disabled=true', { signal }),
			apiList<ReviewRecord[]>('/api/v1/intel/review/queue?page_size=1', { signal }),
			api<RadarCountsData>('/api/v1/intel/radar/counts', { signal }),
			apiList<IntelRecord[]>('/api/v1/intel/records?status=published&page_size=50', { signal }),
			apiList<IntelRecord[]>('/api/v1/intel/records?status=approved&page_size=1', { signal }),
			apiList<SourceEntry[]>('/api/v1/intel/sources/proposed?page_size=1', { signal }),
			apiList<SourceEntry[]>('/api/v1/intel/sources?page_size=1&enabled=true', { signal })
		]);

		if (srcRes.status === 'fulfilled' && srcRes.value.ok) {
			sources = Array.isArray(srcRes.value.data) ? (srcRes.value.data as SourceEntry[]) : [];
			sourcesTotal = srcRes.value.meta.total ?? sources.length;
		}
		if (reviewRes.status === 'fulfilled' && reviewRes.value.ok) {
			reviewTotal = reviewRes.value.meta.total ?? 0;
		}
		if (countsRes.status === 'fulfilled' && countsRes.value.ok) {
			radarCounts = countsRes.value.data;
		}
		if (promotedRes.status === 'fulfilled' && promotedRes.value.ok) {
			promotedRecords = Array.isArray(promotedRes.value.data) ? (promotedRes.value.data as IntelRecord[]) : [];
			publishedTotal = promotedRes.value.meta.total ?? promotedRecords.length;
		}
		if (approvedRes.status === 'fulfilled' && approvedRes.value.ok) {
			approvedTotal = approvedRes.value.meta.total ?? 0;
		}
		if (proposedRes.status === 'fulfilled' && proposedRes.value.ok) {
			proposedTotal = proposedRes.value.meta.total ?? 0;
		}
		if (enabledRes.status === 'fulfilled' && enabledRes.value.ok) {
			enabledTotal = enabledRes.value.meta.total ?? 0;
		}

		// Surface the first non-abort error so the operator knows something
		// is wrong, but only if every endpoint failed.
		const failed = [srcRes, reviewRes, countsRes, promotedRes, approvedRes, proposedRes, enabledRes].filter(
			(r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value.ok)
		);
		if (failed.length === 7) {
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

	// Fetch queue: registered sources awaiting/eligible for crawl (total
	// from the sources directory meta).
	const fetchQueueCount = $derived(sourcesTotal);

	// Extract queue: records that have been extracted and are flowing
	// through the pipeline toward review — mirrors the review queue depth.
	const extractQueueCount = $derived(reviewTotal);

	// Dedup queue: records that have been approved but not yet promoted.
	// These are records in 'approved' status waiting for the intel:promote
	// task to project them into domain tables.
	const dedupQueueCount = $derived(approvedTotal);

	// Review queue: records awaiting human moderation (server-side total).
	const reviewQueueCount = $derived(reviewTotal);

	// Publish queue: promoted records that are live in domain projections.
	const publishQueueCount = $derived(publishedTotal);

	// Proposed queue: sources awaiting admin approval (server-side total).
	const proposedQueueCount = $derived(proposedTotal);

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

	const enabledSources = $derived(enabledTotal);
	// Countries covered: count of distinct countries in the radar counts.
	const countriesCovered = $derived(
		radarCounts?.by_country ? Object.keys(radarCounts.by_country).length : 0
	);

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
			href: '/intelligence/review?status=review_required',
			subtitle: t('intel.queue_extract_sub', 'Records post-extraction')
		},
		{
			key: 'dedup',
			label: t('intel.queue_dedup', 'Approved Queue'),
			count: dedupQueueCount,
			icon: Copy,
			accent: 'text-cyan-500',
			href: '/intelligence/review?status=approved',
			subtitle: t('intel.queue_dedup_sub', 'Approved, awaiting promotion')
		},
		{
			key: 'review',
			label: t('intel.queue_review', 'Review Queue'),
			count: reviewQueueCount,
			icon: Eye,
			accent: 'text-amber-500',
			href: '/intelligence/review?status=review_required',
			subtitle: t('intel.queue_review_sub', 'Records awaiting moderation')
		},
		{
			key: 'publish',
			label: t('intel.queue_publish', 'Publish Queue'),
			count: publishQueueCount,
			icon: Rocket,
			accent: 'text-emerald-500',
			href: '/intelligence/review?status=published',
			subtitle: t('intel.queue_publish_sub', 'Promoted records live')
		},
		{
			key: 'proposed',
			label: t('intel.queue_proposed', 'Proposed Sources'),
			count: proposedQueueCount,
			icon: Search,
			accent: 'text-pink-500',
			href: '/intelligence/sources/proposed',
			subtitle: t('intel.queue_proposed_sub', 'Discovered sources awaiting review')
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

	// Entity-type breakdown of promoted records for the list section.
	const promotedByType = $derived.by(() => {
		const map: Record<string, number> = {};
		for (const r of promotedRecords) {
			map[r.entity_type] = (map[r.entity_type] ?? 0) + 1;
		}
		return map;
	});

	// Extract a display title from the intel record's extracted JSON blob.
	function recordTitle(rec: IntelRecord): string {
		if (!rec.extracted) return rec.entity_type;
		let raw: Record<string, unknown> | null = null;
		if (typeof rec.extracted === 'string') {
			try { raw = JSON.parse(rec.extracted); } catch { /* ignore */ }
		} else if (typeof rec.extracted === 'object') {
			raw = rec.extracted as Record<string, unknown>;
		}
		if (raw) {
			for (const key of ['title', 'name', 'program', 'scholarship_name', 'job_title']) {
				const v = raw[key];
				if (typeof v === 'string' && v.trim()) return v;
				if (v && typeof v === 'object' && 'value' in v) {
					const val = (v as { value: unknown }).value;
					if (typeof val === 'string' && val.trim()) return val;
				}
			}
		}
		return `${rec.entity_type} record`;
	}

	function recordEntityTypeLabel(et: string): string {
		const labels: Record<string, string> = {
			scholarship: 'Scholarship', job: 'Job', visa: 'Visa',
			housing: 'Housing', remittance: 'Remittance', cost_of_living: 'Cost of Living',
			guide: 'Guide', news: 'News', community: 'Community', mentorship: 'Mentorship'
		};
		return labels[et] ?? et;
	}

	function recordEntityTypeColor(et: string): string {
		const colors: Record<string, string> = {
			scholarship: 'bg-emerald-500/15 text-emerald-400',
			job: 'bg-sky-500/15 text-sky-400',
			visa: 'bg-violet-500/15 text-violet-400',
			housing: 'bg-amber-500/15 text-amber-400',
			remittance: 'bg-cyan-500/15 text-cyan-400',
			cost_of_living: 'bg-rose-500/15 text-rose-400',
			guide: 'bg-indigo-500/15 text-indigo-400'
		};
		return colors[et] ?? 'bg-muted text-muted-foreground';
	}

	function formatDate(s?: string | null): string {
		if (!s) return '';
		const d = new Date(s);
		if (isNaN(d.getTime())) return '';
		return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
	}

	// Build the Main app URL for cross-app links. In dev, the Control app
	// runs on port 3002 and the Main app on port 3000. In production, both
	// are served from the same domain (e.g. japa.to) so a relative
	// /scholarships path works. We detect dev by checking if the port is
	// 3002 and swap to 3000.
	function mainAppUrl(path: string): string {
		const origin = window.location.origin;
		if (origin.includes(':3002')) {
			return origin.replace(':3002', ':3000') + path;
		}
		return origin + path;
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
					onclick={() => {
						if (!q.href) return;
						if (q.href.startsWith('#')) {
							document.querySelector(q.href)?.scrollIntoView({ behavior: 'smooth' });
						} else {
							goto(q.href);
						}
					}}
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
				<p class="text-foreground text-lg font-bold">{formatNumber(sourcesTotal)}</p>
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

	<!-- Promoted records list -->
	{#if !loading}
		<div id="promoted-records" class="mt-6 scroll-mt-4">
			<div class="mb-3 flex items-center justify-between">
				<div class="flex items-center gap-2">
					<CheckCircle2 class="text-emerald-500 size-5" />
					<h2 class="text-foreground text-lg font-semibold">
						{t('intel.promoted_records', 'Promoted Records')}
					</h2>
					<span class="bg-emerald-500/15 text-emerald-400 rounded-full px-2 py-0.5 text-xs font-medium">
						{publishedTotal}
					</span>
				</div>
				{#if Object.keys(promotedByType).length > 0}
					<div class="flex flex-wrap gap-2">
						{#each Object.entries(promotedByType) as [type, count]}
							<span class="rounded-full px-2.5 py-0.5 text-xs font-medium {recordEntityTypeColor(type)}">
								{recordEntityTypeLabel(type)}: {count}
							</span>
						{/each}
					</div>
				{/if}
			</div>

			{#if promotedRecords.length === 0}
				<div class="border-border-card bg-background-alt rounded-xl border p-8 text-center">
					<Rocket class="text-muted-foreground mx-auto mb-2 size-8" />
					<p class="text-muted-foreground text-sm">
						{t('intel.no_promoted', 'No promoted records yet. Approve records in the review queue to promote them.')}
					</p>
					<a
						href="/intelligence/review"
						class="text-emerald-500 hover:text-emerald-400 mt-2 inline-flex items-center gap-1 text-sm font-medium"
					>
						{t('intel.open_review_queue', 'Open Review Queue')}
						<ArrowRight class="size-3.5" />
					</a>
				</div>
			{:else}
				<div class="border-border-card bg-background-alt overflow-hidden rounded-xl border">
					<div class="divide-y divide-border-card">
						{#each promotedRecords.slice(0, 20) as rec (rec.id)}
							<div class="hover:bg-muted/50 flex items-center gap-3 px-4 py-3 transition-colors">
								<div class="flex size-9 shrink-0 items-center justify-center rounded-lg {recordEntityTypeColor(rec.entity_type)}">
									<span class="text-xs font-bold uppercase">{rec.entity_type.slice(0, 2)}</span>
								</div>
								<div class="min-w-0 flex-1">
									<p class="text-foreground truncate text-sm font-medium">
										{recordTitle(rec)}
									</p>
									<div class="text-muted-foreground mt-0.5 flex items-center gap-2 text-xs">
										<span class="rounded-full px-2 py-0.5 {recordEntityTypeColor(rec.entity_type)}">
											{recordEntityTypeLabel(rec.entity_type)}
										</span>
										{#if rec.confidence_overall != null}
											<span>Confidence: {(rec.confidence_overall * 100).toFixed(0)}%</span>
										{/if}
										{#if rec.extracted_at}
											<span>Promoted: {formatDate(rec.extracted_at)}</span>
										{/if}
									</div>
								</div>
								<div class="flex shrink-0 items-center gap-2">
									<span class="bg-emerald-500/15 text-emerald-400 flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium">
										<CheckCircle2 class="size-3" />
										Published
									</span>
									{#if rec.entity_type === 'scholarship'}
										<a
											href={mainAppUrl('/scholarships')}
											target="_blank"
											rel="noopener"
											class="text-muted-foreground hover:text-foreground transition-colors"
											title="View on public site"
										>
											<ExternalLink class="size-4" />
										</a>
									{/if}
								</div>
							</div>
						{/each}
					</div>
					{#if publishedTotal > promotedRecords.length}
						<div class="border-t-border-card text-muted-foreground border-t px-4 py-2.5 text-center text-xs">
							Showing {promotedRecords.length} of {publishedTotal} promoted records
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{/if}

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
