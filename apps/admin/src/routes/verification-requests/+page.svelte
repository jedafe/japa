<script lang="ts">
	// §07 Verification Requests — admin review queue.
	// Two-step flow: approve/reject records the review only; "Grant badge"
	// on approved rows calls POST /admin/{organizations|users}/{id}/verify.
	import AutocompleteSelect from '#lib/components/ui/AutocompleteSelect.svelte';
	import { siteName } from '#lib/stores/site.svelte.js';
	import { EntityPage, type EntityPageConfig, type EntityPageController } from "#lib/components/entity/index.js";
	import { Modal } from "#lib/components/ui/modal/index.js";
	import { Button } from "#lib/components/ui/button/index.js";
	import { verificationAdminApi, organisationsAdminApi, type VerificationTypeItem } from "#lib/api.js";
	import { Tabs, TabsContent } from "#lib/components/ui/tabs/index.js";
	import BadgeTypesPanel from "./BadgeTypesPanel.svelte";
	import { t } from "#lib/stores/i18n.svelte.js";
	import { hasPermission } from "#lib/stores/permissions.svelte.js";
	import { Eye, Check, X, BadgeCheck } from "lucide-svelte";
	import type { ActionItem, ActionContext } from "#lib/components/ui/data-table/index.js";

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
		requester_verified?: boolean;
		org_name?: string | null;
		org_slug?: string | null;
		org_verified?: boolean | null;
		reviewed_by_username?: string | null;
	};

	type Submission = {
		field_values: Record<string, unknown> | null;
	};

	const canModerate = $derived(hasPermission("admin.moderation"));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let successMessage = $state("");
	let busy = $state(false);

	// Page-local state-action modals (EntityPage's built-in confirm only
	// covers the default delete — state actions keep their own Modals).
	let rejectTarget = $state<VRequest | null>(null);
	let showRejectModal = $state(false);
	let rejectNote = $state("");
	let grantTarget = $state<VRequest | null>(null);
	let showGrantModal = $state(false);
	let grantType = $state("");
	let types = $state<VerificationTypeItem[]>([]);
	let activeTab = $state("requests");

	$effect(() => {
		verificationAdminApi.types().then((res) => {
			if (res.ok) types = res.data.types ?? [];
		}).catch(() => {});
	});

	// Grant modal options — scoped to the target request's entity_type
	// (organization rows see organization types, users see user types).
	const grantTypeOptions = $derived(
		types
			.filter((ty) => ty.scope === (grantTarget?.entity_type === "organization" ? "organization" : "user"))
			.map((ty) => ({ value: ty.value, label: ty.label }))
	);

	// Only http(s)/relative URLs may render as links — field_values are
	// requester-controlled, so anything else renders as plain text (XSS).
	// Protocol-relative "//host" is rejected: it silently goes off-site.
	function isSafeHref(u: unknown): u is string {
		if (typeof u !== "string" || u.startsWith("//")) return false;
		return /^https?:\/\//i.test(u) || u.startsWith("/") || u.startsWith("#");
	}

	const entityVerified = (row: VRequest) =>
		row.entity_type === "organization" ? !!row.org_verified : !!row.requester_verified;

	const entityLabel = (row: VRequest) =>
		row.entity_type + (row.org_name ? ` — ${row.org_name}` : "");

	// Evidence (view modal) — the linked form submission is fetched per row.
	// viewTargetId guards against a slow fetch for a previously-opened row
	// overwriting the current row's evidence.
	let submission = $state<Submission | null>(null);
	let submissionLoading = $state(false);
	let viewTargetId = $state("");

	function openView(row: VRequest, ctx?: ActionContext) {
		viewTargetId = row.id;
		submission = null;
		submissionLoading = true;
		verificationAdminApi.requestSubmission(row.id)
			.then((res) => { if (viewTargetId === row.id) submission = res.ok ? res.data : null; })
			.catch(() => { if (viewTargetId === row.id) submission = null; })
			.finally(() => { if (viewTargetId === row.id) submissionLoading = false; });
		ctx?.openView(row);
	}

	function openReject(row: VRequest) { rejectTarget = row; rejectNote = ""; showRejectModal = true; }
	function openGrant(row: VRequest) { grantTarget = row; grantType = row.requested_type ?? ""; showGrantModal = true; }

	async function approve(row: VRequest, ctx?: ActionContext) {
		if (busy) return;
		busy = true;
		pageError = "";
		successMessage = "";
		const res = await verificationAdminApi.review(row.id, { status: "approved" });
		busy = false;
		if (!res.ok) { pageError = res.error; return; }
		successMessage = t("vreq.approved", "Request approved — grant the badge when ready.");
		ctx?.reload();
	}

	async function confirmReject() {
		if (!rejectTarget || busy) return;
		busy = true;
		pageError = "";
		successMessage = "";
		const res = await verificationAdminApi.review(rejectTarget.id, { status: "rejected", resolution_note: rejectNote });
		busy = false;
		if (!res.ok) { pageError = res.error; return; }
		showRejectModal = false;
		rejectTarget = null;
		successMessage = t("vreq.rejected", "Request rejected.");
		entityController?.reload();
	}

	async function confirmGrant() {
		if (!grantTarget || busy) return;
		busy = true;
		pageError = "";
		successMessage = "";
		const res = grantTarget.entity_type === "organization" && grantTarget.organization_id
			? await organisationsAdminApi.verify(grantTarget.organization_id, { verification_type: grantType || undefined })
			: await verificationAdminApi.verifyUser(grantTarget.requester_user_id, { verification_type: grantType || undefined });
		busy = false;
		if (!res.ok) { pageError = res.error; return; }
		showGrantModal = false;
		grantTarget = null;
		successMessage = t("vreq.granted", "Verification badge granted.");
		entityController?.reload();
	}

	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t("vreq.view", "View"), icon: Eye, onclick: openView }
		];
		if (canModerate) {
			acts.push({
				label: t("vreq.approve", "Approve"),
				icon: Check,
				onclick: approve,
				show: (row: VRequest) => row.status === "pending"
			});
			acts.push({
				label: t("vreq.reject", "Reject"),
				icon: X,
				variant: "destructive",
				onclick: openReject,
				show: (row: VRequest) => row.status === "pending"
			});
			acts.push({
				label: t("vreq.grant", "Grant badge"),
				icon: BadgeCheck,
				onclick: openGrant,
				// Two-step flow: the badge can only be granted on an approved
				// request whose entity does not already carry it.
				show: (row: VRequest) => row.status === "approved" && !entityVerified(row)
			});
		}
		return acts;
	});

	const statusBadge = (v: string) => {
		const colors =
			v === "approved" ? "bg-success/15 text-success dark:text-success" :
			v === "rejected" ? "bg-danger/15 text-danger dark:text-danger" :
			v === "pending" ? "bg-warning/15 text-warning dark:text-warning" :
			"bg-muted/30 text-muted dark:text-muted";
		return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
	};

	const config = $derived<EntityPageConfig>({
		title: t("vreq.title", "Verification Requests"),
		description: t("vreq.desc", "Review evidence submissions and manage verification badges"),
		hideCreate: true,
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: "requester_username", label: t("vreq.col.requester", "Requester"), sortable: true, sortKey: "requester", componentSnippet: requesterCell },
			{ key: "entity_type", label: t("vreq.col.entity", "Entity"), sortable: true, componentSnippet: entityCell },
			{ key: "requested_type", label: t("vreq.col.type", "Suggested type"), render: (v: string | null) => v ? v.replaceAll("_", " ") : `<span style="color: var(--color-muted)">—</span>` },
			{ key: "status", label: t("vreq.col.status", "Status"), sortable: true, render: statusBadge },
			{ key: "badge", label: t("vreq.col.badge", "Badge"), componentSnippet: badgeCell, hideable: false },
			{ key: "created_at", label: t("vreq.col.filed", "Filed"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() },
			{
				key: "reviewed_by_username",
				label: t("vreq.col.reviewed_by", "Reviewed by"),
				defaultHidden: true,
				render: (v: string | null, row: VRequest) => v ? `${v} · ${row.reviewed_at ? new Date(row.reviewed_at).toLocaleDateString() : ""}` : `<span style="color: var(--color-muted)">—</span>`
			}
		],
		filters: [
			{
				key: "status",
				label: t("vreq.col.status", "Status"),
				options: [
					{ value: "pending", label: t("vreq.status_pending", "Pending") },
					{ value: "approved", label: t("vreq.status_approved", "Approved") },
					{ value: "rejected", label: t("vreq.status_rejected", "Rejected") },
					{ value: "withdrawn", label: t("vreq.status_withdrawn", "Withdrawn") }
				]
			},
			{
				key: "entity_type",
				label: t("vreq.col.entity", "Entity"),
				options: [
					{ value: "organization", label: t("vreq.entity_organization", "Organization") },
					{ value: "user", label: t("vreq.entity_user", "User") }
				]
			}
		],
		actions: rowActions,
		fields: [],
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page: String(page + 1),
				page_size: String(pageSize)
			});
			if (q) params.set("q", q);
			if (filter.status) params.set("status", filter.status);
			if (filter.entity_type) params.set("entity_type", filter.entity_type);
			// EntityPage passes sort as "key:dir"; the API takes sort + sort_dir.
			if (sort) {
				const [key, dir] = sort.split(":");
				params.set("sort", key);
				params.set("sort_dir", dir === "asc" ? "asc" : "desc");
			}
			const res = await verificationAdminApi.list(params.toString());
			if (!res.ok) throw new Error(res.error ?? t("common.error", "Failed to load"));
			return {
				data: res.data.data ?? [],
				total: res.data.meta?.total ?? 0,
				totalPages: Math.max(res.data.meta?.page_count ?? 1, 1)
			};
		},
		viewSnippet: RequestView
	});
