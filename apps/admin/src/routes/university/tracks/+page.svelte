<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import type { ActionItem } from "$lib/components/ui/data-table";
	import { Check, Eye, RefreshCw, X } from "lucide-svelte";
	import { universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { onMount } from "svelte";

	type PendingTrack = {
		id: string;
		title: string;
		slug: string;
		description?: string | null;
		difficulty: string;
		instructor_id?: string | null;
		instructor_name: string;
		price_kobo?: number | null;
		currency: string;
		category?: string | null;
		language: string;
		geo_space_id?: string | null;
		total_enrolled: number;
		is_published: boolean;
		is_published_to_university: boolean;
		created_at: string;
		updated_at: string;
	};

	let tracks = $state<PendingTrack[]>([]);
	let loading = $state(true);
	let errorMessage = $state("");
	let successMessage = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});

	let showApproveModal = $state(false);
	let showRejectModal = $state(false);
	let showViewSlideOver = $state(false);
	let target = $state<PendingTrack | null>(null);
	let actionLoading = $state(false);
	let rejectReason = $state("");
	let showAllTracks = $state(false);

	const canApprove = $derived(hasPermission("university.course.approve"));

	const difficultyFilters = [
		{ key: "difficulty", label: t("university.difficulty", "Difficulty"), options: [
			{ value: "beginner", label: t("university.beginner", "Beginner") },
			{ value: "intermediate", label: t("university.intermediate", "Intermediate") },
			{ value: "advanced", label: t("university.advanced", "Advanced") }
		]}
	];

	const statusFilters = [
		{ key: "status", label: t("university.status", "Status"), options: [
			{ value: "pending", label: t("university.pending", "Pending") },
			{ value: "approved", label: t("university.approved", "Approved") },
			{ value: "rejected", label: t("university.rejected", "Rejected") }
		]}
	];

	const activeFilters = $derived(showAllTracks ? [...difficultyFilters, ...statusFilters] : difficultyFilters);

	function formatPrice(kobo: number | null | undefined, currency: string): string {
		if (kobo == null) return t("university.free", "Free");
		const major = kobo / 100;
		return `${currency} ${major.toLocaleString()}`;
	}

	const columns = [
		{ key: "title", label: t("university.track_title", "Track"), render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: "instructor_name", label: t("university.instructor", "Instructor") },
		{ key: "difficulty", label: t("university.difficulty", "Difficulty"), render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'advanced' ? 'bg-danger/15 text-danger' : v === 'intermediate' ? 'bg-warning/15 text-warning' : 'bg-success/15 text-success'}">${v}</span>` },
		{ key: "price_kobo", label: t("university.price", "Price"), render: (_: any, row: PendingTrack) => `<span class="text-xs">${formatPrice(row.price_kobo, row.currency)}</span>` },
		{ key: "category", label: t("university.category", "Category"), render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v ?? "—"}</span>` },
		{ key: "language", label: t("university.language", "Language"), render: (v: string) => `<span class="text-xs">${v}</span>` },
		{ key: "total_enrolled", label: t("university.enrolled", "Enrolled"), render: (v: number) => `<span class="text-xs">${v}</span>` },
		{ key: "created_at", label: t("common.submitted", "Submitted"), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` }
	];

	const rowActions: ActionItem[] = [
		{ label: t("common.view", "View"), icon: Eye, onclick: (row) => openView(row) },
		{
			label: t("common.approve", "Approve"),
			icon: Check,
			onclick: (row) => openApprove(row),
			show: () => canApprove
		},
		{
			label: t("common.reject", "Reject"),
			icon: X,
			variant: "destructive",
			onclick: (row) => openReject(row),
			show: () => canApprove
		}
	];

	const filtered = $derived(
		tracks.filter((tr) => {
			const matchesSearch = !searchQuery
				|| tr.title.toLowerCase().includes(searchQuery.toLowerCase())
				|| tr.instructor_name.toLowerCase().includes(searchQuery.toLowerCase())
				|| (tr.category ?? "").toLowerCase().includes(searchQuery.toLowerCase());
			const matchesDifficulty = !filterValues.difficulty
				|| tr.difficulty === filterValues.difficulty;
			return matchesSearch && matchesDifficulty;
		})
	);

	async function load() {
		loading = true;
		errorMessage = "";
		try {
			const params = new URLSearchParams();
			if (showAllTracks && filterValues.status) params.set("status", filterValues.status);
			const qs = params.toString() || undefined;
			const result = showAllTracks
				? await universityAdminApi.listAllTracks(qs)
				: await universityAdminApi.listPendingTracks(qs);
			if (result.ok) {
				const data = result.data as { data?: PendingTrack[] } | null | undefined;
				tracks = data?.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("common.error", "Failed to load tracks");
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
		load();
	}

	function openApprove(tr: PendingTrack) { target = tr; showApproveModal = true; }
	function openReject(tr: PendingTrack) { target = tr; rejectReason = ""; showRejectModal = true; }
	function openView(tr: PendingTrack) { target = tr; showViewSlideOver = true; }

	async function doApprove() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const result = await universityAdminApi.approveTrack(target.id);
		if (result.ok) {
			showApproveModal = false;
			successMessage = t("university.track_approved", "Track approved to marketplace");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	async function doReject() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const result = await universityAdminApi.rejectTrack(target.id, { reason: rejectReason.trim() || undefined });
		if (result.ok) {
			showRejectModal = false;
			successMessage = t("university.track_rejected", "Track rejected");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	function toggleAllTracks() {
		showAllTracks = !showAllTracks;
		filterValues = {};
		searchQuery = "";
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t("university.tracks_title", "University Track Approvals")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("university.tracks_title", "University Track Approvals")}
	description={t("university.tracks_desc", "Review and approve instructor-submitted tracks for the Japa University marketplace")}
	actions={[
		{ label: showAllTracks ? t("university.showPending", "Show Pending Only") : t("university.showAll", "Show All Tracks"), variant: "outline" as const, onclick: toggleAllTracks },
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: load }
	]}
/>

{#if errorMessage}
	<div class="mb-4 flex items-center justify-between rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		<span>{errorMessage}</span>
		<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
	</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{successMessage}
	</div>
{/if}

<div class="mt-4">
{#if loading}
	<TableSkeleton rows={5} />
{:else}
	<DataTable
		{columns}
		data={filtered}
		pageSize={25}
		emptyMessage={t("university.no_pending_tracks", "No tracks pending approval")}
		searchable
		onsearch={(q) => { searchQuery = q; load(); }}
		filters={activeFilters}
		bind:filterValue={filterValues}
		onfilterapply={() => { load(); }}
		onfilterclear={() => { load(); }}
		actions={rowActions}
	/>
{/if}
</div>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t("university.track_details", "Track Details")}>
	{#if target}
		<div class="space-y-3">
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.track_title", "Track")}</p><p class="mt-1 text-sm font-medium">{target.title}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.instructor", "Instructor")}</p><p class="mt-1 text-sm">{target.instructor_name}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.slug", "Slug")}</p><p class="mt-1 text-sm font-mono">{target.slug}</p></div>
			{#if target.description}
				<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.description", "Description")}</p><p class="mt-1 text-sm whitespace-pre-wrap">{target.description}</p></div>
			{/if}
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.difficulty", "Difficulty")}</p><p class="mt-1 text-sm">{target.difficulty}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.price", "Price")}</p><p class="mt-1 text-sm">{formatPrice(target.price_kobo, target.currency)}</p></div>
			{#if target.category}
				<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.category", "Category")}</p><p class="mt-1 text-sm">{target.category}</p></div>
			{/if}
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.language", "Language")}</p><p class="mt-1 text-sm">{target.language}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.enrolled", "Enrolled")}</p><p class="mt-1 text-sm">{target.total_enrolled}</p></div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => { showViewSlideOver = false; }}>{t("common.close", "Close")}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Approve Modal -->
<Modal open={showApproveModal} onclose={() => { showApproveModal = false; }} title={t("university.approve_track", "Approve Track")}>
	{#if target}
		<p class="text-sm" style="color: var(--color-muted)">{t("university.approve_track_confirm", "Publish")} <strong>{target.title}</strong> {t("university.to_marketplace", "to the Japa University marketplace?")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showApproveModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={doApprove} disabled={actionLoading}>
			{actionLoading ? t("common.processing", "Processing...") : t("common.approve", "Approve")}
		</Button>
	</div>
</Modal>

<!-- Reject Modal -->
<Modal open={showRejectModal} onclose={() => { showRejectModal = false; }} title={t("university.reject_track", "Reject Track")}>
	{#if target}
		<p class="mb-3 text-sm" style="color: var(--color-muted)">{t("university.reject_track_confirm", "Reject")} <strong>{target.title}</strong>?</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("university.rejectReason", "Reason (optional)")}</label>
			<textarea id="reject-reason" bind:value={rejectReason} rows={3} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t("university.rejectReasonPlaceholder", "Reason for rejection...")}></textarea>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showRejectModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doReject} disabled={actionLoading}>
			{actionLoading ? t("common.processing", "Processing...") : t("common.reject", "Reject")}
		</Button>
	</div>
</Modal>

