<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /moderation — canonical review queue (merged with the former
	// /moderation-queue page, which now redirects here). EntityPage scaffold
	// with state actions: approve/reject/delete (reason modal), Assign to Me,
	// and §61 ad-campaign approve/reject, plus bulk approve/reject/assign.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "#lib/components/entity/index.js";
	import { Modal } from "#lib/components/ui/modal/index.js";
	import { Button } from "#lib/components/ui/button/index.js";
	import { moderationAdminApi, adsAdminApi } from "#lib/api.js";
	import { t } from "#lib/stores/i18n.svelte.js";
	import { hasPermission } from "#lib/stores/permissions.svelte.js";
	import { user } from "#lib/stores/auth.svelte.js";
	import { Eye, Check, X, Trash2, UserPlus, Ban } from "lucide-svelte";
	import type { ActionItem, FilterDef } from "#lib/components/ui/data-table/index.js";

	type ModItem = {
		id: string;
		type: string;
		status: string;
		content?: string;
		reporter?: string;
		author_id?: string;
		created_at: string;
		entity_ref?: string;
	};

	const canModerate = $derived(hasPermission("admin.moderation"));

	// ── Filters ─────────────────────────────────────────────────────────────
	// Rule 16: moderation status and content_type options are loaded from
	// GET /admin/moderation/options instead of being hardcoded.
	let statusOptions = $state<{ value: string; label: string }[]>([
		{ value: "flagged", label: t("app.moderation.queue.flagged", "Flagged") },
		{ value: "published", label: t("app.moderation.queue.published", "Published") },
		{ value: "removed", label: t("app.moderation.queue.removed", "Removed") },
		{ value: "pending_review", label: t("app.moderation.queue.pending_review", "Pending Review") }
	]);
	let contentTypeOptions = $state<{ value: string; label: string }[]>([
		{ value: "organization", label: t("app.moderation.queue.organization", "Organization") },
		{ value: "service", label: t("app.moderation.queue.service", "Service") },
		{ value: "mentor", label: t("app.moderation.queue.mentor", "Mentor") },
		{ value: "university", label: t("app.moderation.queue.university", "University") },
		{ value: "ad_campaign", label: t("app.moderation.queue.ad_campaign", "Ad Campaign") }
	]);

	$effect(() => {
		void (async () => {
			const res = await moderationAdminApi.getOptions();
			if (res.ok && res.data) {
				if (res.data.statuses?.length) {
					statusOptions = res.data.statuses.map((s) => ({
						value: s.value,
						label: t(`app.moderation.queue.${s.value}`, s.label)
					}));
					// ad_campaign moderation still exposes a pending_review status that
					// the generic catalog does not yet include.
					if (!statusOptions.some((o) => o.value === "pending_review")) {
						statusOptions = [
							...statusOptions,
							{ value: "pending_review", label: t("app.moderation.queue.pending_review", "Pending Review") }
						];
					}
				}
				if (res.data.content_types?.length) {
					contentTypeOptions = res.data.content_types.map((c) => ({
						value: c.value,
						label: t(`app.moderation.queue.${c.value}`, c.label)
					}));
				}
			}
		})();
	});

	const filters = $derived<FilterDef[]>([
		{
			key: "status",
			label: t("common.status", "Status"),
			options: statusOptions
		},
		{
			key: "content_type",
			label: t("moderation.content_type", "Content Type"),
			options: contentTypeOptions
		}
	]);

	// ── Bulk actions ────────────────────────────────────────────────────────
	// EntityPage auto-reloads + clears selection after bulk handlers complete.
	// The POST /admin/moderation-queue/bulk endpoint only processes
	// type='review' items, so bulk approve/reject loop the per-item PATCH
	// (ModerateItem) which covers every queue source — reviews, persona
	// actions, studio HITL and tool reviews. Bulk "dismiss" has no backend
	// action, so it is not offered.
	let pageError = $state("");
	let bulkActing = $state(false);

	async function bulkModerate(ids: Set<string>, action: "approve" | "reject") {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkActing) return; // in-flight guard
		bulkActing = true;
		pageError = "";
		try {
			const results = await Promise.allSettled(
				idArr.map((id) => moderationAdminApi.updateItem(id, { action }))
			);
			const failed = results.filter(
				(r) => r.status === "rejected" || (r.status === "fulfilled" && !r.value.ok)
			);
			if (failed.length > 0) {
				pageError = t("moderation.bulk_error", "Bulk action failed. Please try again.");
			}
		} finally {
			bulkActing = false;
		}
	}

	async function bulkAssignToMe(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkActing) return; // in-flight guard
		const assigneeId = user.current?.id;
		if (!assigneeId) {
			pageError = t("moderation.assign_self_unavailable", "Unable to determine your user id");
			return;
		}
		bulkActing = true;
		pageError = "";
		try {
			const results = await Promise.allSettled(
				idArr.map((id) => moderationAdminApi.assignItem(id, { assignee_id: assigneeId }))
			);
			const failed = results.filter(
				(r) => r.status === "rejected" || (r.status === "fulfilled" && !r.value.ok)
			);
			if (failed.length > 0) {
				pageError = t("moderation.bulk_error", "Bulk action failed. Please try again.");
			}
		} finally {
			bulkActing = false;
		}
	}

	// ── Single-row assign + ad-campaign actions ─────────────────────────────
	// "Assign to Me" uses the current admin user's id as the assignee.
	async function assignToMe(row: ModItem, ctx?: { reload?: () => void }) {
		const assigneeId = user.current?.id;
		if (!assigneeId) {
			pageError = t("moderation.assign_self_unavailable", "Unable to determine your user id");
			return;
		}
		pageError = "";
		try {
			const res = await moderationAdminApi.assignItem(row.id, { assignee_id: assigneeId });
			if (res.ok) {
				ctx?.reload?.();
			} else {
				pageError = res.error ?? t("common.error", "An error occurred");
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "An error occurred");
		}
	}

	// §61: ad campaigns live outside content_reviews (hard FK to contents), so
	// approve/reject go through the ads endpoints — activate sets status to
	// active, reject sets rejected + enqueues a refund.
	async function approveAdCampaign(row: ModItem, ctx?: { reload?: () => void }) {
		pageError = "";
		try {
			const res = await adsAdminApi.activateCampaign(row.id);
			if (res.ok) {
				ctx?.reload?.();
			} else {
				pageError = res.error ?? t("common.error", "An error occurred");
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "An error occurred");
		}
	}

	async function rejectAdCampaign(row: ModItem, ctx?: { reload?: () => void }) {
		pageError = "";
		try {
			const res = await adsAdminApi.rejectCampaign(row.id);
			if (res.ok) {
				ctx?.reload?.();
			} else {
				pageError = res.error ?? t("common.error", "An error occurred");
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "An error occurred");
		}
	}

	// ── State-action confirmation modal ─────────────────────────────────────
	// EntityPage's confirm Modal only covers default delete; the moderation
	// approve/reject/delete actions need an optional reason, so they keep a
	// page-local Modal. After the API call succeeds, controller.reload() refetches.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let acting = $state(false);
	let actionTarget = $state<ModItem | null>(null);
	let actionType = $state<"approve" | "reject" | "delete">("approve");
	let actionReason = $state("");
	let showActionModal = $state(false);

	function openAction(item: ModItem, action: "approve" | "reject" | "delete") {
		actionTarget = item;
		actionType = action;
		actionReason = "";
		showActionModal = true;
	}

	async function executeAction() {
		if (!actionTarget) return;
		acting = true;
		try {
			const body: Record<string, unknown> = { action: actionType };
			if (actionReason.trim()) body.reason = actionReason.trim();
			const result = await moderationAdminApi.updateItem(actionTarget.id, body);
			if (result.ok) {
				showActionModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Action failed");
			}
		} catch {
			pageError = t("common.error", "Action failed");
		} finally {
			acting = false;
		}
	}

	// ── Row-action predicates ───────────────────────────────────────────────
	// Generic approve/reject/delete go through PATCH /admin/moderation-queue/{id}
	// which covers reviews, persona actions, studio HITL and tool reviews — but
	// not ad campaigns (they live outside content_reviews and have their own
	// ads endpoints) and only while the item is still open.
	const OPEN_STATUSES = new Set(["pending", "flagged", "draft", "ai_draft"]);
	const isOpen = (row: ModItem) => row.type !== "ad_campaign" && OPEN_STATUSES.has(row.status);
	// PATCH /{id}/assign probes every queue source table (migration 000988
	// added assigned_to to the tool tables + reviews). ad_campaigns have no
	// assignee column — excluded so the action can't 404 on them.
	const isAssignable = (row: ModItem) => row.type !== "ad_campaign";

	// ── Row actions ─────────────────────────────────────────────────────────
	// Custom actions replace defaults, so View is included explicitly.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{
				label: t("common.view", "View"),
				icon: Eye,
				onclick: (row: ModItem, ctx) => ctx?.openView(row)
			}
		];
		if (canModerate) {
			acts.push({
				label: t("moderation.assign_to_me", "Assign to Me"),
				icon: UserPlus,
				onclick: (row: ModItem, ctx) => assignToMe(row, ctx),
				show: isAssignable
			});
			acts.push({
				label: t("moderation.approve", "Approve"),
				icon: Check,
				onclick: (row: ModItem) => openAction(row, "approve"),
				show: isOpen
			});
			acts.push({
				label: t("moderation.reject", "Reject"),
				icon: X,
				variant: "destructive",
				onclick: (row: ModItem) => openAction(row, "reject"),
				show: isOpen
			});
			acts.push({
				label: t("moderation.approve", "Approve"),
				icon: Check,
				onclick: (row: ModItem, ctx) => approveAdCampaign(row, ctx),
				show: (row: ModItem) => row.type === "ad_campaign" && row.status === "pending_review"
			});
			acts.push({
				label: t("moderation.reject", "Reject"),
				icon: Ban,
				variant: "destructive",
				onclick: (row: ModItem, ctx) => rejectAdCampaign(row, ctx),
				show: (row: ModItem) => row.type === "ad_campaign" && row.status === "pending_review"
			});
			acts.push({
				label: t("common.delete", "Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: ModItem) => openAction(row, "delete"),
				show: isOpen
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig but this page is readonly (no
	// create/edit form), so an empty array suffices.
	const fields: FieldDef[] = [];

	// Config built as a $derived so canModerate / rowActions updates propagate.
	const config = $derived<EntityPageConfig>({
		title: t("moderationQueue.title", "Moderation Queue"),
		description: t("moderationQueue.description", "Unified moderation queue for reports and reviews"),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: "type",
				label: t("moderation.content_type", "Content Type"),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">${v}</span>`
			},
			{
				key: "content",
				label: t("moderation.content_preview", "Content"),
				render: (v: string) => v ? `<span class="text-xs max-w-xs truncate block">${v}</span>` : '<span style="color: var(--color-muted)">—</span>'
			},
			{
				key: "status",
				label: t("common.status", "Status"),
				sortable: true,
				render: (v: string) => {
					const colors = v === "pending" || v === "pending_review" ? "bg-warning/15 text-warning dark:text-warning" : v === "resolved" || v === "published" || v === "approved" ? "bg-success/15 text-success dark:text-success" : v === "dismissed" || v === "removed" || v === "rejected" ? "bg-danger/15 text-danger dark:text-danger" : v === "flagged" ? "bg-warning/15 text-warning dark:text-warning" : "bg-info/15 text-info dark:text-info";
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
				}
			},
			{
				key: "reporter",
				label: t("moderation.reporter", "Reporter"),
				render: (v: string, row: ModItem) => v || row.author_id || "—"
			},
			{
				key: "created_at",
				label: t("common.created", "Created"),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString()
			}
		],
		filters,
		actions: rowActions,
		bulkActions: canModerate
			? [
					{
						label: t("moderation.approve", "Approve"),
						icon: Check,
						onclick: (ids: Set<string>) => bulkModerate(ids, "approve")
					},
					{
						label: t("moderation.reject", "Reject"),
						icon: X,
						variant: "destructive",
						onclick: (ids: Set<string>) => bulkModerate(ids, "reject")
					},
					{
						label: t("moderation.bulk_assign", "Assign Selected to Me"),
						icon: UserPlus,
						onclick: bulkAssignToMe,
						disabled: (_ids: Set<string>, selectedRows?: ModItem[]) =>
							!(selectedRows ?? []).some(isAssignable)
					}
				]
			: undefined,
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set("q", q);
			if (filter.status) params.set("status", filter.status);
			if (filter.content_type) params.set("content_type", filter.content_type);
			const result = await moderationAdminApi.getQueue(params.toString());
			if (result.ok) {
				const d = result.data as { items: ModItem[]; total?: number; page: number; total_pages: number } | null;
				return {
					data: d?.items ?? [],
					total: d?.total ?? (d?.items ?? []).length,
					totalPages: d?.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t("common.error", "Failed to load moderation queue"));
		},
		viewSnippet: ModItemView
	});
</script>

<svelte:head>
	<title>{t("moderationQueue.title", "Moderation Queue")} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── State-action confirmation modal (approve/reject/delete with optional reason) ── -->
<Modal
	bind:open={showActionModal}
	title={actionType === "approve"
		? t("moderation.approve_title", "Approve Content")
		: actionType === "reject"
			? t("moderation.reject_title", "Reject Content")
			: t("moderation.delete_title", "Delete Content")}
>
	{#if actionTarget}
		<div class="mb-3 rounded-lg p-3 text-sm" style="background: var(--color-surface); border: 1px solid var(--color-border)">
			<p class="font-medium" style="color: var(--color-foreground)">{actionTarget.type}</p>
			<p style="color: var(--color-muted)">{actionTarget.content || "—"}</p>
		</div>
	{/if}
	{#if actionType !== "approve"}
		<textarea
			class="w-full rounded-lg border p-3 text-sm"
			style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			rows="3"
			placeholder={t("moderation.action_reason", "Reason (optional)...")}
			bind:value={actionReason}
		></textarea>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showActionModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant={actionType === "approve" ? "default" : "destructive"} onclick={executeAction} disabled={acting}>
			{acting
				? t("app.moderation.queue.processing", "Processing...")
				: actionType === "approve"
					? t("moderation.approve", "Approve")
					: actionType === "reject"
						? t("moderation.reject", "Reject")
						: t("common.delete", "Delete")}
		</Button>
	</div>
</Modal>

<!-- ── View modal body (mirrors the legacy view modal) ──────────────── -->
{#snippet ModItemView({ row }: { row: ModItem })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("moderation.content_type", "Content Type")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.status", "Status")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			{#if row.entity_ref}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("moderationQueue.content_id", "Content ID")}</p>
					<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.entity_ref}</p>
				</div>
			{/if}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("moderation.reporter", "Reporter")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.reporter || row.author_id || "—"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.created", "Created")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
			{#if row.content}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("moderation.content_preview", "Content Preview")}</p>
					<p class="mt-1 rounded-lg p-3 text-sm" style="background: var(--color-surface); color: var(--color-foreground)">{row.content}</p>
				</div>
			{/if}
		</div>
	</div>
{/snippet}
