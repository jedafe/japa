<script lang="ts">
	import { t, locale } from '#lib/stores/i18n.svelte.js';
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import PageHeader from "#lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "#lib/components/ui/card/index.js";
	import { Tabs } from "#lib/components/ui/tabs/index.js";
	import { DataTable } from "#lib/components/ui/data-table/index.js";
	import { Button } from "#lib/components/ui/button/index.js";
	import { hasPermission } from "#lib/stores/permissions.svelte.js";
	import {
		BarChart3, Download, ArrowLeft, DollarSign,
		Megaphone, Mail, Repeat2, ListOrdered
	} from "lucide-svelte";
	import { analyticsAdminApi } from "#lib/api.js";
	import { CardSkeleton } from "#lib/components/ui/skeleton/index.js";
	import AutocompleteSelect from '#lib/components/ui/AutocompleteSelect.svelte';

	// Section 66 — Advanced Analytics Dashboard (ADR-084). Dedicated route so
	// each dataset gets its own tab + lazy fetch instead of sharing the main
	// /analytics tab state.
	let activeTab = $state("top_content");

	const PERIODS = ['7d', '30d', '90d'];
	const urlPeriod = page.url.searchParams.get("period") ?? "";
	let period = $state(PERIODS.includes(urlPeriod) ? urlPeriod : "7d");
	let advType = $state(page.url.searchParams.get("type") ?? "");

	let topContent = $state<Array<Record<string, unknown>> | null>(null);
	let revenueStreams = $state<Array<Record<string, unknown>> | null>(null);
	let adPerformance = $state<Array<Record<string, unknown>> | null>(null);
	let newsletterEngagement = $state<{ campaigns?: Array<Record<string, unknown>>; lists?: Array<Record<string, unknown>> } | null>(null);
	let retentionCohorts = $state<Array<Record<string, unknown>> | null>(null);
	let tabLoading = $state(false);
	let tabError = $state("");
	// Datasets already fetched with the current period/type — cleared whenever
	// either filter changes so each tab refetches on next activation.
	let loadedTabs = $state(new Set<string>());

	// Content-type options accumulate from every fetched leaderboard — deriving
	// from the *filtered* rows would collapse the picker to just the active
	// filter after the first selection.
	let seenContentTypes = new Set<string>();
	const advTypeOptions = $derived([
		{ value: '', label: t('analytics.all_types', 'All types') },
		...[...seenContentTypes].sort().map((ct) => ({ value: ct, label: ct }))
	]);

	// NGN is the platform's settlement currency (payments.currency defaults to
	// NGN); render major units with the naira symbol rather than a hardcoded $.
	function fmtNaira(v: number | null | undefined): string {
		return `₦${Number(v ?? 0).toLocaleString()}`;
	}

	const tabs = $derived([
		{ value: "top_content", label: t('analytics.top_content', 'Top Content'), icon: ListOrdered },
		{ value: "revenue_streams", label: t('analytics.revenue_streams', 'Revenue Streams'), icon: DollarSign },
		{ value: "ads", label: t('analytics.ad_performance', 'Ad Performance'), icon: Megaphone },
		{ value: "newsletter", label: t('analytics.newsletter_engagement', 'Newsletter'), icon: Mail },
		{ value: "retention", label: t('analytics.retention_cohorts', 'Retention'), icon: Repeat2 }
	]);

	async function fetchDataset(tab: string) {
		const p = `period=${period}`;
		switch (tab) {
			case "top_content": {
				const res = await analyticsAdminApi.getTopContent(`${p}${advType ? `&type=${advType}` : ''}`);
				topContent = res.ok ? (res.data as { content?: Array<Record<string, unknown>> })?.content ?? [] : null;
				for (const r of topContent ?? []) {
					const ct = String(r.content_type ?? '');
					if (ct) seenContentTypes.add(ct);
				}
				seenContentTypes = new Set(seenContentTypes);
				return res.ok;
			}
			case "revenue_streams": {
				const res = await analyticsAdminApi.getRevenueStreams(p);
				revenueStreams = res.ok ? (res.data as { revenue?: Array<Record<string, unknown>> })?.revenue ?? [] : null;
				return res.ok;
			}
			case "ads": {
				const res = await analyticsAdminApi.getAdPerformance(p);
				adPerformance = res.ok ? (res.data as { ads?: Array<Record<string, unknown>> })?.ads ?? [] : null;
				return res.ok;
			}
			case "newsletter": {
				const res = await analyticsAdminApi.getNewsletterEngagement(p);
				newsletterEngagement = res.ok ? (res.data as { campaigns?: Array<Record<string, unknown>>; lists?: Array<Record<string, unknown>> }) : null;
				return res.ok;
			}
			case "retention": {
				const res = await analyticsAdminApi.getRetention(p);
				retentionCohorts = res.ok ? (res.data as { cohorts?: Array<Record<string, unknown>> })?.cohorts ?? [] : null;
				return res.ok;
			}
			default:
				return true;
		}
	}

	async function loadDataset(tab = activeTab) {
		tabLoading = true;
		tabError = "";
		const ok = await fetchDataset(tab).catch(() => false);
		tabLoading = false;
		if (ok) {
			loadedTabs = new Set(loadedTabs).add(tab);
		} else {
			tabError = t('analytics.failed_to_load', 'Failed to load advanced analytics');
		}
	}

	// Lazy per-tab fetch — only the active dataset is requested; the set is
	// reset by filter changes below so stale views can't linger.
	$effect(() => {
		if (!loadedTabs.has(activeTab) && !tabLoading) {
			loadDataset(activeTab);
		}
	});

	// Reflect period/type in the URL so filtered views are shareable.
	function syncFilterUrl() {
		const url = new URL(page.url.href);
		url.searchParams.set("period", period);
		if (advType) url.searchParams.set("type", advType);
		else url.searchParams.delete("type");
		goto(url, { reset: false });
	}

	function changePeriod(v: string) {
		period = v;
		loadedTabs = new Set();
		syncFilterUrl();
	}

	function changeType(v: string) {
		advType = v;
		loadedTabs = new Set();
		syncFilterUrl();
	}

	// Map the active tab to the backend export metric name.
	const exportMetric = $derived(
		({ top_content: 'top_content', revenue_streams: 'revenue_streams', ads: 'ad_performance', newsletter: 'newsletter', retention: 'retention' } as Record<string, string>)[activeTab] ?? 'top_content'
	);

	// Export via the backend /admin/analytics/export route. top_content honors
	// the active content-type filter; other metrics have no type dimension.
	async function handleExport() {
		try {
			const res = await analyticsAdminApi.exportCsv(exportMetric, period, exportMetric === 'top_content' ? advType || undefined : undefined);
			if (!res.ok) throw new Error('Export failed');
			const blob = new Blob([res.data as string], { type: 'text/csv' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `${exportMetric}_export.csv`;
			a.click();
			URL.revokeObjectURL(url);
		} catch (e) {
			tabError = e instanceof Error ? e.message : t('analytics.export_failed', 'Export failed');
		}
	}

	const pageActions = $derived([
		{
			label: t('analytics.back_to_analytics', 'Back to Analytics'),
			icon: ArrowLeft,
			onclick: () => goto('/analytics'),
			variant: "ghost" as const
		},
		...(hasPermission('analytics.export')
			? [{ label: t('analytics.export_report', 'Export Report'), icon: Download, onclick: handleExport, variant: "outline" as const }]
			: [])
	]);
</script>

<PageHeader
	title={t('analytics.advanced_title', 'Advanced Analytics')}
	description={t('analytics.advanced_description', 'Deep-dive content, revenue, advertising, newsletter, and retention metrics')}
	actions={pageActions}
/>

<div class="space-y-6">
	<div class="flex flex-wrap items-center gap-2">
		<label for="advanced-period" class="text-xs font-medium" style="color: var(--color-muted)">{t('analytics.period', 'Period')}</label>
		<AutocompleteSelect
			id="advanced-period"
			value={period}
			onValueChange={changePeriod}
			options={PERIODS.map((p) => ({ value: p, label: p }))}
		/>
		<AutocompleteSelect
			id="advanced-type"
			value={advType}
			onValueChange={changeType}
			options={advTypeOptions}
		/>
	</div>

	<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={['period', 'type']} />

	{#if tabError}
		<div class="flex items-center justify-between gap-2 rounded-lg p-3 text-sm" style="background: var(--color-danger); color: var(--color-white)">
			<span>{tabError}</span>
			<Button variant="outline" size="sm" onclick={() => loadDataset()}>{t('common.retry', 'Retry')}</Button>
		</div>
	{/if}

	{#if tabLoading}
		<CardSkeleton />
	{:else if activeTab === "top_content"}
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
	{:else if activeTab === "revenue_streams"}
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
	{:else if activeTab === "ads"}
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
	{:else if activeTab === "newsletter"}
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
	{:else if activeTab === "retention"}
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
</div>
