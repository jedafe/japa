<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw } from "lucide-svelte";
	import { universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";

	type RevenueSplit = {
		id: string;
		instructor_id: string;
		track_id: string;
		track_title?: string;
		source: "purchase" | "pro_stream";
		gross_kobo: number;
		platform_commission_kobo: number;
		instructor_net_kobo: number;
		payout_id?: string | null;
		created_at: string;
	};

	let splits = $state<RevenueSplit[]>([]);
	let loading = $state(true);
	let errorMessage = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let dateFrom = $state("");
	let dateTo = $state("");

	const sourceFilters = [
		{
			key: "source",
			label: t("university.source", "Source"),
			options: [
				{ value: "purchase", label: t("university.source_purchase", "Purchase") },
				{ value: "pro_stream", label: t("university.source_pro_stream", "Pro Stream") }
			]
		}
	];

	function formatKobo(kobo: number): string {
		const major = kobo / 100;
		return `₦ ${major.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
	}

	const columns = [
		{
			key: "instructor_id",
			label: t("university.instructor_id", "Instructor ID"),
			render: (v: string) =>
				`<span class="font-mono text-xs" style="color: var(--color-muted)">${v.slice(0, 8)}...</span>`
		},
		{
			key: "track_title",
			label: t("university.track_title", "Track"),
			render: (v: string) => `<span class="text-sm">${v ?? "—"}</span>`
		},
		{
			key: "source",
			label: t("university.source", "Source"),
			sortable: true,
			componentSnippet: SourceBadge
		},
		{
			key: "gross_kobo",
			label: t("university.gross", "Gross"),
			sortable: true,
			render: (v: number) =>
				`<span class="text-sm font-medium">${formatKobo(v)}</span>`
		},
		{
			key: "platform_commission_kobo",
			label: t("university.commission", "Commission"),
			render: (v: number) =>
				`<span class="text-xs" style="color: var(--color-muted)">${formatKobo(v)}</span>`
		},
		{
			key: "instructor_net_kobo",
			label: t("university.net", "Instructor Net"),
			sortable: true,
			render: (v: number) =>
				`<span class="text-sm font-semibold" style="color: var(--color-success)">${formatKobo(v)}</span>`
		},
		{
			key: "payout_id",
			label: t("university.payout_id", "Payout ID"),
			render: (v: string | null | undefined) =>
				v
					? `<span class="font-mono text-xs" style="color: var(--color-muted)">${v.slice(0, 8)}...</span>`
					: `<span class="text-xs" style="color: var(--color-muted)">—</span>`
		},
		{
			key: "created_at",
			label: t("common.created", "Created"),
			sortable: true,
			render: (v: string) =>
				`<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>`
		}
	];

	const filtered = $derived(
		searchQuery
			? splits.filter(
					(s) =>
						s.instructor_id.toLowerCase().includes(searchQuery.toLowerCase()) ||
						(s.track_title ?? "").toLowerCase().includes(searchQuery.toLowerCase()) ||
						(s.payout_id ?? "").toLowerCase().includes(searchQuery.toLowerCase())
				)
			: splits
	);

	async function load() {
		loading = true;
		errorMessage = "";
		try {
			const params = new URLSearchParams();
			if (filterValues.source) params.set("source", filterValues.source);
			if (dateFrom) params.set("from", dateFrom);
			if (dateTo) params.set("to", dateTo);
			const result = await universityAdminApi.listRevenueSplits(params.toString() || undefined);
			if (result.ok) {
				const data = result.data as { data: RevenueSplit[] };
				splits = data.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("university.revenue_splits_load_error", "Failed to load revenue splits");
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		load();
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		dateFrom = "";
		dateTo = "";
		load();
	}

	function applyDateFilter() {
		load();
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t("university.revenue_splits_title", "Revenue Splits")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("university.revenue_splits_title", "Revenue Splits")}
	description={t("university.revenue_splits_desc", "Read-only audit viewer for per-event revenue split records")}
/>

{#if errorMessage}
	<div
		class="mb-4 flex items-center justify-between rounded-md border px-4 py-3 text-sm"
		style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
	>
		<span>{errorMessage}</span>
		<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<Button variant="outline" size="sm" onclick={load}>
		<RefreshCw class="h-3.5 w-3.5" />
		{t("common.refresh", "Refresh")}
	</Button>
</div>

<FilterBar
	filters={sourceFilters}
	onsearch={handleSearch}
	onclear={handleClear}
/>

<!-- Date range filters -->
<div class="mb-4 flex flex-wrap items-center gap-3">
	<div class="flex items-center gap-2">
		<label for="date-from" class="text-sm" style="color: var(--color-muted)">
			{t("common.from", "From")}
		</label>
		<input
			id="date-from"
			type="date"
			bind:value={dateFrom}
			onchange={applyDateFilter}
			class="h-10 rounded-lg border px-3 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
		/>
	</div>
	<div class="flex items-center gap-2">
		<label for="date-to" class="text-sm" style="color: var(--color-muted)">
			{t("common.to", "To")}
		</label>
		<input
			id="date-to"
			type="date"
			bind:value={dateTo}
			onchange={applyDateFilter}
			class="h-10 rounded-lg border px-3 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
		/>
	</div>
</div>

{#if loading}
	<TableSkeleton rows={5} />
{:else}
	<DataTable
		{columns}
		data={filtered}
		searchable={true}
		searchPlaceholder={t("university.search_revenue_splits", "Search by instructor, track or payout...")}
		onsearch={(q: string) => {
			searchQuery = q;
		}}
		pageSize={25}
		emptyMessage={t("university.no_revenue_splits", "No revenue split records found")}
	/>
{/if}

{#snippet SourceBadge({ value }: { value: string; row: RevenueSplit })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'pro_stream' ? 'bg-info/15 text-info' : 'bg-primary/15 text-primary'}"
	>
		{value}
	</span>
{/snippet}
