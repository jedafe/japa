<script lang="ts">
	import { goto } from "$app/navigation";
	import { page } from "$app/state";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Button } from "$lib/components/ui/button";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import {
		GraduationCap, Users, BookOpen, DollarSign, TrendingUp,
		Eye, RefreshCw, ArrowRight, BadgeCheck, XCircle
	} from "lucide-svelte";
	import { universitiesAdminApi, universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	// Ref: japaconnect-engineering/sections/39-university-creator-economy.md (ADR-063)
	// The /university hub is the admin landing for the Japa University creator
	// economy: catalog CRUD (universities), instructor review, track approvals,
	// payouts, and revenue splits. All data comes from the backend API (Rule 16).

	type University = {
		id: string;
		name: string;
		slug: string;
		member_count: number;
		status: string;
		verified: boolean;
		created_at: string;
	};

	type UniversityRaw = Omit<University, "verified">;

	function deriveVerified(u: UniversityRaw): University {
		return { ...u, verified: u.status === "active" };
	}

	let universities = $state<University[]>([]);
	let allUniversities = $state<University[]>([]);
	let loading = $state(true);
	let errorMessage = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let currentPage = $state(0);
	let pageSize = $state(10);
	let totalItems = $state(0);
	let totalPages = $state(1);

	// Creator-economy summary counts (best-effort; degrade to — on error).
	let pendingInstructors = $state<number | null>(null);
	let pendingTracks = $state<number | null>(null);
	let pendingPayouts = $state<number | null>(null);
	let summaryError = $state("");

	const canManageUniversities = $derived(hasPermission("admin.settings"));
	const canApproveInstructors = $derived(hasPermission("university.instructor.approve"));
	const canApproveTracks = $derived(hasPermission("university.course.approve"));
	const canProcessPayouts = $derived(hasPermission("university.payout.process"));
	const canViewRevenue = $derived(hasPermission("university.revenue.list"));

	const filterDefs = [
		{
			key: "verified",
			label: t("universities.verified", "Verified"),
			options: [
				{ value: "true", label: t("common.yes", "Yes") },
				{ value: "false", label: t("common.no", "No") }
			]
		}
	];

	const columns = [
		{ key: "name", label: t("universities.name", "Name"), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: "slug", label: t("universities.slug", "Slug"), sortable: true, render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{
			key: "status",
			label: t("universities.status", "Status"),
			sortable: true,
			render: (v: string) => {
				const cls = v === "active" ? "bg-success/15 text-success" : "bg-warning/15 text-warning";
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
			}
		},
		{ key: "member_count", label: t("universities.students", "Members"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
		{ key: "created_at", label: t("universities.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32", componentSnippet: ActionsCell }
	];

	const hubCards = $derived([
		{
			label: t("university.instructors_title", "Instructors"),
			description: t("university.instructors_desc", "Review instructor applications"),
			href: "/university/instructors",
			icon: Users,
			permission: "university.instructor.approve",
			count: pendingInstructors,
			countLabel: t("university.pending", "pending")
		},
		{
			label: t("university.tracks_title", "Track Approvals"),
			description: t("university.tracks_desc", "Approve submitted tracks"),
			href: "/university/tracks",
			icon: BookOpen,
			permission: "university.course.approve",
			count: pendingTracks,
			countLabel: t("university.pending", "pending")
		},
		{
			label: t("university.payouts_title", "Payouts"),
			description: t("university.payouts_desc", "Process instructor payouts"),
			href: "/university/payouts",
			icon: DollarSign,
			permission: "university.payout.process",
			count: pendingPayouts,
			countLabel: t("university.pending", "pending")
		},
		{
			label: t("university.revenue_splits_title", "Revenue Splits"),
			description: t("university.revenue_splits_desc", "Review revenue share"),
			href: "/university/revenue-splits",
			icon: TrendingUp,
			permission: "university.revenue.list",
			count: null,
			countLabel: ""
		}
	]);

	function canSee(permission?: string): boolean {
		if (!permission) return true;
		return hasPermission(permission);
	}

	async function loadSummary() {
		summaryError = "";
		const tasks: Promise<void>[] = [];
		if (canApproveInstructors) {
			tasks.push(
				(async () => {
					try {
						const r = await universityAdminApi.listInstructors("status=pending&limit=1");
						if (r.ok) {
							const d = r.data as { meta?: { total?: number }; data?: unknown[] };
							pendingInstructors = d.meta?.total ?? d.data?.length ?? 0;
						}
					} catch { pendingInstructors = null; }
				})()
			);
		}
		if (canApproveTracks) {
			tasks.push(
				(async () => {
					try {
						const r = await universityAdminApi.listPendingTracks("limit=1");
						if (r.ok) {
							const d = r.data as { meta?: { total?: number }; data?: unknown[] };
							pendingTracks = d.meta?.total ?? d.data?.length ?? 0;
						}
					} catch { pendingTracks = null; }
				})()
			);
		}
		if (canProcessPayouts) {
			tasks.push(
				(async () => {
					try {
						const r = await universityAdminApi.listPayouts("status=pending&limit=1");
						if (r.ok) {
							const d = r.data as { meta?: { total?: number }; data?: unknown[] };
							pendingPayouts = d.meta?.total ?? d.data?.length ?? 0;
						}
					} catch { pendingPayouts = null; }
				})()
			);
		}
		await Promise.all(tasks);
	}

	async function fetchAll() {
		errorMessage = "";
		try {
			const result = await universitiesAdminApi.list();
			if (result.ok) {
				const d = result.data as UniversityRaw[] | { items: UniversityRaw[] };
				const raw = Array.isArray(d) ? d : (d.items ?? []);
				allUniversities = raw.map(deriveVerified);
			} else {
				errorMessage = result.error ?? t("common.error", "Failed to load");
			}
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : t("common.error", "Failed to load");
		}
	}

	function applyFilters() {
		let filtered = allUniversities;
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			filtered = filtered.filter((u) => u.name.toLowerCase().includes(q) || u.slug.toLowerCase().includes(q));
		}
		if (filterValues.verified === "true") {
			filtered = filtered.filter((u) => u.status === "active");
		} else if (filterValues.verified === "false") {
			filtered = filtered.filter((u) => u.status !== "active");
		}
		totalItems = filtered.length;
		totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
		if (currentPage >= totalPages) currentPage = totalPages - 1;
		const start = currentPage * pageSize;
		universities = filtered.slice(start, start + pageSize);
	}

	async function load() {
		loading = true;
		await Promise.all([fetchAll(), loadSummary()]);
		applyFilters();
		loading = false;
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		currentPage = 0;
		applyFilters();
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		currentPage = 0;
		applyFilters();
	}

	function openUniversity(slug: string) {
		goto(`/university/${slug}`);
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		if (allUniversities.length > 0) {
			applyFilters();
		} else {
			load();
		}
	});
</script>

<svelte:head>
	<title>{t("university.hub_title", "Japa University")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("university.hub_title", "Japa University")}
	description={t("university.hub_desc", "Creator economy hub — manage universities, instructors, tracks, payouts, and revenue splits")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: load }
	]}
/>

{#if errorMessage}
	<div class="mb-4 flex items-center justify-between rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		<span>{errorMessage}</span>
		<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
	</div>
{/if}

<!-- Creator-economy hub cards -->
<div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
	{#each hubCards as card}
		{#if canSee(card.permission)}
			<a
				href={card.href}
				class="group rounded-lg border p-4 transition-colors hover:border-primary/40"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
			>
				<div class="flex items-start justify-between">
					<div class="flex h-10 w-10 items-center justify-center rounded-lg" style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)">
						<card.icon class="h-5 w-5" style="color: var(--color-primary)" />
					</div>
					{#if card.count != null}
						<span class="rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: color-mix(in srgb, var(--color-warning) 15%, transparent); color: var(--color-warning)">
							{card.count} {card.countLabel}
						</span>
					{/if}
				</div>
				<p class="mt-3 text-sm font-medium" style="color: var(--color-foreground)">{card.label}</p>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{card.description}</p>
				<div class="mt-3 flex items-center gap-1 text-xs font-medium" style="color: var(--color-primary)">
					{t("common.open", "Open")}
					<ArrowRight class="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
				</div>
			</a>
		{/if}
	{/each}
</div>

<!-- University catalog -->
<Card>
	<CardHeader>
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				<GraduationCap class="h-4 w-4" style="color: var(--color-muted)" />
				<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">{t("university.catalog", "University Catalog")}</h2>
			</div>
			{#if canManageUniversities}
				<Button variant="outline" size="sm" href="/universities">
					{t("universities.manage", "Manage")}
					<ArrowRight class="ml-1 h-3 w-3" />
				</Button>
			{/if}
		</div>
	</CardHeader>
	<CardContent>
		<div class="mb-4">
			<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />
		</div>
		{#if loading}
			<TableSkeleton rows={5} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable {columns} data={universities} searchable={false} pageSize={pageSize} emptyMessage={t("university.no_universities", "No universities found")} />
			</div>
			<Pagination bind:page={currentPage} {totalPages} {totalItems} bind:pageSize />
		{/if}
	</CardContent>
</Card>

{#snippet ActionsCell({ row }: { value: any; row: University })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openUniversity(row.slug)} title={t("common.view", "View")} aria-label={t("common.view", "View")}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageUniversities}
			<Button
				variant="ghost"
				size="sm"
				onclick={() => openUniversity(row.slug)}
				title={row.verified ? t("universities.unverify", "Unverify") : t("universities.verify", "Verify")}
				aria-label={row.verified ? t("universities.unverify", "Unverify") : t("universities.verify", "Verify")}
			>
				{#if row.verified}
					<XCircle class="h-3.5 w-3.5" style="color: var(--color-danger)" />
				{:else}
					<BadgeCheck class="h-3.5 w-3.5" style="color: var(--color-success)" />
				{/if}
			</Button>
		{/if}
	</div>
{/snippet}
