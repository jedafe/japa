<script lang="ts">
	// /moderation — migrated to the EntityPage scaffold; moderation queue with
	// state actions (approve/reject/delete) via a custom `actions` array and a
	// page-local confirm Modal (reason textarea), plus bulk approve/reject.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { moderationAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Eye, Check, X, Trash2 } from "lucide-svelte";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";

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
	let moderationOptions = $state<{ statuses: { value: string; label: string }[]; content_types: { value: string; label: string }[] }>({
		statuses: [],
		content_types: []
	});

	$effect(() => {
		void (async () => {
			const res = await moderationAdminApi.getOptions();
			if (res.ok && res.data) {
				moderationOptions = {
					statuses: res.data.statuses.map((s) => ({
						value: s.value,
						label: t(`app.moderation.queue.${s.value}`, s.label)
					})),
					content_types: res.data.content_types.map((c) => ({
						value: c.value,
						label: t(`app.moderation.queue.${c.value}`, c.label)
					}))
				};
			}
		})();
	});

	const filters = $derived<FilterDef[]>([
		{
			key: "status",
			label: t("common.status", "Status"),
			options: moderationOptions.statuses
		},
		{
			key: "content_type",
			label: t("moderation.content_type", "Content Type"),
			options: moderationOptions.content_types
		}
	]);

	// ── Bulk actions ────────────────────────────────────────────────────────
	// EntityPage auto-reloads + clears selection after bulk handlers complete.
	let pageError = $state("");
	let bulkActing = $state(false);

	async function bulkApprove(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkActing) return; // in-flight guard
		bulkActing = true;
		pageError = "";
		try {
			await moderationAdminApi.bulkAction({ ids: idArr, action: "approve" });
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Bulk action failed");
		} finally {
			bulkActing = false;
		}
	}

	async function bulkReject(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkActing) return; // in-flight guard
		bulkActing = true;
		pageError = "";
		try {
			await moderationAdminApi.bulkAction({ ids: idArr, action: "reject" });
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Bulk action failed");
		} finally {
			bulkActing = false;
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
				label: t("moderation.approve", "Approve"),
				icon: Check,
				onclick: (row: ModItem) => openAction(row, "approve"),
				show: (row: ModItem) => row.status === "pending"
			});
			acts.push({
				label: t("moderation.reject", "Reject"),
				icon: X,
				variant: "destructive",
				onclick: (row: ModItem) => openAction(row, "reject"),
				show: (row: ModItem) => row.status === "pending"
			});
			acts.push({
				label: t("common.delete", "Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: ModItem) => openAction(row, "delete")
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig but this page is readonly (no
	// create/edit form), so an empty array suffices.
	const fields: FieldDef[] = [];

	// Config built as a $derived so canModerate / rowActions updates propagate.
	const config = $derived<EntityPageConfig>({
		title: t("moderation.title", "Moderation"),
		description: t("moderation.description", "Review and moderate platform content"),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: "type",
				label: t("moderation.source", "Source"),
				sortable: true,
				render: (v: string) => {
					const colors = v === "ai_flag" ? "bg-info/15 text-info dark:text-info" : "bg-warning/15 text-warning dark:text-warning";
					const label = v === "ai_flag" ? t("app.moderation.queue.ai_flag", "AI Flag") : t("app.moderation.queue.user_report", "User Report");
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${label}</span>`;
				}
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
					const colors = v === "pending" ? "bg-warning/15 text-warning dark:text-warning" : v === "resolved" ? "bg-success/15 text-success dark:text-success" : v === "dismissed" ? "bg-muted/15 text-muted dark:text-muted" : v === "flagged" ? "bg-warning/15 text-warning dark:text-warning" : "bg-info/15 text-info dark:text-info";
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
				}
			},
			{
				key: "reporter",
				label: t("moderation.reporter", "Reporter"),
				render: (v: string) => v || "—"
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
						onclick: bulkApprove
					},
					{
						label: t("moderation.reject", "Reject"),
						icon: X,
						variant: "destructive",
						onclick: bulkReject
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
	<title>{t("moderation.title", "Moderation")} — JapaConnect Admin</title>
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

<!-- ── View slide-over body (mirrors the legacy view modal) ──────────────── -->
{#snippet ModItemView({ row }: { row: ModItem })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("moderation.source", "Source")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.status", "Status")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("moderation.reporter", "Reporter")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.reporter || "—"}</p>
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
