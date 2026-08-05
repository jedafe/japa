<script lang="ts">
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import {
		GraduationCap, ArrowLeft, RefreshCw, BadgeCheck, XCircle,
		Pencil, Trash2, FileEdit, Users, BookOpen, DollarSign, TrendingUp,
		ArrowRight, ExternalLink
	} from "lucide-svelte";
	import { universitiesAdminApi, universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	// Ref: japaconnect-engineering/sections/39-university-creator-economy.md (ADR-063)
	// University detail: shows the university space profile (from the public
	// GET /universities/{slug} endpoint, which returns the space + custom
	// field values + logo_url) and links to the creator-economy sub-pages
	// scoped to this university. All data comes from the backend API (Rule 16).

	type CustomFieldValue = {
		id: string;
		field_id: string;
		entity_type: string;
		entity_id: string;
		value: string | null;
		created_at?: string;
		updated_at?: string;
	};

	type UniversityDetail = {
		id: string;
		name: string;
		slug: string;
		description?: string | null;
		geo_code?: string | null;
		level?: string;
		member_count: number;
		post_count: number;
		follower_count: number;
		visibility?: string;
		status: string;
		currency_code?: string | null;
		created_at: string;
		updated_at: string;
		fields: CustomFieldValue[];
		logo_url?: string | null;
	};

	let university = $state<UniversityDetail | null>(null);
	let loading = $state(true);
	let errorMessage = $state("");
	let successMessage = $state("");
	let acting = $state(false);

	let showEditModal = $state(false);
	let showDeleteModal = $state(false);
	let showStatusModal = $state(false);
	let formName = $state("");
	let formSlug = $state("");
	let formVerified = $state(false);
	let formStatus = $state("draft");

	const slug = $derived(page.params.slug ?? "");
	const canManage = $derived(hasPermission("admin.settings"));
	const canApproveInstructors = $derived(hasPermission("university.instructor.approve"));
	const canApproveTracks = $derived(hasPermission("university.course.approve"));
	const canProcessPayouts = $derived(hasPermission("university.payout.process"));
	const canViewRevenue = $derived(hasPermission("university.revenue.list"));
	const isVerified = $derived(university?.status === "active");

	const subPages = $derived([
		{
			label: t("university.instructors_title", "Instructors"),
			href: "/university/instructors",
			icon: Users,
			permission: "university.instructor.approve",
			description: t("university.instructors_desc", "Review instructor applications")
		},
		{
			label: t("university.tracks_title", "Track Approvals"),
			href: "/university/tracks",
			icon: BookOpen,
			permission: "university.course.approve",
			description: t("university.tracks_desc", "Approve submitted tracks")
		},
		{
			label: t("university.payouts_title", "Payouts"),
			href: "/university/payouts",
			icon: DollarSign,
			permission: "university.payout.process",
			description: t("university.payouts_desc", "Process instructor payouts")
		},
		{
			label: t("university.revenue_splits_title", "Revenue Splits"),
			href: "/university/revenue-splits",
			icon: TrendingUp,
			permission: "university.revenue.list",
			description: t("university.revenue_splits_desc", "Review revenue share")
		}
	]);

	function canSee(permission?: string): boolean {
		if (!permission) return true;
		return hasPermission(permission);
	}

	async function load() {
		loading = true;
		errorMessage = "";
		try {
			const result = await universitiesAdminApi.getBySlug(slug);
			if (result.ok) {
				university = result.data as UniversityDetail;
			} else {
				errorMessage = result.error ?? t("universities.notFound", "University not found");
			}
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : t("common.error", "Failed to load");
		} finally {
			loading = false;
		}
	}

	function openEdit() {
		if (!university) return;
		formName = university.name;
		formSlug = university.slug;
		formVerified = university.status === "active";
		showEditModal = true;
	}

	async function submitEdit() {
		if (!university) return;
		acting = true;
		errorMessage = "";
		try {
			const result = await universitiesAdminApi.update(university.id, {
				name: formName.trim(),
				slug: formSlug.trim(),
				verified: formVerified
			});
			if (result.ok) {
				showEditModal = false;
				successMessage = t("universities.updated", "University updated");
				if (formSlug.trim() && formSlug.trim() !== university.slug) {
					goto(`/university/${formSlug.trim()}`);
				} else {
					load();
				}
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("common.error", "Failed to update");
		} finally {
			acting = false;
		}
	}

	function openDelete() {
		showDeleteModal = true;
	}

	async function executeDelete() {
		if (!university) return;
		acting = true;
		errorMessage = "";
		try {
			const result = await universitiesAdminApi.delete(university.id);
			if (result.ok) {
				showDeleteModal = false;
				goto("/university");
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("common.error", "Failed to delete");
		} finally {
			acting = false;
		}
	}

	async function toggleVerify() {
		if (!university) return;
		acting = true;
		errorMessage = "";
		try {
			const result = isVerified
				? await universitiesAdminApi.unverify(university.id)
				: await universitiesAdminApi.verify(university.id);
			if (result.ok) {
				successMessage = isVerified
					? t("universities.unverified", "University unverified")
					: t("universities.verified", "University verified");
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("common.error", "Failed to update verification");
		} finally {
			acting = false;
		}
	}

	function openStatusModal() {
		if (!university) return;
		formStatus = university.status;
		showStatusModal = true;
	}

	async function submitStatusChange() {
		if (!university) return;
		acting = true;
		errorMessage = "";
		try {
			const result = await universitiesAdminApi.updateStatus(university.id, { status: formStatus });
			if (result.ok) {
				showStatusModal = false;
				successMessage = t("universities.statusUpdated", "Status updated");
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("common.error", "Failed to update status");
		} finally {
			acting = false;
		}
	}

	function fmtDate(v?: string | null): string {
		if (!v) return "—";
		try {
			return new Date(v).toLocaleString();
		} catch {
			return v;
		}
	}

	$effect(() => {
		if (slug) load();
	});
</script>

<svelte:head>
	<title>{university?.name ?? t("university.detail", "University")} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<Button variant="ghost" size="sm" href="/university">
		<ArrowLeft class="mr-1 h-3.5 w-3.5" />
		{t("university.back_to_hub", "Back to University Hub")}
	</Button>
</div>

{#if errorMessage && !university}
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
			{#each Array(3) as _}
				<div class="h-24 animate-pulse rounded-lg border" style="border-color: var(--color-border); background: var(--color-surface)"></div>
			{/each}
		</div>
	</div>
{:else if university}
	<PageHeader
		title={university.name}
		description={university.description ?? t("universities.description", "Manage university registrations and verification")}
		actions={[
			{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: load },
			...(canManage ? [{ label: t("universities.changeStatus", "Change Status"), icon: FileEdit, variant: "outline" as const, onclick: openStatusModal }] : []),
			...(canManage ? [{ label: isVerified ? t("universities.unverify", "Unverify") : t("universities.verify", "Verify"), icon: isVerified ? XCircle : BadgeCheck, variant: "outline" as const, onclick: toggleVerify }] : []),
			...(canManage ? [{ label: t("common.edit", "Edit"), icon: Pencil, variant: "outline" as const, onclick: openEdit }] : []),
			...(canManage ? [{ label: t("common.delete", "Delete"), icon: Trash2, variant: "destructive" as const, onclick: openDelete }] : [])
		]}
	/>

	{#if errorMessage}
		<div class="mb-4 flex items-center justify-between rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			<span>{errorMessage}</span>
			<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
		</div>
	{/if}
	{#if successMessage}
		<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
			{successMessage}
		</div>
	{/if}

	<!-- Identity + stats -->
	<div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-3">
		<Card>
			<CardContent>
				<div class="flex items-center gap-3">
					<div class="flex h-12 w-12 items-center justify-center rounded-lg" style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)">
						{#if university.logo_url}
							<img src={university.logo_url} alt={university.name} class="h-12 w-12 rounded-lg object-cover" />
						{:else}
							<GraduationCap class="h-6 w-6" style="color: var(--color-primary)" />
						{/if}
					</div>
					<div class="min-w-0">
						<p class="truncate text-sm font-semibold" style="color: var(--color-foreground)">{university.name}</p>
						<p class="truncate font-mono text-xs" style="color: var(--color-muted)">{university.slug}</p>
						<div class="mt-1 flex items-center gap-2">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {isVerified ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning'}">{university.status}</span>
							{#if isVerified}
								<BadgeCheck class="h-3.5 w-3.5" style="color: var(--color-success)" />
							{/if}
						</div>
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.students", "Members")}</p>
				<p class="mt-1 text-2xl font-semibold" style="color: var(--color-foreground)">{(university.member_count ?? 0).toLocaleString()}</p>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{(university.follower_count ?? 0).toLocaleString()} {t("university.followers", "followers")} · {(university.post_count ?? 0).toLocaleString()} {t("university.posts", "posts")}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.created", "Created")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{fmtDate(university.created_at)}</p>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t("universities.updated", "Updated")}: {fmtDate(university.updated_at)}</p>
				{#if university.geo_code}
					<p class="mt-1 text-xs" style="color: var(--color-muted)">{t("university.country", "Country")}: {university.geo_code}</p>
				{/if}
			</CardContent>
		</Card>
	</div>

	<!-- Creator-economy sub-pages -->
	<div class="mb-6">
		<h2 class="mb-3 text-sm font-semibold" style="color: var(--color-foreground)">{t("university.creator_economy", "Creator Economy")}</h2>
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
			{#each subPages as sp}
				{#if canSee(sp.permission)}
					<a
						href={sp.href}
						class="group rounded-lg border p-4 transition-colors hover:border-primary/40"
						style="border-color: var(--color-border); background-color: var(--color-surface)"
					>
						<div class="flex h-9 w-9 items-center justify-center rounded-lg" style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)">
							<sp.icon class="h-5 w-5" style="color: var(--color-primary)" />
						</div>
						<p class="mt-3 text-sm font-medium" style="color: var(--color-foreground)">{sp.label}</p>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">{sp.description}</p>
						<div class="mt-2 flex items-center gap-1 text-xs font-medium" style="color: var(--color-primary)">
							{t("common.open", "Open")}
							<ArrowRight class="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
						</div>
					</a>
				{/if}
			{/each}
		</div>
	</div>

	<!-- Custom fields -->
	{#if university.fields && university.fields.length > 0}
		<Card>
			<CardHeader>
				<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">{t("university.custom_fields", "University Fields")}</h2>
			</CardHeader>
			<CardContent>
				<dl class="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
					{#each university.fields as f}
						<div>
							<dt class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{f.field_id}</dt>
							<dd class="mt-0.5 text-sm" style="color: var(--color-foreground)">{f.value ?? "—"}</dd>
						</div>
					{/each}
				</dl>
			</CardContent>
		</Card>
	{/if}
{/if}

<!-- Edit Modal -->
<Modal bind:open={showEditModal} title={t("universities.edit", "Edit University")} size="default">
	{#if university}
		<div class="space-y-3">
			<div>
				<label for="edit-uni-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.name", "Name")}</label>
				<input id="edit-uni-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} />
			</div>
			<div>
				<label for="edit-uni-domain" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.domain_slug", "Domain")}</label>
				<input id="edit-uni-domain" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formSlug} />
			</div>
			<div class="flex items-center gap-2">
				<input id="edit-uni-verified" type="checkbox" bind:checked={formVerified} />
				<label for="edit-uni-verified" class="text-sm">{t("universities.verified", "Verified")}</label>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showEditModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitEdit} disabled={acting || !formName.trim()}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- Delete Modal -->
<Modal bind:open={showDeleteModal} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if university}
		<p class="text-sm" style="color: var(--color-foreground)">{t("universities.deleteConfirm", "Delete university")} <strong>{university.name}</strong>? {t("universities.deleteWarning", "This cannot be undone.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- Status Modal -->
<Modal bind:open={showStatusModal} title={t("universities.changeStatus", "Change Status")} size="sm">
	{#if university}
		<p class="mb-3 text-sm" style="color: var(--color-foreground)">{t("universities.statusFor", "Status for")} <strong>{university.name}</strong></p>
		<div class="space-y-2">
			{#each ["draft", "published", "archived"] as s}
				<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
					<input type="radio" name="status" value={s} bind:group={formStatus} />
					{s}
				</label>
			{/each}
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showStatusModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitStatusChange} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>
