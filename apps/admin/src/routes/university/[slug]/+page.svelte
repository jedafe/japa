<script lang="ts">
	import { page } from "$app/state";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Button } from "$lib/components/ui/button";
	import { DataTable } from "$lib/components/ui/data-table";
	import {
		ArrowLeft, RefreshCw, GraduationCap, Users, BookOpen,
		DollarSign, TrendingUp, ExternalLink
	} from "lucide-svelte";
	import { universitiesAdminApi, universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	// Ref: japaconnect-engineering/adr/ADR-063-japa-university-creator-economy.md
	// Per-university creator-economy view. The [slug] is a University space slug
	// (Section 39 space type). We resolve it to a space via the public
	// GET /universities/{slug} endpoint, then scope the ADR-063 admin creator-
	// economy lists to that space via ?space_id=<id> (tracks are filtered
	// directly on learning_tracks.geo_space_id; instructors/payouts/revenue
	// splits are filtered to entities linked to a track in that space). All
	// data comes from the backend API (Rule 16).

	type UniversitySpace = {
		id: string;
		name: string;
		slug: string;
		description?: string | null;
		member_count: number;
		post_count: number;
		follower_count: number;
		status: string;
		geo_code?: string | null;
		created_at: string;
	};

	type Instructor = {
		id: string;
		display_name: string;
		headline?: string | null;
		tier: string;
		status: string;
		kyc_status: string;
		created_at: string;
	};

	type Track = {
		id: string;
		title: string;
		slug: string;
		difficulty: string;
		instructor_name: string;
		price_kobo?: number | null;
		currency: string;
		category?: string | null;
		language: string;
		total_enrolled: number;
		is_published_to_university: boolean;
		university_submission_status?: string | null;
		created_at: string;
	};

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
		created_at: string;
	};

	type RevenueSplit = {
		id: string;
		instructor_id: string;
		track_id: string;
		source: string;
		gross_kobo: number;
		platform_kobo: number;
		creator_kobo: number;
		currency: string;
		created_at: string;
	};

	let space = $state<UniversitySpace | null>(null);
	let spaceId = $state("");
	let loading = $state(true);
	let errorMessage = $state("");

	let instructors = $state<Instructor[]>([]);
	let tracks = $state<Track[]>([]);
	let payouts = $state<Payout[]>([]);
	let splits = $state<RevenueSplit[]>([]);
	let instructorsTotal = $state(0);
	let tracksTotal = $state(0);
	let payoutsTotal = $state(0);
	let splitsTotal = $state(0);

	const slug = $derived(page.params.slug ?? "");
	const canApproveInstructors = $derived(hasPermission("university.instructor.approve"));
	const canApproveTracks = $derived(hasPermission("university.course.approve"));
	const canProcessPayouts = $derived(hasPermission("university.payout.process"));
	const canViewRevenue = $derived(hasPermission("university.revenue.list"));

	function fmtDate(v?: string | null): string {
		if (!v) return "—";
		try { return new Date(v).toLocaleDateString(); } catch { return v; }
	}

	function fmtKobo(kobo: number, currency: string): string {
		if (!kobo) return `${currency} 0`;
		return `${currency} ${(kobo / 100).toLocaleString()}`;
	}

	const instructorColumns = [
		{ key: "display_name", label: t("university.instructor", "Instructor"), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: "tier", label: t("university.tier", "Tier"), sortable: true, render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-primary/15 text-primary">${v}</span>` },
		{ key: "status", label: t("university.status", "Status"), sortable: true, render: (v: string) => `<span class="text-xs ${v === 'approved' ? 'text-success' : v === 'pending' ? 'text-warning' : 'text-danger'}">${v}</span>` },
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${fmtDate(v)}</span>` }
	];

	const trackColumns = [
		{ key: "title", label: t("university.track_title", "Track"), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: "instructor_name", label: t("university.instructor", "Instructor"), sortable: true },
		{ key: "difficulty", label: t("university.difficulty", "Difficulty"), sortable: true, render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'advanced' ? 'bg-danger/15 text-danger' : v === 'intermediate' ? 'bg-warning/15 text-warning' : 'bg-success/15 text-success'}">${v}</span>` },
		{ key: "total_enrolled", label: t("university.enrolled", "Enrolled"), sortable: true, render: (v: number) => `<span class="text-xs">${v}</span>` },
		{ key: "university_submission_status", label: t("university.submission_status", "Submission"), render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v ?? "—"}</span>` }
	];

	const payoutColumns = [
		{ key: "period_start", label: t("university.period", "Period"), sortable: true, render: (_: any, row: Payout) => `<span class="text-xs">${fmtDate(row.period_start)} → ${fmtDate(row.period_end)}</span>` },
		{ key: "status", label: t("university.status", "Status"), sortable: true, render: (v: string) => `<span class="text-xs ${v === 'paid' ? 'text-success' : v === 'failed' ? 'text-danger' : 'text-warning'}">${v}</span>` },
		{ key: "net_kobo", label: t("university.net", "Net"), sortable: true, render: (v: number, row: Payout) => `<span class="text-xs">${fmtKobo(v, row.currency)}</span>` },
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${fmtDate(v)}</span>` }
	];

	const splitColumns = [
		{ key: "source", label: t("university.source", "Source"), sortable: true, render: (v: string) => `<span class="text-xs">${v}</span>` },
		{ key: "creator_kobo", label: t("university.creator_share", "Creator"), sortable: true, render: (v: number, row: RevenueSplit) => `<span class="text-xs">${fmtKobo(v, row.currency)}</span>` },
		{ key: "platform_kobo", label: t("university.platform_share", "Platform"), sortable: true, render: (v: number, row: RevenueSplit) => `<span class="text-xs">${fmtKobo(v, row.currency)}</span>` },
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${fmtDate(v)}</span>` }
	];

	async function loadSpace() {
		const result = await universitiesAdminApi.getBySlug(slug);
		if (result.ok) {
			space = result.data as UniversitySpace;
			spaceId = space.id;
		} else {
			throw new Error(result.error ?? t("universities.notFound", "University not found"));
		}
	}

	async function loadScoped() {
		errorMessage = "";
		const sid = spaceId;
		const tasks: Promise<void>[] = [];
		if (canApproveInstructors) {
			tasks.push((async () => {
				try {
					const r = await universityAdminApi.listInstructors(`space_id=${sid}&limit=10`);
					if (r.ok) {
						const d = r.data as { data?: Instructor[]; meta?: { total?: number } };
						instructors = d.data ?? [];
						instructorsTotal = d.meta?.total ?? instructors.length;
					}
				} catch { instructors = []; }
			})());
		}
		if (canApproveTracks) {
			tasks.push((async () => {
				try {
					const r = await universityAdminApi.listAllTracks(`space_id=${sid}&limit=10`);
					if (r.ok) {
						const d = r.data as { data?: Track[]; meta?: { total?: number } };
						tracks = d.data ?? [];
						tracksTotal = d.meta?.total ?? tracks.length;
					}
				} catch { tracks = []; }
			})());
		}
		if (canProcessPayouts) {
			tasks.push((async () => {
				try {
					const r = await universityAdminApi.listPayouts(`space_id=${sid}&limit=10`);
					if (r.ok) {
						const d = r.data as { data?: Payout[]; meta?: { total?: number } };
						payouts = d.data ?? [];
						payoutsTotal = d.meta?.total ?? payouts.length;
					}
				} catch { payouts = []; }
			})());
		}
		if (canViewRevenue) {
			tasks.push((async () => {
				try {
					const r = await universityAdminApi.listRevenueSplits(`space_id=${sid}&limit=10`);
					if (r.ok) {
						const d = r.data as { data?: RevenueSplit[]; meta?: { total?: number } };
						splits = d.data ?? [];
						splitsTotal = d.meta?.total ?? splits.length;
					}
				} catch { splits = []; }
			})());
		}
		await Promise.all(tasks);
	}

	async function load() {
		loading = true;
		errorMessage = "";
		try {
			await loadSpace();
			await loadScoped();
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : t("common.error", "Failed to load");
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (slug) load();
	});
</script>

<svelte:head>
	<title>{space?.name ?? t("university.detail", "University")} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<Button variant="ghost" size="sm" href="/university">
		<ArrowLeft class="mr-1 h-3.5 w-3.5" />
		{t("university.back_to_hub", "Back to University Hub")}
	</Button>
</div>

{#if errorMessage && !space}
	<div class="flex flex-col items-center justify-center gap-3 py-16">
		<GraduationCap class="h-10 w-10" style="color: var(--color-muted)" />
		<p class="text-sm" style="color: var(--color-danger)">{errorMessage}</p>
		<div class="flex gap-2">
			<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
			<Button variant="ghost" size="sm" href="/university">{t("university.back_to_hub", "Back to University Hub")}</Button>
		</div>
	</div>
{:else if loading}
	<div class="space-y-4">
		<div class="h-8 w-64 animate-pulse rounded" style="background: var(--color-muted)"></div>
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			{#each Array(3) as _, i (i)}
				<div class="h-24 animate-pulse rounded-lg border" style="border-color: var(--color-border); background: var(--color-surface)"></div>
			{/each}
		</div>
	</div>
{:else if space}
	<PageHeader
		title={space.name}
		description={space.description ?? t("university.scoped_desc", "Creator economy activity for tracks targeted at this university space")}
		actions={[
			{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: load },
			{ label: t("universities.manage", "Manage University"), icon: ExternalLink, variant: "outline" as const, href: "/universities" }
		]}
	/>

	{#if errorMessage}
		<div class="mb-4 flex items-center justify-between rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			<span>{errorMessage}</span>
			<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
		</div>
	{/if}

	<!-- Space identity + stats -->
	<div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-3">
		<Card>
			<CardContent>
				<div class="flex items-center gap-3">
					<div class="flex h-12 w-12 items-center justify-center rounded-lg" style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)">
						<GraduationCap class="h-6 w-6" style="color: var(--color-primary)" />
					</div>
					<div class="min-w-0">
						<p class="truncate text-sm font-semibold" style="color: var(--color-foreground)">{space.name}</p>
						<p class="truncate font-mono text-xs" style="color: var(--color-muted)">{space.slug}</p>
						<span class="mt-1 inline-block rounded-full px-2 py-0.5 text-xs font-medium {space.status === 'active' ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning'}">{space.status}</span>
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.students", "Members")}</p>
				<p class="mt-1 text-2xl font-semibold" style="color: var(--color-foreground)">{(space.member_count ?? 0).toLocaleString()}</p>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{(space.follower_count ?? 0).toLocaleString()} {t("university.followers", "followers")} · {(space.post_count ?? 0).toLocaleString()} {t("university.posts", "posts")}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.creator_economy", "Creator Economy")}</p>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t("university.scoped_note", "Tracks targeted at this university space and their associated instructors, payouts, and revenue splits.")}</p>
			</CardContent>
		</Card>
	</div>

	<!-- Instructors scoped to this space -->
	{#if canApproveInstructors}
		<Card class="mb-6">
			<CardHeader>
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-2">
						<Users class="h-4 w-4" style="color: var(--color-muted)" />
						<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">{t("university.instructors_title", "Instructors")}</h2>
						<span class="text-xs" style="color: var(--color-muted)">({instructorsTotal})</span>
					</div>
					<Button variant="ghost" size="sm" href="/university/instructors">{t("common.view_all", "View all")} →</Button>
				</div>
			</CardHeader>
			<CardContent>
				{#if instructors.length === 0}
					<p class="py-6 text-center text-xs" style="color: var(--color-muted)">{t("university.no_instructors_scoped", "No instructors have tracks targeted at this university space.")}</p>
				{:else}
					<DataTable columns={instructorColumns} data={instructors} pageSize={10} emptyMessage="" />
				{/if}
			</CardContent>
		</Card>
	{/if}

	<!-- Tracks scoped to this space -->
	{#if canApproveTracks}
		<Card class="mb-6">
			<CardHeader>
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-2">
						<BookOpen class="h-4 w-4" style="color: var(--color-muted)" />
						<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">{t("university.tracks_title", "Tracks")}</h2>
						<span class="text-xs" style="color: var(--color-muted)">({tracksTotal})</span>
					</div>
					<Button variant="ghost" size="sm" href="/university/tracks">{t("common.view_all", "View all")} →</Button>
				</div>
			</CardHeader>
			<CardContent>
				{#if tracks.length === 0}
					<p class="py-6 text-center text-xs" style="color: var(--color-muted)">{t("university.no_tracks_scoped", "No tracks targeted at this university space.")}</p>
				{:else}
					<DataTable columns={trackColumns} data={tracks} pageSize={10} emptyMessage="" />
				{/if}
			</CardContent>
		</Card>
	{/if}

	<!-- Payouts scoped to this space -->
	{#if canProcessPayouts}
		<Card class="mb-6">
			<CardHeader>
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-2">
						<DollarSign class="h-4 w-4" style="color: var(--color-muted)" />
						<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">{t("university.payouts_title", "Payouts")}</h2>
						<span class="text-xs" style="color: var(--color-muted)">({payoutsTotal})</span>
					</div>
					<Button variant="ghost" size="sm" href="/university/payouts">{t("common.view_all", "View all")} →</Button>
				</div>
			</CardHeader>
			<CardContent>
				{#if payouts.length === 0}
					<p class="py-6 text-center text-xs" style="color: var(--color-muted)">{t("university.no_payouts_scoped", "No payouts for instructors with tracks in this university space.")}</p>
				{:else}
					<DataTable columns={payoutColumns} data={payouts} pageSize={10} emptyMessage="" />
				{/if}
			</CardContent>
		</Card>
	{/if}

	<!-- Revenue splits scoped to this space -->
	{#if canViewRevenue}
		<Card class="mb-6">
			<CardHeader>
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-2">
						<TrendingUp class="h-4 w-4" style="color: var(--color-muted)" />
						<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">{t("university.revenue_splits_title", "Revenue Splits")}</h2>
						<span class="text-xs" style="color: var(--color-muted)">({splitsTotal})</span>
					</div>
					<Button variant="ghost" size="sm" href="/university/revenue-splits">{t("common.view_all", "View all")} →</Button>
				</div>
			</CardHeader>
			<CardContent>
				{#if splits.length === 0}
					<p class="py-6 text-center text-xs" style="color: var(--color-muted)">{t("university.no_splits_scoped", "No revenue splits for tracks in this university space.")}</p>
				{:else}
					<DataTable columns={splitColumns} data={splits} pageSize={10} emptyMessage="" />
				{/if}
			</CardContent>
		</Card>
	{/if}
{/if}
