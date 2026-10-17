<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { Button } from "$lib/components/ui/button";
	import {
		Users, BookOpen, DollarSign, TrendingUp,
		RefreshCw, ArrowRight
	} from "lucide-svelte";
	import { academyAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	// Ref: japaconnect-engineering/adr/ADR-063-japa-academy-creator-economy.md
	// The /academy hub is the admin landing for the Japa Academy creator
	// economy (ADR-063): instructor review, track approvals, payouts, and the
	// read-only revenue-split audit viewer. "Japa Academy" is the brand name
	// of a single platform-wide creator marketplace — it is NOT the Section 39
	// university space-type directory (that lives at /universities). All counts
	// come from the backend admin API (Rule 16).

	let loading = $state(true);
	let errorMessage = $state("");

	// Pending-work counts (best-effort; degrade to null on error).
	let pendingInstructors = $state<number | null>(null);
	let pendingTracks = $state<number | null>(null);
	let pendingPayouts = $state<number | null>(null);
	let revenueSplitCount = $state<number | null>(null);

	const canApproveInstructors = $derived(hasPermission("academy.instructor.approve"));
	const canApproveTracks = $derived(hasPermission("academy.course.approve"));
	const canProcessPayouts = $derived(hasPermission("academy.payout.process"));
	const canViewRevenue = $derived(hasPermission("academy.revenue.list"));

	type HubCard = {
		label: string;
		description: string;
		href: string;
		icon: any;
		permission: string;
		count: number | null;
		countLabel: string;
	};

	const cards = $derived<HubCard[]>([
		{
			label: t("academy.instructors_title", "Instructors"),
			description: t("academy.instructors_desc", "Review and manage instructor applications for the Japa Academy creator economy"),
			href: "/academy/instructors",
			icon: Users,
			permission: "academy.instructor.approve",
			count: pendingInstructors,
			countLabel: t("academy.pending", "pending")
		},
		{
			label: t("academy.tracks_title", "Track Approvals"),
			description: t("academy.tracks_desc", "Review and approve instructor-submitted tracks for the Japa Academy marketplace"),
			href: "/academy/tracks",
			icon: BookOpen,
			permission: "academy.course.approve",
			count: pendingTracks,
			countLabel: t("academy.pending", "pending")
		},
		{
			label: t("academy.payouts_title", "Payouts"),
			description: t("academy.payouts_desc", "Process instructor payout periods and mark failed transfers"),
			href: "/academy/payouts",
			icon: DollarSign,
			permission: "academy.payout.process",
			count: pendingPayouts,
			countLabel: t("academy.pending", "pending")
		},
		{
			label: t("academy.revenue_splits_title", "Revenue Splits"),
			description: t("academy.revenue_splits_desc", "Read-only audit viewer over the immutable revenue-split ledger"),
			href: "/academy/revenue-splits",
			icon: TrendingUp,
			permission: "academy.revenue.list",
			count: revenueSplitCount,
			countLabel: t("academy.records", "records")
		}
	]);

	function canSee(permission?: string): boolean {
		if (!permission) return true;
		return hasPermission(permission);
	}

	async function loadCounts() {
		errorMessage = "";
		const tasks: Promise<void>[] = [];
		if (canApproveInstructors) {
			tasks.push(
				(async () => {
					try {
						const r = await academyAdminApi.listInstructors("status=pending&limit=1");
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
						const r = await academyAdminApi.listPendingTracks("limit=1");
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
						const r = await academyAdminApi.listPayouts("status=pending&limit=1");
						if (r.ok) {
							const d = r.data as { meta?: { total?: number }; data?: unknown[] };
							pendingPayouts = d.meta?.total ?? d.data?.length ?? 0;
						}
					} catch { pendingPayouts = null; }
				})()
			);
		}
		if (canViewRevenue) {
			tasks.push(
				(async () => {
					try {
						const r = await academyAdminApi.listRevenueSplits("limit=1");
						if (r.ok) {
							const d = r.data as { meta?: { total?: number }; data?: unknown[] };
							revenueSplitCount = d.meta?.total ?? d.data?.length ?? 0;
						}
					} catch { revenueSplitCount = null; }
				})()
			);
		}
		await Promise.all(tasks);
	}

	async function load() {
		loading = true;
		await loadCounts();
		loading = false;
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t("academy.hub_title", "Japa Academy")} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t("academy.hub_title", "Japa Academy")}
	description={t("academy.hub_desc", "Creator economy — manage instructors, track approvals, payouts, and the revenue-split audit ledger")}
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

<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
	{#each cards as card (card)}
		{#if canSee(card.permission)}
			<a
				href={card.href}
				class="group flex flex-col rounded-lg border p-5 transition-colors hover:border-primary/40"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
			>
				<div class="flex items-start justify-between">
					<div class="flex h-11 w-11 items-center justify-center rounded-lg" style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)">
						<card.icon class="h-5 w-5" style="color: var(--color-primary)" />
					</div>
					{#if loading}
						<span class="h-5 w-12 animate-pulse rounded-full" style="background: var(--color-muted)"></span>
					{:else if card.count != null}
						<span class="rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: color-mix(in srgb, var(--color-warning) 15%, transparent); color: var(--color-warning)">
							{card.count} {card.countLabel}
						</span>
					{/if}
				</div>
				<p class="mt-4 text-sm font-semibold" style="color: var(--color-foreground)">{card.label}</p>
				<p class="mt-1 flex-1 text-xs leading-relaxed" style="color: var(--color-muted)">{card.description}</p>
				<div class="mt-4 flex items-center gap-1 text-xs font-medium" style="color: var(--color-primary)">
					{t("common.open", "Open")}
					<ArrowRight class="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
				</div>
			</a>
		{/if}
	{/each}
</div>

{#if !canApproveInstructors && !canApproveTracks && !canProcessPayouts && !canViewRevenue}
	<Card>
		<CardContent>
			<div class="flex flex-col items-center justify-center gap-2 py-12 text-center">
				<Users class="h-8 w-8" style="color: var(--color-muted)" />
				<p class="text-sm font-medium" style="color: var(--color-foreground)">{t("academy.no_access", "No Japa Academy permissions")}</p>
				<p class="text-xs" style="color: var(--color-muted)">{t("academy.no_access_desc", "You need at least one academy.instructor.approve, academy.course.approve, academy.payout.process, or academy.revenue.list permission to manage the creator economy.")}</p>
			</div>
		</CardContent>
	</Card>
{/if}