</script>

<svelte:head><title>{t("vreq.title", "Verification Requests")} — {siteName()}</title></svelte:head>

{#snippet requesterCell({ row }: { row: VRequest })}
	<span>{row.requester_username}</span>
	<span class="block text-xs" style="color: var(--color-muted)">{row.requester_email}</span>
{/snippet}

{#snippet entityCell({ row }: { row: VRequest })}
	<span>{entityLabel(row)}</span>
{/snippet}

{#snippet badgeCell({ row }: { row: VRequest })}
	{#if entityVerified(row)}
		<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">{t("vreq.badge_granted", "Verified")}</span>
	{:else}
		<span style="color: var(--color-muted)">—</span>
	{/if}
{/snippet}

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)" role="alert">{pageError}</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)" role="status">{successMessage}</div>
{/if}

<Tabs
	tabs={[
		{ value: "requests", label: t("vreq.tab_requests", "Requests") },
		{ value: "types", label: t("vreq.tab_types", "Badge types") }
	]}
	bind:value={activeTab}
	tabParam="tab"
	tabPreserve={[]}
>
	<TabsContent value="requests">
		<div class="mt-4"><EntityPage {config} bind:controller={entityController} /></div>
	</TabsContent>
	<TabsContent value="types">
		<div class="mt-4"><BadgeTypesPanel /></div>
	</TabsContent>
</Tabs>

{#snippet RequestView({ row }: { row: VRequest })}
	<dl class="space-y-2 text-sm">
		<div><dt class="font-medium">{t("vreq.col.requester", "Requester")}</dt><dd>{row.requester_username} ({row.requester_email})</dd></div>
		<div><dt class="font-medium">{t("vreq.col.entity", "Entity")}</dt><dd>{entityLabel(row)}</dd></div>
		{#if row.requested_type}<div><dt class="font-medium">{t("vreq.col.type", "Suggested type")}</dt><dd>{row.requested_type.replaceAll("_", " ")}</dd></div>{/if}
		{#if row.resolution_note}<div><dt class="font-medium">{t("vreq.resolution_note", "Resolution note")}</dt><dd>{row.resolution_note}</dd></div>{/if}
		{#if row.reviewed_by_username}<div><dt class="font-medium">{t("vreq.col.reviewed_by", "Reviewed by")}</dt><dd>{row.reviewed_by_username}{row.reviewed_at ? ` · ${new Date(row.reviewed_at).toLocaleDateString()}` : ""}</dd></div>{/if}
	</dl>
	{#if submissionLoading}
		<p class="mt-4 text-sm" style="color: var(--color-muted)">{t("vreq.loading_evidence", "Loading evidence…")}</p>
	{:else if submission?.field_values && typeof submission.field_values === "object" && !Array.isArray(submission.field_values)}
		<h3 class="mt-4 text-sm font-semibold">{t("vreq.submission", "Form submission")}</h3>
		<dl class="mt-2 space-y-1 text-sm">
			{#each Object.entries(submission.field_values) as [k, v]}
				<div>
					<dt class="font-medium">{k}</dt>
					<dd class="whitespace-pre-wrap">
						{#if Array.isArray(v)}
							{#each v as item}
								{#if item && typeof item === "object" && (item.url || item.id)}
									{#if isSafeHref(item.url)}
										<a class="block" style="color: var(--color-primary)" href={item.url} target="_blank" rel="noopener noreferrer">{item.alt ?? item.url}</a>
									{:else}
										<span class="block">{item.url ?? item.id}</span>
									{/if}
								{:else}
									{item ?? "—"}
								{/if}
							{/each}
						{:else if v && typeof v === "object" && ((v as Record<string, unknown>).url || (v as Record<string, unknown>).id)}
							{@const vo = v as Record<string, unknown>}
							{#if isSafeHref(vo.url)}
								<a style="color: var(--color-primary)" href={String(vo.url)} target="_blank" rel="noopener noreferrer">{vo.alt ?? vo.url}</a>
							{:else}
								{String(vo.url ?? vo.id)}
							{/if}
						{:else}
							{v ?? "—"}
						{/if}
					</dd>
				</div>
			{/each}
		</dl>
	{:else}
		<p class="mt-4 text-sm" style="color: var(--color-muted)">{t("vreq.no_evidence", "No submission evidence attached")}</p>
	{/if}
{/snippet}

<Modal bind:open={showRejectModal} title={t("vreq.reject", "Reject request")}>
	<label class="block text-sm">
		{t("vreq.resolution_note", "Resolution note")}
		<textarea bind:value={rejectNote} rows="3" class="mt-1 w-full rounded-md border px-3 py-2" style="border-color: var(--color-border); background: var(--color-surface)"></textarea>
	</label>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showRejectModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" disabled={busy} onclick={confirmReject}>{t("vreq.reject", "Reject")}</Button>
	</div>
</Modal>

<Modal bind:open={showGrantModal} title={t("vreq.grant", "Grant verification badge")}>
	<label class="block text-sm">
		{t("vreq.grant_type", "Badge type")}
		<AutocompleteSelect
			bind:value={grantType}
			class="mt-1"
			options={[
				{ value: '', label: t("vreq.grant_type_none", "— none —") },
				...grantTypeOptions
			]}
		/>
	</label>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showGrantModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button disabled={busy} onclick={confirmGrant}>{t("vreq.grant", "Grant badge")}</Button>
	</div>
</Modal>
