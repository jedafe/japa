<script lang="ts">
	// §07 Verification Requests — admin review queue.
	// Two-step flow: approve/reject records the review only; "Grant badge"
	// on approved rows calls POST /admin/{organizations|users}/{id}/verify.
	import { siteName } from '$lib/stores/site.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { verificationAdminApi, organisationsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type VRequest = {
		id: string;
		requester_user_id: string;
		entity_type: string;
		organization_id?: string | null;
		requested_type?: string | null;
		form_submission_id: string;
		status: string;
		resolution_note?: string | null;
		created_at: string;
		reviewed_at?: string | null;
		requester_username?: string;
		requester_email?: string;
		org_name?: string | null;
		org_slug?: string | null;
		reviewed_by_username?: string | null;
	};

	type Submission = {
		field_values: Record<string, unknown> | null;
	};

	let requests = $state<VRequest[]>([]);
	let loading = $state(true);
	let pageError = $state("");
	let successMessage = $state("");
	let statusFilter = $state("pending");
	let entityFilter = $state("");

	let showViewModal = $state(false);
	let showRejectModal = $state(false);
	let showGrantModal = $state(false);
	let target = $state<VRequest | null>(null);
	let submission = $state<Submission | null>(null);
	let rejectNote = $state("");
	let grantType = $state("");
	let busy = $state(false);
	let types = $state<string[]>([]);

	const canModerate = $derived(hasPermission("admin.moderation"));

	async function load() {
		loading = true;
		pageError = "";
		const params = new URLSearchParams({ page_size: "100" });
		if (statusFilter) params.set("status", statusFilter);
		if (entityFilter) params.set("entity_type", entityFilter);
		const res = await verificationAdminApi.list(params.toString());
		if (!res.ok) { pageError = res.error; requests = []; }
		else { requests = res.data.data ?? []; }
		loading = false;
	}

	function openView(row: VRequest) {
		target = row;
		submission = null;
		showViewModal = true;
		organisationsAdminApi.claimSubmission(row.id)
			.then((res) => { submission = res.ok ? res.data : null; })
			.catch(() => { submission = null; });
	}

	async function approve(row: VRequest) {
		busy = true;
		const res = await verificationAdminApi.review(row.id, { status: "approved" });
		busy = false;
		if (!res.ok) { pageError = res.error; return; }
		successMessage = t("vreq.approved", "Request approved — grant the badge when ready.");
		await load();
	}

	async function reject() {
		if (!target) return;
		busy = true;
		const res = await verificationAdminApi.review(target.id, { status: "rejected", resolution_note: rejectNote });
		busy = false;
		showRejectModal = false;
		rejectNote = "";
		if (!res.ok) { pageError = res.error; return; }
		successMessage = t("vreq.rejected", "Request rejected.");
		await load();
	}

	async function grantBadge() {
		if (!target) return;
		busy = true;
		const res = target.entity_type === "organization" && target.organization_id
			? await organisationsAdminApi.verify(target.organization_id, { verification_type: grantType || undefined } as never)
			: await verificationAdminApi.verifyUser(target.requester_user_id, { verification_type: grantType || undefined });
		busy = false;
		showGrantModal = false;
		if (!res.ok) { pageError = res.error; return; }
		successMessage = t("vreq.granted", "Verification badge granted.");
		await load();
	}

	$effect(() => { statusFilter; entityFilter; load(); });
	verificationAdminApi.types().then((res) => { if (res.ok) types = res.data.data.types; }).catch(() => {});
</script>

<svelte:head><title>{t("vreq.title", "Verification Requests")} — {siteName()}</title></svelte:head>

<PageHeader title={t("vreq.title", "Verification Requests")} description={t("vreq.desc", "Review evidence submissions and manage verification badges")} />

