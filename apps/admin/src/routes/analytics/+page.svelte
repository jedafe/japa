<script lang="ts">
	import { t, locale } from '#lib/stores/i18n.svelte.js';
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import PageHeader from "#lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "#lib/components/ui/card/index.js";
	import { Tabs } from "#lib/components/ui/tabs/index.js";
	import { DataTable } from "#lib/components/ui/data-table/index.js";
	import { Button } from "#lib/components/ui/button/index.js";
	import { Modal } from "#lib/components/ui/modal/index.js";
	import { hasPermission } from "#lib/stores/permissions.svelte.js";
	import { 
		BarChart3, Users, Eye, MessageSquare, TrendingUp, 
		Download, RefreshCw, Shield, DollarSign, Search, UserPlus
	} from "lucide-svelte";
	import { analyticsAdminApi } from "#lib/api.js";
	import { LineChart, AreaChart, BarChart } from "layerchart";
	import { curveMonotoneX } from "d3-shape";
	import StatCard from "#lib/components/ui/StatCard.svelte";
	import { CardSkeleton } from "#lib/components/ui/skeleton/index.js";
	import AutocompleteSelect from '#lib/components/ui/AutocompleteSelect.svelte';
	
	let activeTab = $state("overview");
	let flushModalOpen = $state(false);
	let flushing = $state(false);
	let loading = $state(true);
	let error = $state("");
	// Date-range state sourced from the URL so it drives the analytics fetch
	let from = $state(page.url.searchParams.get("from") ?? "");
	let to = $state(page.url.searchParams.get("to") ?? "");
	
	// Analytics data
	type OverviewData = {
		daily_active_users: Array<{ day: string; dau: number }> | null;
		monthly_active_users: Array<{ month: string; mau: number }> | null;
		event_counts: Array<{ event_type: string; count: number }> | null;
		content_over_time: Array<{ day: string; content_count: number }> | null;
		user_growth: Array<{ day: string; new_users: number }> | null;
		traffic_sources: Array<{ referrer: string | null; visits: number }> | null;
		top_content: Array<{ content_id: string; total_views: number; total_reactions: number; total_comments: number; total_unique_viewers: number }> | null;
		content_stats: unknown[] | null;
		space_stats: unknown[] | null;
	};

	let overview = $state<OverviewData | null>(null);
	let security = $state<Array<Record<string, unknown>> | null>(null);
	let userAnalytics = $state<{ total_users?: number; active_users?: number; new_users?: number } | null>(null);
	let contentAnalytics = $state<{ content_stats?: Array<Record<string, unknown>>; total_content?: number; total_views?: number; total_reactions?: number; total_comments?: number; total_unique_viewers?: number } | null>(null);
	let spaceAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let revenueAnalytics = $state<{ total_revenue?: number; monthly_revenue?: number; active_subscriptions?: number; periods?: Array<{ period: string; revenue: number; transactions: number }> } | null>(null);
	let searchAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let scanning = $state(false);
	let loadAbortController: AbortController | null = null;
	let deleteModalOpen = $state(false);
	let pendingDeleteScan = $state<Record<string, unknown> | null>(null);
	let deleting = $state(false);
	let notice = $state("");

	// Server-paginated table state (content / spaces / security). The backend
	// returns { items, total } envelopes; sort is applied server-side via
	// allowlisted CASE ordering (?sort=&dir=).
	let contentPage = $state(0);
	let contentPageSize = $state(10);
	let contentTotal = $state(0);
	let contentSort = $state<{ key: string | null; dir: 'asc' | 'desc' }>({ key: null, dir: 'desc' });
	let spacePage = $state(0);
	let spacePageSize = $state(10);
	let spaceTotal = $state(0);
	let spaceSort = $state<{ key: string | null; dir: 'asc' | 'desc' }>({ key: null, dir: 'desc' });
	let spaceTotals = $state<{ total_spaces?: number; total_joins?: number; total_posts?: number } | null>(null);
	let secPage = $state(0);
	let secPageSize = $state(10);
	let secTotal = $state(0);
	let secSort = $state<{ key: string | null; dir: 'asc' | 'desc' }>({ key: null, dir: 'desc' });
	let secSeverity = $state("");
	let secUnresolvedOnly = $state(false);

	// monthly_revenue is the calendar month containing the range's `to` bound
	// (today when unset) — name that month in the card label so the number
	// can't be misread as "revenue across the whole selected range".
	const revenueMonthLabel = $derived(
		(to ? new Date(`${to}T00:00:00`) : new Date())
			.toLocaleDateString(undefined, { month: 'short', year: 'numeric' })
	);

	// NGN is the platform's settlement currency (payments.currency defaults to
	// NGN); render major units with the naira symbol rather than a hardcoded $.
	function fmtNaira(v: number | null | undefined): string {
		return `₦${Number(v ?? 0).toLocaleString()}`;
	}

	// Section 66 — Advanced Analytics state (ADR-084)
	let advancedPeriod = $state("7d");
	let topContent = $state<Array<Record<string, unknown>> | null>(null);
	let revenueStreams = $state<Array<Record<string, unknown>> | null>(null);
	let adPerformance = $state<Array<Record<string, unknown>> | null>(null);
	let newsletterEngagement = $state<{ campaigns?: Array<Record<string, unknown>>; lists?: Array<Record<string, unknown>> } | null>(null);
	let retentionCohorts = $state<Array<Record<string, unknown>> | null>(null);
	let advancedLoading = $state(false);
	let advancedError = $state("");
	let advType = $state("");
	// Set once the advanced tab has attempted its first fetch — the lazy-load
	// effect below must not refire on every state change (infinite loop when
	// any request fails, since a failed call leaves its slice null).
	let advancedFetched = $state(false);
	
	async function loadAnalytics(f = from, toVal = to) {
		if (loadAbortController) loadAbortController.abort();
		loadAbortController = new AbortController();
		const signal = loadAbortController.signal;

		loading = true;
		error = "";
		try {
		// Pass the ?from=&to= range to every windowed endpoint so the date
		// picker drives all tabs, not just the overview.
		const rangeParams = [f && `from=${f}`, toVal && `to=${toVal}`].filter(Boolean).join("&") || undefined;
		const [overviewRes, usersRes, revenueRes, searchRes] = await Promise.allSettled([
			analyticsAdminApi.getOverview(rangeParams, { signal }),
			analyticsAdminApi.getUsers(rangeParams, { signal }),
			analyticsAdminApi.getRevenue(rangeParams, { signal }),
			analyticsAdminApi.getSearch(rangeParams, { signal })
		]);
		// Server-paginated slices fetch independently so table controls can
		// refetch a single dataset without re-running the whole batch.
		await Promise.allSettled([
			loadSecurity(f, toVal, signal),
			loadContent(f, toVal, signal),
			loadSpaces(f, toVal, signal)
		]);

			if (signal.aborted) return;

			overview = overviewRes.status === 'fulfilled' && overviewRes.value.ok ? (overviewRes.value.data as OverviewData) : null;
			userAnalytics = usersRes.status === 'fulfilled' && usersRes.value.ok ? (usersRes.value.data as { total_users?: number; active_users?: number; new_users?: number }) : null;
			revenueAnalytics = revenueRes.status === 'fulfilled' && revenueRes.value.ok ? (revenueRes.value.data as { total_revenue?: number; monthly_revenue?: number; active_subscriptions?: number; periods?: Array<{ period: string; revenue: number; transactions: number }> }) : null;
			searchAnalytics = searchRes.status === 'fulfilled' && searchRes.value.ok ? (searchRes.value.data as Array<Record<string, unknown>>) : null;
		} catch (e) {
			if (signal.aborted) return;
			error = e instanceof Error ? e.message : t('analytics.failed_to_load', 'Failed to load analytics');
		} finally {
			if (!signal.aborted) loading = false;
		}
	}

	// Server-paginated loaders — each refetches only its own slice so a table
	// page/sort/filter change doesn't re-run the whole dashboard batch.
	function rangeQS(f = from, t = to): string {
		return [f && `from=${f}`, t && `to=${t}`].filter(Boolean).join("&");
	}

	async function loadSecurity(f = from, t = to, signal?: AbortSignal) {
		const params = [
			rangeQS(f, t),
			`page_size=${secPageSize}`,
			`offset=${secPage * secPageSize}`,
			secSort.key && `sort=${secSort.key}&dir=${secSort.dir}`,
			secSeverity && `severity=${secSeverity}`,
			secUnresolvedOnly && `unresolved=true`
		].filter(Boolean).join("&");
		const res = await analyticsAdminApi.getSecurity(params, { signal });
		if (res.ok) {
			const d = res.data as { items?: Array<Record<string, unknown>>; total?: number };
			security = d.items ?? [];
			secTotal = d.total ?? 0;
		} else if (!signal?.aborted) {
			error = res.error ?? 'Failed to load security findings';
		}
	}

	async function loadContent(f = from, t = to, signal?: AbortSignal) {
		const params = [
			rangeQS(f, t),
			`page_size=${contentPageSize}`,
			`offset=${contentPage * contentPageSize}`,
			contentSort.key && `sort=${contentSort.key}&dir=${contentSort.dir}`
		].filter(Boolean).join("&");
		const res = await analyticsAdminApi.getContent(params, { signal });
		if (res.ok) {
			contentAnalytics = res.data as typeof contentAnalytics;
			contentTotal = (res.data as { total?: number }).total ?? 0;
		} else if (!signal?.aborted) {
			error = res.error ?? 'Failed to load content analytics';
		}
	}

	async function loadSpaces(f = from, t = to, signal?: AbortSignal) {
		const params = [
			rangeQS(f, t),
			`page_size=${spacePageSize}`,
			`offset=${spacePage * spacePageSize}`,
			spaceSort.key && `sort=${spaceSort.key}&dir=${spaceSort.dir}`
		].filter(Boolean).join("&");
		const res = await analyticsAdminApi.getSpaces(params, { signal });
		if (res.ok) {
			const d = res.data as { items?: Array<Record<string, unknown>>; total?: number; total_spaces?: number; total_joins?: number; total_posts?: number };
			spaceAnalytics = d.items ?? [];
			spaceTotal = d.total ?? 0;
			spaceTotals = { total_spaces: d.total, total_joins: d.total_joins, total_posts: d.total_posts };
		} else if (!signal?.aborted) {
			error = res.error ?? 'Failed to load space analytics';
		}
	}

	// Section 66 — Load advanced analytics from the §66 backend routes.
	async function loadAdvancedAnalytics(period = advancedPeriod) {
		advancedLoading = true;
		advancedError = "";
		try {
			const periodParam = `period=${period}`;
			const results = await Promise.allSettled([
				analyticsAdminApi.getTopContent(`${periodParam}${advType ? `&type=${advType}` : ''}`),
				analyticsAdminApi.getRevenueStreams(periodParam),
				analyticsAdminApi.getAdPerformance(periodParam),
				analyticsAdminApi.getNewsletterEngagement(periodParam),
				analyticsAdminApi.getRetention(periodParam)
			]);
			const [topRes, revRes, adsRes, newsRes, retRes] = results;
			topContent = topRes.status === 'fulfilled' && topRes.value.ok ? (topRes.value.data as { content?: Array<Record<string, unknown>> })?.content ?? null : null;
			for (const r of topContent ?? []) {
				const ct = String(r.content_type ?? '');
				if (ct) seenContentTypes.add(ct);
			}
			seenContentTypes = new Set(seenContentTypes);
			revenueStreams = revRes.status === 'fulfilled' && revRes.value.ok ? (revRes.value.data as { revenue?: Array<Record<string, unknown>> })?.revenue ?? null : null;
			adPerformance = adsRes.status === 'fulfilled' && adsRes.value.ok ? (adsRes.value.data as { ads?: Array<Record<string, unknown>> })?.ads ?? null : null;
			newsletterEngagement = newsRes.status === 'fulfilled' && newsRes.value.ok ? (newsRes.value.data as { campaigns?: Array<Record<string, unknown>>; lists?: Array<Record<string, unknown>> }) : null;
			retentionCohorts = retRes.status === 'fulfilled' && retRes.value.ok ? (retRes.value.data as { cohorts?: Array<Record<string, unknown>> })?.cohorts ?? null : null;
			// Surface a hard failure when every advanced request failed so the
			// lazy-load effect stops retrying in a loop.
			if (results.every((r) => r.status === 'rejected' || !r.value.ok)) {
				advancedError = t('analytics.failed_to_load', 'Failed to load advanced analytics');
			}
		} catch (e) {
			advancedError = e instanceof Error ? e.message : t('analytics.failed_to_load', 'Failed to load advanced analytics');
		} finally {
			advancedLoading = false;
		}
	}

	// Section 66 — Export via the backend /admin/analytics/export route.
	// top_content honors the active content-type filter; other metrics
	// don't expose a type dimension so they export unfiltered.
	async function handleAdvancedExport(metric: string) {
		try {
			const res = await analyticsAdminApi.exportCsv(metric, advancedPeriod, metric === 'top_content' ? advType || undefined : undefined);
			if (!res.ok) throw new Error('Export failed');
			// The backend returns CSV text; trigger a download.
			const blob = new Blob([res.data as string], { type: 'text/csv' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `${metric}_export.csv`;
			a.click();
			URL.revokeObjectURL(url);
		} catch (e) {
			advancedError = e instanceof Error ? e.message : t('analytics.export_failed', 'Export failed');
		}
	}

	// Fetches only the newest finding — the probe the scan poller compares
	// against so detection doesn't depend on which page/sort is loaded.
	async function newestSecurityProbe(): Promise<{ id: string; total: number } | null> {
		const res = await analyticsAdminApi.getSecurity('page_size=1&sort=created_at&dir=desc');
		if (!res.ok) return null;
		const d = res.data as { items?: Array<Record<string, unknown>>; total?: number };
		return { id: String(d.items?.[0]?.id ?? ''), total: d.total ?? 0 };
	}

	async function runSecurityScan() {
		scanning = true;
		try {
			// Baseline newest + count BEFORE enqueueing — the poller stops when
			// either changes, so findings land detected regardless of which
			// page or sort order the table is currently showing.
			const baseline = await newestSecurityProbe();
			const res = await analyticsAdminApi.runSecurityScan({});
			if (!res.ok) throw new Error('Scan failed');
			// The scan runs as an Asynq task — poll until new findings land
			// (or ~20s elapses) instead of hoping a fixed sleep covers it.
			const deadline = Date.now() + 20000;
			for (;;) {
				await new Promise(r => setTimeout(r, 2000));
				const probe = await newestSecurityProbe();
				if (!probe || probe.id !== baseline?.id || probe.total !== baseline?.total || Date.now() > deadline) break;
			}
			await loadSecurity();
		} catch (e) {
			error = e instanceof Error ? e.message : t('analytics.scan_failed', 'Scan failed');
		} finally {
			scanning = false;
		}
	}

	async function resolveScan(id: string) {
		try {
			const res = await analyticsAdminApi.resolveSecurityItem(id, {});
			if (!res.ok) throw new Error(res.error ?? 'Resolve failed');
			// Refetch rather than patch locally — with the unresolved filter
			// active the row must leave the result set, and the total changes.
			await loadSecurity();
		} catch (e) {
			error = e instanceof Error ? e.message : t('analytics.resolve_failed', 'Failed to resolve');
		}
	}

	async function deleteScan() {
		const id = pendingDeleteScan?.id;
		if (id == null) return;
		deleting = true;
		try {
			const res = await analyticsAdminApi.deleteSecurityItem(String(id));
			if (!res.ok) throw new Error(res.error ?? 'Delete failed');
			deleteModalOpen = false;
			pendingDeleteScan = null;
			// Refetch — local filtering leaves the server total stale.
			await loadSecurity();
		} catch (e) {
			error = e instanceof Error ? e.message : t('analytics.delete_failed', 'Delete failed');
		} finally {
			deleting = false;
		}
	}

	// Row actions for the security findings table (resolve/delete are wired
	// to POST .../security/{id}/resolve and DELETE .../security/{id}, both
	// gated on the analytics.admin node — hide them without it).
	const securityActions = $derived([
		{
			label: t('analytics.resolve', 'Resolve'),
			onclick: (row: Record<string, unknown>) => resolveScan(String(row.id)),
			show: (row: Record<string, unknown>) => hasPermission('analytics.admin') && !row.resolved_at
		},
		{
			label: t('common.delete', 'Delete'),
			variant: 'destructive' as const,
			onclick: (row: Record<string, unknown>) => {
				pendingDeleteScan = row;
				deleteModalOpen = true;
			},
			show: () => hasPermission('analytics.admin')
		}
	]);

	// Push the chosen date range to the URL (?from=&to=) to drive the reload
	function applyRange() {
		const url = new URL(page.url.href);
		if (from) url.searchParams.set("from", from);
		else url.searchParams.delete("from");
		if (to) url.searchParams.set("to", to);
		else url.searchParams.delete("to");
		goto(url, { reset: false });
	}

	let lastRange = "";
	$effect(() => {
		// Re-sync controls from the URL and reload whenever the range changes
		const f = page.url.searchParams.get("from") ?? "";
		const t = page.url.searchParams.get("to") ?? "";
		from = f;
		to = t;
		const rangeKey = `${f}|${t}`;
		if (lastRange && lastRange !== rangeKey) {
			// The range actually changed — reset server pagination so a stale
			// page offset can't render an empty table on the narrower result.
			contentPage = 0;
			spacePage = 0;
			secPage = 0;
		}
		lastRange = rangeKey;
		loadAnalytics(f, t);

		return () => {
			if (loadAbortController) loadAbortController.abort();
		};
	});

	// Section 66 — Load advanced analytics once when the Advanced tab is
	// active. Retries go through the period selector or the error Retry
	// button (both call loadAdvancedAnalytics directly). The tab guard keeps
	// a deep-linked ?tab=advanced from firing §66 calls for operators
	// without analytics.advanced.
	$effect(() => {
		if (activeTab === "advanced" && !advancedFetched && !advancedLoading && hasPermission('analytics.advanced')) {
			advancedFetched = true;
			loadAdvancedAnalytics();
		}
	});

	// A deep-linked tab the operator can't see (e.g. ?tab=advanced without
	// analytics.advanced) falls back to Overview rather than rendering
	// permission errors.
	$effect(() => {
		if (activeTab === "advanced" && !hasPermission('analytics.advanced')) {
			activeTab = "overview";
		}
	});
	
	const tabs = $derived([
		{ value: "overview", label: t('analytics.tab_overview', 'Overview'), icon: TrendingUp },
		{ value: "users", label: t('analytics.tab_users', 'Users'), icon: Users },
		{ value: "content", label: t('analytics.tab_content', 'Content'), icon: MessageSquare },
		{ value: "spaces", label: t('analytics.tab_spaces', 'Spaces'), icon: Eye },
		{ value: "revenue", label: t('analytics.tab_revenue', 'Revenue'), icon: DollarSign },
		{ value: "search", label: t('analytics.tab_search', 'Search'), icon: Search },
		{ value: "security", label: t('analytics.tab_security', 'Security'), icon: Shield },
		// §66 endpoints require the analytics.advanced node — hide the tab
		// entirely for operators who would only see permission errors.
		...(hasPermission('analytics.advanced')
			? [{ value: "advanced", label: t('analytics.tab_advanced', 'Advanced'), icon: BarChart3 }]
			: [])
	]);
	
	// Analytics series arrive in mixed orders (DAU/content are newest-first,
	// user growth is oldest-first) — normalize ascending before taking the
	// "latest" element or feeding a chart, or cards show the oldest day.
	const sortAsc = <T>(arr: T[] | null | undefined, key: keyof T): T[] =>
		[...(arr ?? [])].sort((a, b) => String(a[key]).localeCompare(String(b[key])));

	// pctChange computes the percentage change between the last and
	// second-to-last values of a time series. Returns undefined when there
	// is not enough data or the previous value is zero (avoid div-by-zero).
	function pctChange<T>(series: T[] | null | undefined, pick: (item: T) => number): number | undefined {
		if (!series || series.length < 2) return undefined;
		const last = pick(series[series.length - 1]);
		const prev = pick(series[series.length - 2]);
		if (prev === 0) return undefined;
		return Math.round(((last - prev) / prev) * 100);
	}

	const overviewCards = $derived([
		{
			label: t('analytics.daily_active_users', 'Daily Active Users'),
			value: sortAsc(overview?.daily_active_users, 'day').at(-1)?.dau ?? 0,
			change: pctChange(sortAsc(overview?.daily_active_users, 'day'), (d) => d.dau),
			icon: Users,
			color: "primary"
		},
		{
			label: t('analytics.monthly_active_users', 'Monthly Active Users'),
			value: sortAsc(overview?.monthly_active_users, 'month').at(-1)?.mau ?? 0,
			change: pctChange(sortAsc(overview?.monthly_active_users, 'month'), (d) => d.mau),
			icon: Users,
			color: "success"
		},
		{
			label: t('analytics.new_signups', 'New Signups (Latest)'),
			value: sortAsc(overview?.user_growth, 'day').at(-1)?.new_users ?? 0,
			change: pctChange(sortAsc(overview?.user_growth, 'day'), (d) => d.new_users),
			icon: UserPlus,
			color: "info"
		},
		{
			label: t('analytics.total_spaces', 'Total Spaces'),
			value: overview?.space_stats ? new Set(overview.space_stats.map((s: any) => s.space_id)).size : 0,
			change: undefined,
			icon: Eye,
			color: "warning"
		},
		{
			label: t('analytics.content_count', 'Content Items'),
			value: overview?.content_over_time?.reduce((sum, d) => sum + d.content_count, 0) ?? 0,
			change: undefined,
			icon: MessageSquare,
			color: "primary"
		},
		{
			label: t('analytics.event_count', 'Events'),
			value: overview?.event_counts?.reduce((sum, e) => sum + e.count, 0) ?? 0,
			change: undefined,
			icon: TrendingUp,
			color: "success"
		}
	]);

	// Content-type options accumulate from every fetched leaderboard — deriving
	// from the *filtered* rows would collapse the picker to just the active
	// filter after the first selection.
	let seenContentTypes = new Set<string>();
	const advTypeOptions = $derived([
		{ value: '', label: t('analytics.all_types', 'All types') },
		...[...seenContentTypes].sort().map((ct) => ({ value: ct, label: ct }))
	]);

	// Top Content table columns for the Overview tab.
	const topContentColumns = $derived([
		{ key: "content_title", label: t('analytics.content_title', 'Content Title'), sortable: true, render: (v: string) => v ? `<span class="font-medium">${v}</span>` : '—' },
		{ key: "total_views", label: t('analytics.views', 'Views'), sortable: true },
		{ key: "total_reactions", label: t('analytics.reactions', 'Reactions'), sortable: true },
		{ key: "total_comments", label: t('analytics.comments', 'Comments'), sortable: true },
		{ key: "total_unique_viewers", label: t('analytics.unique_viewers', 'Unique Viewers'), sortable: true }
	]);

	// Map the user analytics summary object into metric/value rows for the
	// Users tab DataTable (backend returns an object, not an array).
	const userRows = $derived([
		{ metric: t('analytics.total_users', 'Total Users (all time)'), value: userAnalytics?.total_users ?? 0 },
		{ metric: t('analytics.active_users', 'Active Users (in range)'), value: userAnalytics?.active_users ?? 0 },
		{ metric: t('analytics.new_users_today', 'New Users (in range)'), value: userAnalytics?.new_users ?? 0 }
	]);

	// LayerChart Axis format: dates on the x-axis, numbers on the y-axis.
	// Typed `any` to satisfy the chart's union `axis` prop type.
	const chartAxisFormat: any = {
		format: (v: any) =>
			v instanceof Date
				? v.toLocaleDateString(locale.current, { month: 'short', day: 'numeric' })
				: Number(v).toLocaleString()
	};

	// Axis config for band-scale (string) x-axes — chartAxisFormat coerces
	// non-Date values through Number(), which renders NaN for string labels.
	const barChartAxis: any = {
		x: { format: (v: any) => String(v), ticks: { rotate: -30 } },
		y: { format: (v: any) => Number(v).toLocaleString() }
	};

	const securityColumns = $derived([
		{ key: "scan_type", label: t('analytics.type', 'Type'), sortable: true },
		{
			key: "severity",
			label: t('analytics.severity', 'Severity'),
			sortable: true,
			render: (value: string) => {
				const colors: Record<string, string> = { info: 'bg-info-100 text-info-800', warning: 'bg-warning/15 text-warning', low: 'bg-info-100 text-info-800', medium: 'bg-warning/15 text-warning', high: 'bg-danger/15 text-danger', critical: 'bg-danger text-white' };
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colors[value] || 'bg-surface text-foreground'}">${value}</span>`;
			}
		},
		{
			key: "created_at",
			label: t('analytics.date', 'Date'),
			sortable: true,
			render: (value: string) => value ? new Date(value).toLocaleString() : '—'
		},
		{ key: "finding", label: t('analytics.findings', 'Finding'), sortable: true },
		{
			key: "resolved_at",
			label: t('analytics.resolved', 'Resolved'),
			sortable: true,
			render: (value: string) => value ? new Date(value).toLocaleString() : `<span class="text-warning font-medium">${t('analytics.unresolved', 'Unresolved')}</span>`
		}
	]);

	async function handleFlushCache() {
		flushing = true;
		try {
			// "analytics" scope only — the backend clamps to jc:cache:analytics:*
			// regardless, but sending the right scope keeps intent explicit.
			const res = await analyticsAdminApi.flushCache({ scope: "analytics" });
			if (!res.ok) throw new Error(res.error ?? 'flush failed');
			flushModalOpen = false;
			const deleted = (res.data as { deleted?: number })?.deleted;
			notice = deleted != null
				? t('analytics.cache_flushed_count', 'Analytics cache flushed ({n} keys)').replace('{n}', String(deleted))
				: t('analytics.cache_flushed', 'Analytics cache flushed');
			setTimeout(() => { notice = ""; }, 6000);
			// Refetch in place — a goto back to /analytics would drop the
			// active ?tab and leave stale data (the load effect only fires
			// when ?from/?to change).
			await loadAnalytics();
			if (advancedFetched) await loadAdvancedAnalytics();
		} catch {
			error = t('analytics.failed_to_flush_cache', 'Failed to flush cache');
		} finally {
			flushing = false;
		}
	}

	// Enqueue a materialized-view refresh (POST /admin/analytics/refresh).
	async function handleRefreshViews() {
		const res = await analyticsAdminApi.refresh({});
		if (res.ok) {
			notice = t('analytics.refresh_queued', 'Analytics refresh queued');
			setTimeout(() => { notice = ""; }, 6000);
		} else {
			error = res.error ?? t('analytics.failed_to_load', 'Refresh failed');
		}
	}

	// PageHeader actions — Flush Cache is gated on the analytics.cache.flush
	// node, Refresh Views on analytics.admin, Export on analytics.export
	// (matching the backend route permissions).
	const pageActions = $derived([
		...(hasPermission('analytics.export')
			? [{ label: t('analytics.export_report', 'Export Report'), icon: Download, onclick: handleExport, variant: "outline" as const }]
			: []),
		...(hasPermission('analytics.admin')
			? [{ label: t('analytics.refresh_views', 'Refresh Views'), icon: RefreshCw, onclick: handleRefreshViews, variant: "ghost" as const }]
			: []),
		...(hasPermission('analytics.cache.flush')
			? [{ label: t('analytics.flush_cache', 'Flush Cache'), icon: RefreshCw, onclick: () => { flushModalOpen = true; }, variant: "ghost" as const }]
			: [])
	]);

	function handleExport() {
		// Build CSV from overview data. Values are neutralized against
		// spreadsheet formula injection (=, +, -, @, tab, pipe, DDE).
		const csvCell = (v: unknown): string => {
			let s = String(v ?? '');
			if (/^[=+\-@\t|]/.test(s) || /^cmd\|/i.test(s)) s = `'${s}`;
			return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
		};
		const rows: string[] = ['Metric,Value'];
		const range = [from && `from=${from}`, to && `to=${to}`].filter(Boolean).join(' ');
		if (range) rows.push(`${csvCell('Range')},${csvCell(range)}`);
		// Series arrive newest-first — sort ascending before taking "latest",
		// same normalization as the stat cards.
		const dau = sortAsc(overview?.daily_active_users, 'day').at(-1);
		if (dau) rows.push(`${csvCell('Daily Active Users')},${csvCell(dau.dau)}`);
		const mau = sortAsc(overview?.monthly_active_users, 'month').at(-1);
		if (mau) rows.push(`${csvCell('Monthly Active Users')},${csvCell(mau.mau)}`);
		const signups = sortAsc(overview?.user_growth, 'day').at(-1);
		if (signups) rows.push(`${csvCell('New Signups (Latest)')},${csvCell(signups.new_users)}`);
		if (overview?.content_over_time?.length) {
			const total = overview.content_over_time.reduce((sum, d) => sum + d.content_count, 0);
			rows.push(`${csvCell('Total Content Items')},${csvCell(total)}`);
		}
		if (overview?.event_counts?.length) {
			const total = overview.event_counts.reduce((sum, e) => sum + e.count, 0);
			rows.push(`${csvCell('Total Events')},${csvCell(total)}`);
		}
		if (userAnalytics) {
			rows.push(`${csvCell('Total Users')},${csvCell(userAnalytics.total_users)}`);
			rows.push(`${csvCell('Active Users')},${csvCell(userAnalytics.active_users)}`);
			rows.push(`${csvCell('New Users')},${csvCell(userAnalytics.new_users)}`);
		}
		if (revenueAnalytics) {
			rows.push(`${csvCell('Total Revenue (NGN)')},${csvCell(revenueAnalytics.total_revenue)}`);
			rows.push(`${csvCell(`Monthly Revenue (${revenueMonthLabel}, NGN)`)},${csvCell(revenueAnalytics.monthly_revenue)}`);
			rows.push(`${csvCell('Active Subscriptions')},${csvCell(revenueAnalytics.active_subscriptions)}`);
		}
		const unresolved = (security ?? []).filter((s) => !s.resolved_at).length;
		rows.push(`${csvCell('Unresolved Security Findings')},${csvCell(unresolved)}`);
		for (const q of (searchAnalytics ?? []).slice(0, 10)) {
			rows.push(`${csvCell(`Top Search: ${q.query}`)},${csvCell(q.count)}`);
		}
		const csv = rows.join('\n');
		const blob = new Blob([csv], { type: 'text/csv' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `analytics-report-${new Date().toISOString().slice(0, 10)}.csv`;
		a.click();
		URL.revokeObjectURL(url);
	}
</script>

<PageHeader
	title={t('analytics.page_title', 'Analytics')}
	description={t('analytics.page_description', 'Monitor platform performance and user engagement')}
	actions={pageActions}
/>

<div class="space-y-6">
	<!-- Date-range picker that writes ?from=&to= to the URL and refetches -->
	<div class="flex flex-wrap items-end gap-2">
		<div>
			<label for="analytics-from" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.from', 'From')}</label>
			<input
				id="analytics-from"
				type="date"
				bind:value={from}
				class="rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>
		<div>
			<label for="analytics-to" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.to', 'To')}</label>
			<input
				id="analytics-to"
				type="date"
				bind:value={to}
				class="rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>
		<Button variant="outline" size="sm" onclick={applyRange}>{t('common.apply', 'Apply')}</Button>
	</div>

	{#if error}
		<div class="rounded-lg p-3 text-sm" style="background: var(--color-danger); color: var(--color-white)">{error}</div>
	{/if}
	{#if notice}
		<div class="rounded-lg p-3 text-sm" style="background: var(--color-success); color: var(--color-white)">{notice}</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={['from', 'to']} />
	
	{#if loading}
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _, i (i)}
				<CardSkeleton />
			{/each}
		</div>
		<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
			{#each Array(2) as _, i (i)}
				<Card>
					<CardHeader>
						<div class="h-4 w-32 animate-pulse rounded" style="background: var(--color-surface)"></div>
					</CardHeader>
					<CardContent>
						<div class="h-64 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					</CardContent>
				</Card>
			{/each}
		</div>
	{:else if activeTab === "overview"}
		<!-- Stats Grid -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each overviewCards as card (card)}
				<StatCard label={card.label} value={card.value} delta={card.change} />
			{/each}
		</div>
		
		<!-- Charts -->
		<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.user_growth', 'User Growth')}</h3>
				</CardHeader>
				<CardContent>
					{#if loading}
						<div class="h-64 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					{:else if ((overview?.user_growth) ?? []).length > 0}
						<div class="h-64">
							<LineChart
								data={sortAsc(overview?.user_growth, 'day').map((d) => ({ date: new Date(d.day), count: d.new_users }))}
								x="date"
								y="count"
								cRange={['var(--chart-1)']}
								axis={chartAxisFormat}
								props={{
									spline: { curve: curveMonotoneX, strokeWidth: 2, stroke: 'var(--chart-1)' }
								}}
							/>
						</div>
					{:else}
						<div class="flex h-64 items-center justify-center">
							<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available', 'No data available')}</p>
						</div>
					{/if}
				</CardContent>
			</Card>
			
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.content_activity', 'Content Activity')}</h3>
				</CardHeader>
				<CardContent>
					{#if loading}
						<div class="h-64 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					{:else if ((overview?.content_over_time) ?? []).length > 0}
						<div class="h-64">
							<AreaChart
								data={sortAsc(overview?.content_over_time, 'day').map((d) => ({ date: new Date(d.day), count: d.content_count }))}
								x="date"
								y="count"
								cRange={['var(--chart-1)']}
								axis={chartAxisFormat}
								props={{
									area: { curve: curveMonotoneX, fill: 'var(--chart-1)', fillOpacity: 0.2, strokeWidth: 2, stroke: 'var(--chart-1)', line: { curve: curveMonotoneX } }
								}}
							/>
						</div>
					{:else}
						<div class="flex h-64 items-center justify-center">
							<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available', 'No data available')}</p>
						</div>
					{/if}
				</CardContent>
			</Card>
		</div>

		<!-- Traffic Sources + Top Content (data the backend already returns) -->
		<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.traffic_sources', 'Traffic Sources')}</h3>
				</CardHeader>
				<CardContent>
					{#if loading}
						<div class="h-48 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					{:else if (overview?.traffic_sources ?? []).length > 0}
						<div class="h-48">
							<BarChart
								data={(overview?.traffic_sources ?? []).map((d) => ({ source: d.referrer ?? t('analytics.direct', 'Direct'), visits: d.visits }))}
								x="source"
								y="visits"
								xPadding={[0.1, 0.1]}
								cRange={['var(--chart-2)']}
								axis={barChartAxis}
							/>
						</div>
					{:else}
						<div class="flex h-48 items-center justify-center">
							<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available', 'No data available')}</p>
						</div>
					{/if}
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.top_content', 'Top Content')}</h3>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={topContentColumns}
						data={overview?.top_content ?? []}
						pageSize={5}
						bulkExport={false}
						emptyMessage={t('analytics.no_top_content', 'No top content data')}
					/>
				</CardContent>
			</Card>
		</div>

		{#if (overview?.event_counts ?? []).length > 0}
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.events_by_type', 'Events by Type')}</h3>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={[
							{ key: "event_type", label: t('analytics.type', 'Type'), sortable: true },
							{ key: "count", label: t('analytics.count', 'Count'), sortable: true, render: (v: number) => Number(v).toLocaleString() }
						]}
						data={[...(overview?.event_counts ?? [])].sort((a, b) => b.count - a.count)}
						pageSize={8}
						urlSync={false}
						rowKey={(r) => String(r.event_type)}
						bulkExport={false}
						emptyMessage={t('analytics.no_data_available', 'No data available')}
					/>
				</CardContent>
			</Card>
		{/if}
	{/if}

	{#if !loading && activeTab === "security"}
		<Card>
			<CardHeader>
				<div class="flex items-center justify-between gap-2 flex-wrap">
					<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.security_scans', 'Security Scans')}</h3>
					<div class="flex items-center gap-2 flex-wrap">
						<AutocompleteSelect
							id="security-severity"
							value={secSeverity}
							onValueChange={(v) => { secSeverity = v; secPage = 0; loadSecurity(); }}
							options={[
								{ value: '', label: t('analytics.all_severities', 'All severities') },
								{ value: 'info', label: 'Info' },
								{ value: 'warning', label: 'Warning' },
								{ value: 'low', label: 'Low' },
								{ value: 'medium', label: 'Medium' },
								{ value: 'high', label: 'High' },
								{ value: 'critical', label: 'Critical' }
							]}
						/>
						<AutocompleteSelect
							id="security-status"
							value={secUnresolvedOnly ? 'unresolved' : 'all'}
							onValueChange={(v) => { secUnresolvedOnly = v === 'unresolved'; secPage = 0; loadSecurity(); }}
							options={[
								{ value: 'all', label: t('analytics.all_findings', 'All findings') },
								{ value: 'unresolved', label: t('analytics.unresolved', 'Unresolved') }
							]}
						/>
						<Button onclick={runSecurityScan} disabled={scanning || !hasPermission('analytics.admin')}>
							{#if scanning}
								<RefreshCw class="me-2 h-4 w-4 animate-spin" />
								{t('analytics.scanning', 'Scanning...')}
							{:else}
								<Shield class="me-2 h-4 w-4" />
								{t('analytics.run_scan', 'Run Scan')}
							{/if}
						</Button>
					</div>
				</div>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={securityColumns}
					data={security ?? []}
					serverSide
					urlSync={false}
					bind:page={secPage}
					bind:pageSize={secPageSize}
					totalRows={secTotal}
					rowKey={(r) => String(r.id)}
					onsortchange={(s) => { secSort = { key: s.key, dir: s.dir }; secPage = 0; loadSecurity(); }}
					onpagechange={() => loadSecurity()}
					onpagesizechange={() => { secPage = 0; loadSecurity(); }}
					actions={securityActions}
					emptyMessage={t('analytics.no_security_scans', 'No security scans found. Click "Run Scan" to start a security scan.')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if !loading && activeTab === "users"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.user_analytics', 'User Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "metric", label: t('analytics.metric', 'Metric'), sortable: true },
						{ key: "value", label: t('analytics.value', 'Value'), sortable: true, render: (v: number) => Number(v).toLocaleString() }
					]}
					data={userRows}
					bulkExport={false}
					emptyMessage={t('analytics.no_user_analytics', 'No user analytics data')}
				/>
			</CardContent>
		</Card>
		<Card>
			<CardHeader>
				<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.user_growth', 'User Growth')}</h3>
			</CardHeader>
			<CardContent>
				{#if (overview?.user_growth ?? []).length > 0}
					<div class="h-64">
						<LineChart
							data={sortAsc(overview?.user_growth, 'day').map((d) => ({ date: new Date(d.day), count: d.new_users }))}
							x="date"
							y="count"
							cRange={['var(--chart-1)']}
							axis={chartAxisFormat}
							props={{ spline: { curve: curveMonotoneX, strokeWidth: 2, stroke: 'var(--chart-1)' } }}
						/>
					</div>
				{:else}
					<div class="flex h-64 items-center justify-center">
						<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available', 'No data available')}</p>
					</div>
				{/if}
			</CardContent>
		</Card>
	{/if}
	
	{#if !loading && activeTab === "content"}
		<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
			<StatCard label={t('analytics.total_content', 'Content in Range')} value={contentAnalytics?.total_content ?? 0} />
			<StatCard label={t('analytics.views', 'Views')} value={contentAnalytics?.total_views ?? 0} />
			<StatCard label={t('analytics.reactions', 'Reactions')} value={contentAnalytics?.total_reactions ?? 0} />
			<StatCard label={t('analytics.comments', 'Comments')} value={contentAnalytics?.total_comments ?? 0} />
			<StatCard label={t('analytics.unique_viewers', 'Unique Viewers')} value={contentAnalytics?.total_unique_viewers ?? 0} />
		</div>
		<p class="text-xs" style="color: var(--color-muted)">
			{t('analytics.lifetime_stats_note', 'Rows and totals cover content created inside the selected range; engagement figures are lifetime counters on those items.')}
		</p>
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.content_analytics', 'Content Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "day", label: t('analytics.day', 'Day'), sortable: true, render: (v: string) => v ? new Date(v).toLocaleDateString() : '—' },
						{ key: "content_title", label: t('analytics.content_title', 'Content Title'), sortable: true, render: (v: string) => v ? `<span class="font-medium">${v}</span>` : '—' },
						{ key: "views", label: t('analytics.views', 'Views'), sortable: true },
						{ key: "reactions", label: t('analytics.reactions', 'Reactions'), sortable: true },
						{ key: "comments", label: t('analytics.comments', 'Comments'), sortable: true },
						{ key: "unique_viewers", label: t('analytics.unique_viewers', 'Unique Viewers'), sortable: true }
					]}
					data={contentAnalytics?.content_stats ?? []}
					serverSide
					bind:page={contentPage}
					bind:pageSize={contentPageSize}
					totalRows={contentTotal}
					rowKey={(r) => String(r.content_id)}
					urlSync={false}
					onpagechange={() => loadContent()}
					onpagesizechange={() => { contentPage = 0; loadContent(); }}
					onsortchange={(s) => { contentSort = { key: s.key, dir: s.dir }; contentPage = 0; loadContent(); }}
					emptyMessage={t('analytics.no_content_analytics', 'No content analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if !loading && activeTab === "spaces"}
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<StatCard label={t('analytics.spaces_in_range', 'Spaces Created in Range')} value={spaceTotals?.total_spaces ?? 0} />
			<StatCard label={t('analytics.total_joins', 'Joins (lifetime)')} value={spaceTotals?.total_joins ?? 0} />
			<StatCard label={t('analytics.total_posts', 'Posts (lifetime)')} value={spaceTotals?.total_posts ?? 0} />
		</div>
		<p class="text-xs" style="color: var(--color-muted)">
			{t('analytics.lifetime_stats_note', 'Rows and totals cover spaces created inside the selected range; join/post figures are lifetime counters on those spaces.')}
		</p>
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.space_analytics', 'Space Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "space_name", label: t('analytics.space_name', 'Space Name'), sortable: true },
						{ key: "total_joins", label: t('analytics.total_joins', 'Total Joins'), sortable: true },
						{ key: "total_posts", label: t('analytics.total_posts', 'Total Posts'), sortable: true }
					]}
					data={spaceAnalytics ?? []}
					serverSide
					bind:page={spacePage}
					bind:pageSize={spacePageSize}
					totalRows={spaceTotal}
					rowKey={(r) => String(r.space_id)}
					urlSync={false}
					onpagechange={() => loadSpaces()}
					onpagesizechange={() => { spacePage = 0; loadSpaces(); }}
					onsortchange={(s) => { spaceSort = { key: s.key, dir: s.dir }; spacePage = 0; loadSpaces(); }}
					emptyMessage={t('analytics.no_space_analytics', 'No space analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if !loading && activeTab === "revenue"}
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<StatCard label={t('analytics.total_revenue', 'Total Revenue')} value={fmtNaira(revenueAnalytics?.total_revenue)} />
			<StatCard label={`${t('analytics.monthly_revenue', 'Monthly Revenue')} · ${revenueMonthLabel}`} value={fmtNaira(revenueAnalytics?.monthly_revenue)} />
			<StatCard label={t('analytics.active_subscriptions', 'Active Subscriptions')} value={revenueAnalytics?.active_subscriptions ?? 0} />
		</div>
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.revenue_analytics', 'Revenue Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "period", label: t('analytics.period', 'Period'), sortable: true },
						{ key: "revenue", label: t('analytics.revenue', 'Revenue'), sortable: true, render: (v: number) => fmtNaira(v) },
						{ key: "transactions", label: t('analytics.transactions', 'Transactions'), sortable: true }
					]}
					data={revenueAnalytics?.periods ?? []}
					bulkExport={false}
					emptyMessage={t('analytics.no_revenue_analytics', 'No revenue analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if !loading && activeTab === "search"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.search_analytics', 'Search Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "query", label: t('analytics.query', 'Query'), sortable: true },
						{ key: "count", label: t('analytics.count', 'Count'), sortable: true },
						{ key: "avg_results", label: t('analytics.avg_results', 'Avg Results'), sortable: true },
						{ key: "zero_result_count", label: t('analytics.zero_result', 'Zero Results'), sortable: true,
							render: (v: number) => Number(v ?? 0) > 0 ? `<span class="font-medium" style="color: var(--color-warning)">${v}</span>` : '0' }
					]}
					data={searchAnalytics ?? []}
					rowKey={(r) => String(r.query)}
					emptyMessage={t('analytics.no_search_analytics', 'No search analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}

	{#if !loading && activeTab === "advanced"}
		<!-- Section 66 — Advanced Analytics Dashboard (ADR-084) -->
		<div class="flex flex-wrap items-center gap-2">
			<label for="advanced-period" class="text-xs font-medium" style="color: var(--color-muted)">{t('analytics.period', 'Period')}</label>
			<AutocompleteSelect
				id="advanced-period"
				value={advancedPeriod}
				onValueChange={(v) => { advancedPeriod = v; loadAdvancedAnalytics(v); }}
				options={[{ value: '7d', label: '7d' }, { value: '30d', label: '30d' }, { value: '90d', label: '90d' }]}
			/>
			<AutocompleteSelect
				id="advanced-type"
				value={advType}
				onValueChange={(v) => { advType = v; loadAdvancedAnalytics(); }}
				options={advTypeOptions}
			/>
			{#if hasPermission('analytics.export')}
				<div class="ms-auto flex gap-2">
					<Button variant="outline" size="sm" onclick={() => handleAdvancedExport('top_content')}>{t('analytics.export_top_content', 'Export Top Content')}</Button>
					<Button variant="outline" size="sm" onclick={() => handleAdvancedExport('revenue_streams')}>{t('analytics.export_revenue', 'Export Revenue')}</Button>
					<Button variant="outline" size="sm" onclick={() => handleAdvancedExport('ad_performance')}>{t('analytics.export_ads', 'Export Ads')}</Button>
					<Button variant="outline" size="sm" onclick={() => handleAdvancedExport('newsletter')}>{t('analytics.export_newsletter', 'Export Newsletter')}</Button>
					<Button variant="outline" size="sm" onclick={() => handleAdvancedExport('retention')}>{t('analytics.export_retention', 'Export Retention')}</Button>
				</div>
			{/if}
		</div>

		{#if advancedError}
			<div class="flex items-center justify-between gap-2 rounded-lg p-3 text-sm" style="background: var(--color-danger); color: var(--color-white)">
				<span>{advancedError}</span>
				<Button variant="outline" size="sm" onclick={() => loadAdvancedAnalytics()}>{t('common.retry', 'Retry')}</Button>
			</div>
		{/if}

		{#if advancedLoading}
			<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
				{#each Array(4) as _, i (i)}
					<CardSkeleton />
				{/each}
			</div>
		{:else}
			<!-- Top Content Leaderboard -->
			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.top_content', 'Top Content')}</h3>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={[
							{ key: "title", label: t('analytics.content_title', 'Title'), sortable: true, render: (v: string) => v ? `<span class="font-medium">${v}</span>` : '—' },
							{ key: "content_type", label: t('analytics.type', 'Type'), sortable: true },
							{ key: "total_views", label: t('analytics.views', 'Views'), sortable: true },
							{ key: "total_reactions", label: t('analytics.reactions', 'Reactions'), sortable: true },
							{ key: "total_comments", label: t('analytics.comments', 'Comments'), sortable: true },
							{ key: "engagement_score", label: t('analytics.engagement_score', 'Engagement Score'), sortable: true }
						]}
						data={topContent ?? []}
						pageSize={10}
						urlSync={false}
						rowKey={(r) => String(r.content_id)}
						emptyMessage={t('analytics.no_top_content', 'No top content data')}
					/>
				</CardContent>
			</Card>

			<!-- Revenue Stream Breakdown -->
			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.revenue_streams', 'Revenue Streams')}</h3>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={[
							{ key: "month", label: t('analytics.month', 'Month'), sortable: true, render: (v: string) => v ? new Date(v).toLocaleDateString(locale.current, { year: 'numeric', month: 'short' }) : '—' },
							{ key: "subscription_revenue", label: t('analytics.subscription', 'Subscription'), sortable: true, render: (v: number) => fmtNaira(v) },
							{ key: "course_revenue", label: t('analytics.course', 'Course'), sortable: true, render: (v: number) => fmtNaira(v) },
							{ key: "ad_revenue", label: t('analytics.ad_revenue', 'Ads'), sortable: true, render: (v: number) => fmtNaira(v) },
							{ key: "credit_revenue", label: t('analytics.credits', 'Credits'), sortable: true, render: (v: number) => fmtNaira(v) }
						]}
						data={revenueStreams ?? []}
						pageSize={12}
						urlSync={false}
						rowKey={(r) => String(r.month)}
						bulkExport={false}
						emptyMessage={t('analytics.no_revenue_streams', 'No revenue stream data')}
					/>
				</CardContent>
			</Card>

			<!-- Ad Performance -->
			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.ad_performance', 'Ad Performance')}</h3>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={[
							{ key: "campaign_name", label: t('analytics.campaign', 'Campaign'), sortable: true },
							{ key: "status", label: t('analytics.status', 'Status'), sortable: true },
							{ key: "impressions", label: t('analytics.impressions', 'Impressions'), sortable: true },
							{ key: "clicks", label: t('analytics.clicks', 'Clicks'), sortable: true },
							{ key: "ctr", label: t('analytics.ctr', 'CTR %'), sortable: true },
							{ key: "spent", label: t('analytics.spent_credits', 'Spent (credits)'), sortable: true, render: (v: number) => `${Number(v ?? 0).toLocaleString()}` }
						]}
						data={adPerformance ?? []}
						pageSize={10}
						urlSync={false}
						rowKey={(r) => String(r.campaign_id)}
						emptyMessage={t('analytics.no_ad_performance', 'No ad performance data')}
					/>
				</CardContent>
			</Card>

			<!-- Newsletter Engagement -->
			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.newsletter_engagement', 'Newsletter Engagement')}</h3>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={[
							{ key: "campaign_name", label: t('analytics.campaign', 'Campaign'), sortable: true },
							{ key: "total_sent", label: t('analytics.sent', 'Sent'), sortable: true },
							{ key: "total_opened", label: t('analytics.opened', 'Opened'), sortable: true },
							{ key: "open_rate", label: t('analytics.open_rate', 'Open Rate %'), sortable: true },
							{ key: "total_clicked", label: t('analytics.clicked', 'Clicked'), sortable: true },
							{ key: "click_rate", label: t('analytics.click_rate', 'Click Rate %'), sortable: true }
						]}
						data={newsletterEngagement?.campaigns ?? []}
						pageSize={10}
						urlSync={false}
						rowKey={(r) => String(r.campaign_id)}
						emptyMessage={t('analytics.no_newsletter_engagement', 'No newsletter engagement data')}
					/>
					{#if (newsletterEngagement?.lists ?? []).length > 0}
						<h4 class="mb-2 mt-6 text-sm font-medium" style="color: var(--color-muted)">{t('analytics.top_lists', 'Top Lists')}</h4>
						<DataTable
							columns={[
								{ key: "list_name", label: t('analytics.name', 'Name'), sortable: true },
								{ key: "subscriber_count", label: t('analytics.subscribers', 'Subscribers'), sortable: true },
								{ key: "new_subscribers", label: t('analytics.new_in_period', 'New (period)'), sortable: true }
							]}
							data={newsletterEngagement?.lists ?? []}
							pageSize={5}
							urlSync={false}
							rowKey={(r) => String(r.list_id)}
							bulkExport={false}
							emptyMessage={t('analytics.no_data_available', 'No data available')}
						/>
					{/if}
				</CardContent>
			</Card>

			<!-- Retention Cohorts -->
			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.retention_cohorts', 'Retention Cohorts')}</h3>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={[
							{ key: "cohort_date", label: t('analytics.cohort_date', 'Cohort Date'), sortable: true, render: (v: string) => v ? new Date(v).toLocaleDateString() : '—' },
							{ key: "cohort_size", label: t('analytics.cohort_size', 'Cohort Size'), sortable: true },
							{ key: "retained_d1", label: 'D1', sortable: true },
							{ key: "d1_rate", label: 'D1 %', sortable: true },
							{ key: "retained_d7", label: 'D7', sortable: true },
							{ key: "d7_rate", label: 'D7 %', sortable: true },
							{ key: "retained_d30", label: 'D30', sortable: true },
							{ key: "d30_rate", label: 'D30 %', sortable: true }
						]}
						data={retentionCohorts ?? []}
						pageSize={12}
						urlSync={false}
						rowKey={(r) => String(r.cohort_date)}
						bulkExport={false}
						emptyMessage={t('analytics.no_retention_cohorts', 'No retention cohort data')}
					/>
				</CardContent>
			</Card>
		{/if}
	{/if}
</div>

<Modal
	bind:open={flushModalOpen}
	title={t('analytics.flush_analytics_cache', 'Flush Analytics Cache')}
	description={t('analytics.flush_cache_description', 'This will clear all cached analytics data. The cache will rebuild automatically.')}
	size="sm"
>
	<div class="flex justify-end gap-3">
		<Button variant="outline" onclick={() => flushModalOpen = false}>{t('analytics.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={handleFlushCache} disabled={flushing}>
			{#if flushing}
				<RefreshCw class="h-4 w-4 me-2 animate-spin" />
				{t('analytics.flushing', 'Flushing...')}
			{:else}
				{t('analytics.flush_cache', 'Flush Cache')}
			{/if}
		</Button>
	</div>
</Modal>

<Modal
	bind:open={deleteModalOpen}
	title={t('analytics.delete_finding', 'Delete Finding')}
	description={t('analytics.delete_finding_description', 'This will permanently delete this security finding.')}
	size="sm"
>
	<div class="flex justify-end gap-3">
		<Button variant="outline" onclick={() => { deleteModalOpen = false; pendingDeleteScan = null; }}>{t('analytics.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={deleteScan} disabled={deleting}>
			{deleting ? t('analytics.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>
