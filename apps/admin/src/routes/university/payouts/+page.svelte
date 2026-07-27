<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { DollarSign, RefreshCw } from "lucide-svelte";
	import { universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type Payout = {
		id: string;
		instructor_id: string;
		period_start: string;
		period_end: string;
		gross_kobo: number;
		platform_commission_kobo: number;
		net_kobo: number;
		currency: string;
		status: string;
		payout_reference?: string | null;
		failure_reason?: string | null;
		processed_at?: string | null;
		created_at: string;
	};

	let payouts = $state<Payout[]>([]);
	let loading = $state(true);
	let errorMessage = $state("");
	let successMessage = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});

	let showProcessModal = $state(false);
	let target = $state<Payout | null>(null);
	let actionLoading = $state(false);

	const canProcess = $derived(hasPermission("university.payout.process"));

	const statusFilters = [
		{ key: "status", label: t("university.status", "Status"), options: [
			{ value: "pending", label: t("university.pending", "Pending") },
			{ value: "processing", label: t("university.processing", "Processing") },
			{ value: "paid", label: t("university.paid", "Paid") },
			{ value: "failed", label: t("university.failed", "Failed") }
		]}
	];

	function formatKobo(kobo: number, currency: string): string {
		const major = kobo / 100;
		return `${currency} ${major.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
	}

	const columns = [
		{ key: "instructor_id", label: t("university.instructor_id", "Instructor ID"), render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v.slice(0, 8)}...</span>` },
		{ key: "period_start", label: t("university.period", "Period"), sortable: true, render: (_: any, row: Payout) => `<span class="text-xs" style="color: var(--color-muted)">${row.period_start} → ${row.period_end}</span>` },
		{ key: "gross_kobo", label: t("university.gross", "Gross"), sortable: true, render: (v: number, row: Payout) => `<span class="text-sm font-medium">${formatKobo(v, row.currency)}</span>` },
		{ key: "platform_commission_kobo", label: t("university.commission", "Commission"), render: (v: number, row: Payout) => `<span class="text-xs" style="color: var(--color-muted)">${formatKobo(v, row.currency)}</span>` },
		{ key: "net_kobo", label: t("university.net", "Net"), sortable: true, render: (v: number, row: Payout) => `<span class="text-sm font-semibold" style="color: var(--color-success)">${formatKobo(v, row.currency)}</span>` },
		{ key: "status", label: t("university.status", "Status"), sortable: true, componentSnippet: PayoutStatusBadge },
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32", componentSnippet: PayoutActionsCell }
	];

	const filtered = $derived(
		searchQuery
			? payouts.filter((p) => p.instructor_id.toLowerCase().includes(searchQuery.toLowerCase()) || (p.payout_reference ?? "").toLowerCase().includes(searchQuery.toLowerCase()))
			: payouts
	);

	async function load() {
		loading = true;
		errorMessage = "";
		try {
			const params = new URLSearchParams();
			if (filterValues.status) params.set("status", filterValues.status);
			const result = await universityAdminApi.listPayouts(params.toString() || undefined);
			if (result.ok) {
				const data = result.data as { data: Payout[] };
				payouts = data.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("common.error", "Failed to load payouts");
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
	}

	function openProcess(p: Payout) { target = p; showProcessModal = true; }

	async function doProcess() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const result = await universityAdminApi.processPayout(target.id);
		if (result.ok) {
			showProcessModal = false;
			successMessage = t("university.payout_processing", "Payout processing initiated");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t("university.payouts_title", "University Payouts")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("university.payouts_title", "University Payouts")}
	description={t("university.payouts_desc", "Process instructor payout transfers for the Japa University creator economy")}
/>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{successMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<Button variant="outline" size="sm" onclick={load}>
		<RefreshCw class="h-3.5 w-3.5" />
		{t("common.refresh", "Refresh")}
	</Button>
</div>

<FilterBar filters={statusFilters} onsearch={handleSearch} onclear={() => { searchQuery = ""; filterValues = {}; }} />

{#if loading}
	<TableSkeleton rows={5} />
{:else}
	<DataTable
		{columns}
		data={filtered}
		searchable={true}
		searchPlaceholder={t("university.search_payouts", "Search by instructor or reference...")}
		onsearch={(q: string) => { searchQuery = q; }}
		pageSize={25}
		emptyMessage={t("university.no_payouts", "No payouts found")}
	/>
{/if}

<!-- Process Modal -->
<Modal open={showProcessModal} onclose={() => { showProcessModal = false; }} title={t("university.process_payout", "Process Payout")}>
	{#if target}
		<div class="space-y-3">
			<p class="text-sm" style="color: var(--color-muted)">{t("university.process_payout_confirm", "Process this payout transfer?")}</p>
			<div class="rounded-lg border p-4 space-y-2" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<div class="flex justify-between text-sm"><span style="color: var(--color-muted)">{t("university.period", "Period")}</span><span>{target.period_start} → {target.period_end}</span></div>
				<div class="flex justify-between text-sm"><span style="color: var(--color-muted)">{t("university.gross", "Gross")}</span><span class="font-medium">{formatKobo(target.gross_kobo, target.currency)}</span></div>
				<div class="flex justify-between text-sm"><span style="color: var(--color-muted)">{t("university.commission", "Platform Commission")}</span><span>{formatKobo(target.platform_commission_kobo, target.currency)}</span></div>
				<div class="flex justify-between text-sm border-t pt-2" style="border-color: var(--color-border)"><span style="color: var(--color-muted)">{t("university.net", "Net to Instructor")}</span><span class="font-semibold" style="color: var(--color-success)">{formatKobo(target.net_kobo, target.currency)}</span></div>
			</div>
			{#if target.failure_reason}
				<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">
					{t("university.previous_failure", "Previous failure")}: {target.failure_reason}
				</div>
			{/if}
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showProcessModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={doProcess} disabled={actionLoading}>
			<DollarSign class="h-3.5 w-3.5" />
			{actionLoading ? t("common.processing", "Processing...") : t("university.process_payout", "Process Payout")}
		</Button>
	</div>
</Modal>

{#snippet PayoutStatusBadge({ value }: { value: string; row: Payout })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'paid' ? 'bg-success/15 text-success' :
		 value === 'processing' ? 'bg-info/15 text-info' :
		 value === 'failed' ? 'bg-danger/15 text-danger' :
		 'bg-warning/15 text-warning'}">
		{value}
	</span>
{/snippet}

{#snippet PayoutActionsCell({ row }: { value: any; row: Payout })}
	{#if canProcess && (row.status === 'pending' || row.status === 'failed')}
		<Button variant="ghost" size="sm" onclick={() => openProcess(row)} style="color: var(--color-primary)">
			<DollarSign class="h-3.5 w-3.5" />
			{t("university.process", "Process")}
		</Button>
	{/if}
{/snippet}