{#if pageError}<p class="mb-3 rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{pageError}</p>{/if}
{#if successMessage}<p class="mb-3 rounded-md bg-green-500/10 px-3 py-2 text-sm text-green-600" role="status">{successMessage}</p>{/if}

<div class="mb-4 flex gap-3">
	<select bind:value={statusFilter} class="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm">
		<option value="pending">Pending</option>
		<option value="approved">Approved</option>
		<option value="rejected">Rejected</option>
		<option value="withdrawn">Withdrawn</option>
		<option value="">All</option>
	</select>
	<select bind:value={entityFilter} class="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm">
		<option value="">All entities</option>
		<option value="organization">Organization</option>
		<option value="user">User</option>
	</select>
</div>

{#if loading}
	<TableSkeleton rows={5} />
{:else}
	<div class="overflow-x-auto rounded-lg border border-[var(--color-border)]">
		<table class="w-full text-sm">
			<thead class="bg-[var(--color-surface-subtle)] text-left text-xs uppercase text-[var(--color-muted)]">
				<tr>
					<th class="px-4 py-3">{t("vreq.col.requester", "Requester")}</th>
					<th class="px-4 py-3">{t("vreq.col.entity", "Entity")}</th>
					<th class="px-4 py-3">{t("vreq.col.type", "Suggested type")}</th>
					<th class="px-4 py-3">{t("vreq.col.status", "Status")}</th>
					<th class="px-4 py-3">{t("vreq.col.filed", "Filed")}</th>
					<th class="px-4 py-3">{t("vreq.col.actions", "Actions")}</th>
				</tr>
			</thead>
			<tbody>
				{#each requests as row (row.id)}
					<tr class="border-t border-[var(--color-border)]">
						<td class="px-4 py-3">{row.requester_username}<span class="block text-xs text-[var(--color-muted)]">{row.requester_email}</span></td>
						<td class="px-4 py-3">{row.entity_type}{row.org_name ? ` — ${row.org_name}` : ""}</td>
						<td class="px-4 py-3">{row.requested_type ?? "—"}</td>
						<td class="px-4 py-3">{row.status}</td>
						<td class="px-4 py-3">{new Date(row.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3">
							<div class="flex gap-2">
								<Button size="sm" variant="outline" onclick={() => openView(row)}>{t("vreq.view", "View")}</Button>
								{#if canModerate && row.status === "pending"}
									<Button size="sm" variant="outline" onclick={() => approve(row)}>{t("vreq.approve", "Approve")}</Button>
									<Button size="sm" variant="destructive" onclick={() => { target = row; showRejectModal = true; }}>{t("vreq.reject", "Reject")}</Button>
								{/if}
								{#if canModerate && row.status === "approved"}
									<Button size="sm" onclick={() => { target = row; grantType = row.requested_type ?? ""; showGrantModal = true; }}>{t("vreq.grant", "Grant badge")}</Button>
								{/if}
							</div>
						</td>
					</tr>
				{:else}
					<tr><td colspan="6" class="px-4 py-8 text-center text-[var(--color-muted)]">{t("vreq.empty", "No verification requests")}</td></tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<Modal open={showViewModal} onclose={() => (showViewModal = false)} title={t("vreq.evidence", "Request evidence")}>
	{#if target}
		<dl class="space-y-2 text-sm">
			<div><dt class="font-medium">{t("vreq.col.requester", "Requester")}</dt><dd>{target.requester_username} ({target.requester_email})</dd></div>
			<div><dt class="font-medium">{t("vreq.col.entity", "Entity")}</dt><dd>{target.entity_type}{target.org_name ? ` — ${target.org_name}` : ""}</dd></div>
			{#if target.requested_type}<div><dt class="font-medium">{t("vreq.col.type", "Suggested type")}</dt><dd>{target.requested_type}</dd></div>{/if}
			{#if target.resolution_note}<div><dt class="font-medium">{t("vreq.resolution_note", "Resolution note")}</dt><dd>{target.resolution_note}</dd></div>{/if}
		</dl>
		{#if submission?.field_values}
			<h3 class="mt-4 text-sm font-semibold">{t("vreq.submission", "Form submission")}</h3>
			<dl class="mt-2 space-y-1 text-sm">
				{#each Object.entries(submission.field_values) as [k, v]}
					<div><dt class="font-medium">{k}</dt><dd class="whitespace-pre-wrap">{String(v)}</dd></div>
				{/each}
			</dl>
		{/if}
	{/if}
</Modal>

<Modal open={showRejectModal} onclose={() => (showRejectModal = false)} title={t("vreq.reject", "Reject request")}>
	<label class="block text-sm">
		{t("vreq.resolution_note", "Resolution note")}
		<textarea bind:value={rejectNote} rows="3" class="mt-1 w-full rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2"></textarea>
	</label>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showRejectModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" disabled={busy} onclick={reject}>{t("vreq.reject", "Reject")}</Button>
	</div>
</Modal>

<Modal open={showGrantModal} onclose={() => (showGrantModal = false)} title={t("vreq.grant", "Grant verification badge")}>
	<label class="block text-sm">
		{t("vreq.grant_type", "Badge type")}
		<select bind:value={grantType} class="mt-1 w-full rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2">
			<option value="">{t("vreq.grant_type_none", "— none —")}</option>
			{#each types as ty}<option value={ty}>{ty.replaceAll("_", " ")}</option>{/each}
		</select>
	</label>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showGrantModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button disabled={busy} onclick={grantBadge}>{t("vreq.grant", "Grant badge")}</Button>
	</div>
</Modal>
