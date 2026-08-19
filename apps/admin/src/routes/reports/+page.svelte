<script lang="ts">
	// /reports — migrated to the EntityPage scaffold.
	// Read-only list (reports are user-submitted) with Action/Dismiss state actions
	// and Delete. Action opens a page-local Modal to set status to 'actioned' with a
	// resolution note; Dismiss PATCHes status to 'dismissed'. Both call entityController.reload().
	// Ref: japaconnect-engineering/sections/51-content-reports-flagging.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { reportsAdminApiExtended, apiRaw, adminCatalogApi, type CatalogOption } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { Eye, Gavel, Ban, Trash2 } from "lucide-svelte";
	import type { ActionItem } from "$lib/components/ui/data-table";

	type Report = {
		id: string;
		reporter_user_id: string;
		reporter_username?: string;
		reporter_display_name?: string;
		entity_type: string;
		entity_id: string;
		reason: string;
		notes?: string;
		evidence_urls?: string[];
		status: string;
		reviewed_by?: string;
		resolution_note?: string;
		resolved_at?: string;
		created_at: string;
	};

	function statusBadgeClass(status: string): string {
		switch (status) {
			case "pending": return "bg-warning/15 text-warning dark:text-warning";
			case "reviewing": return "bg-info/15 text-info dark:text-info";
			case "actioned": return "bg-success/15 text-success dark:text-success";
			case "dismissed": return "bg-muted/15 text-muted dark:text-muted";
			default: return "bg-muted/15 text-muted dark:text-muted";
		}
	}

	// ── EntityPage controller + page error ──────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);

	let reportStatusOptions = $state<CatalogOption[]>([
		{ value: "pending", label: t("reports.status_pending", "Pending") },
		{ value: "reviewing", label: t("reports.status_reviewing", "Reviewing") },
		{ value: "actioned", label: t("reports.status_actioned", "Actioned") },
		{ value: "dismissed", label: t("reports.status_dismissed", "Dismissed") }
	]);

	let reportEntityTypeOptions = $state<CatalogOption[]>([
		{ value: "content", label: "Content" },
		{ value: "comment", label: "Comment" },
		{ value: "user", label: "User" },
		{ value: "space", label: "Space" },
		{ value: "message", label: "Message" }
	]);

	$effect(() => {
		void (async () => {
			const [statusRes, typeRes] = await Promise.all([
				adminCatalogApi.get("reports.status"),
				adminCatalogApi.get("reports.entity_type")
			]);
			if (statusRes.ok && statusRes.data?.length) reportStatusOptions = statusRes.data;
			if (typeRes.ok && typeRes.data?.length) reportEntityTypeOptions = typeRes.data;
		})();
	});

	let pageError = $state("");

	// ── Bulk dismiss (PATCH all selected to 'dismissed') with in-flight guard ──
	let bulkDismissing = $state(false);

	async function bulkDismiss(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDismissing) return; // in-flight guard
		bulkDismissing = true;
		pageError = "";
		try {
			await Promise.all(
				idArr.map((id) => reportsAdminApiExtended.update(id, { action: "dismiss", reason: "" }))
			);
			// EntityPage auto-reloads + clears selection after this returns.
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("reports.error", "Failed to dismiss reports");
		} finally {
			bulkDismissing = false;
		}
	}

	// ── State-action: Action (set status to 'actioned' with resolution note) ──
	let showActionModal = $state(false);
	let actionTarget = $state<Report | null>(null);
	let resolutionNote = $state("");
	let acting = $state(false);

	function openAction(report: Report) {
		actionTarget = report;
		resolutionNote = "";
		showActionModal = true;
	}

	async function confirmAction() {
		if (!actionTarget) return;
		acting = true;
		pageError = "";
		try {
			const result = await reportsAdminApiExtended.update(actionTarget.id, {
				action: "remove",
				reason: resolutionNote.trim() || ""
			});
			if (result.ok) {
				showActionModal = false;
				actionTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("reports.error", "Failed to action report");
			}
		} catch {
			pageError = t("reports.error", "Failed to action report");
		} finally {
			acting = false;
		}
	}

	// ── State-action: Dismiss (PATCH status to 'dismissed') ──────────────────
	async function dismissReport(report: Report) {
		pageError = "";
		try {
			const result = await reportsAdminApiExtended.update(report.id, { action: "dismiss", reason: "" });
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t("reports.error", "Failed to dismiss report");
			}
		} catch {
			pageError = t("reports.error", "Failed to dismiss report");
		}
	}

	// ── Row actions: View / Action / Dismiss / Delete ───────────────────────
	// Custom actions replace defaults, so View/Delete are included explicitly.
	const rowActions = $derived.by<ActionItem[]>(() => [
		{ label: t("common.view", "View"), icon: Eye, onclick: (row: Report, ctx) => ctx?.openView(row) },
		{
			label: t("reports.action", "Action"),
			icon: Gavel,
			onclick: (row: Report) => openAction(row),
			show: (row: Report) => row.status === "pending" || row.status === "reviewing"
		},
		{
			label: t("reports.dismiss", "Dismiss"),
			icon: Ban,
			onclick: (row: Report) => dismissReport(row),
			show: (row: Report) => row.status === "pending" || row.status === "reviewing"
		},
		{
			label: t("common.delete", "Delete"),
			icon: Trash2,
			variant: "destructive",
			onclick: (row: Report, ctx) => ctx?.openDelete(row)
		}
	]);

	// `fields` is required by EntityPage even for read-only entities (used for edit-form seeding).
	const fields: FieldDef[] = [
		{ key: "status", label: t("reports.status", "Status"), type: "text" },
		{ key: "resolution_note", label: t("reports.resolution", "Resolution"), type: "textarea" }
	];

	const config = $derived<EntityPageConfig>({
		title: t("reports.title", "Reports"),
		description: t("reports.description", "View and manage reported content"),
		readonly: true,
		deletePermission: "reports.review",
		pageSize: 25,
		columns: [
			{
				key: "entity_type",
				label: t("reports.target", "Target"),
				sortable: true,
				render: (_: any, row: Report) =>
					`<span class="text-xs" style="color: var(--color-muted)">${row.entity_type} &middot; ${(row.entity_id ?? "").slice(0, 10)}</span>`
			},
			{
				key: "reason",
				label: t("reports.reason", "Reason"),
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || "—"}</span>`
			},
			{
				key: "reporter_display_name",
				label: t("reports.reporter", "Reporter"),
				sortable: true,
				render: (v: string, row: Report) =>
					`<span class="text-sm">${v || row.reporter_username || "anonymous"}</span>`
			},
			{
				key: "status",
				label: t("reports.status", "Status"),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusBadgeClass(v)}">${v}</span>`
			},
			{
				key: "created_at",
				label: t("reports.created", "Created"),
				sortable: true,
				render: (v: string) => {
					if (!v) return `<span class="text-xs" style="color: var(--color-muted)">—</span>`;
					const d = new Date(v);
					return isNaN(d.getTime())
						? `<span class="text-xs" style="color: var(--color-muted)">—</span>`
						: `<span class="text-xs" style="color: var(--color-muted)">${d.toLocaleString()}</span>`;
				}
			}
		],
		filters: [
			{
				key: "status",
				label: t("reports.status", "Status"),
				options: reportStatusOptions
			},
			{
				key: "entity_type",
				label: t("reports.type", "Type"),
				options: reportEntityTypeOptions
			}
		],
		actions: rowActions,
		bulkActions: [
			{
				label: t("reports.bulk_dismiss", "Dismiss Selected"),
				icon: Ban,
				onclick: bulkDismiss
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			// Backend ListReports reads `page` (0-indexed) and computes offset
			// internally as page * page_size. EntityPage passes a 1-based page,
			// so convert to 0-based.
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page: String(Math.max(0, page - 1))
			});
			if (q) params.set("q", q);
			if (filter.status) params.set("status", filter.status);
			if (filter.entity_type) params.set("entity_type", filter.entity_type);
			// Use apiRaw to access the full response body. The backend returns
			// { data: { items: [...], total, total_pages } }.
			const result = await apiRaw<{ data?: { items?: Report[]; total?: number; total_pages?: number } }>(`/admin/reports?${params.toString()}`);
			if (result.ok) {
				const d = result.data;
				const items = d.data?.items ?? [];
				return {
					data: items,
					total: d.data?.total ?? items.length,
					totalPages: d.data?.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t("reports.error", "Failed to load reports"));
		},
		remove: async (id) => {
			const res = await reportsAdminApiExtended.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ReportView
	});
</script>

<svelte:head>
	<title>{t("reports.title", "Reports")} — JapaConnect Admin</title>
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

<!-- ── State-action: Action modal (set status to 'actioned' with resolution note) ── -->
<Modal bind:open={showActionModal} title={t("reports.action_report", "Action Report")}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("reports.action_confirm", "Action report")} <strong>{actionTarget.entity_type} &middot; {actionTarget.entity_id}</strong>?
		</p>
		<p class="mt-2 text-xs" style="color: var(--color-muted)">{actionTarget.reason || "—"}</p>
		<div class="mt-4">
			<label class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)" for="resolution-note">
				{t("reports.resolution", "Resolution")}
			</label>
			<textarea
				id="resolution-note"
				class="mt-1 w-full rounded-md border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
				bind:value={resolutionNote}
				placeholder={t("reports.resolution_placeholder", "Add a resolution note (optional)")}
				rows={3}
			></textarea>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showActionModal = false)}>{t("reports.cancel", "Cancel")}</Button>
		<Button onclick={confirmAction} disabled={acting}>
			{acting ? t("reports.actioning", "Actioning...") : t("reports.action", "Action")}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View slide-over body (full report detail) -->
{#snippet ReportView({ row }: { row: Report })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.reporter", "Reporter")}</p>
				<p class="mt-1 text-sm font-medium font-mono" style="color: var(--color-foreground)">{row.reporter_user_id}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.target", "Target")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.entity_type} &middot; {row.entity_id}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.reason", "Reason")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.reason || "—"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.status", "Status")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.created", "Created")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : "—"}</p>
			</div>
			{#if row.resolved_at}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.resolved_at", "Resolved")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.resolved_at).toLocaleString()}</p>
				</div>
			{/if}
		</div>
		{#if row.notes}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.notes", "Notes")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.notes}</p>
			</div>
		{/if}
		{#if row.resolution_note}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.resolution", "Resolution")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.resolution_note}</p>
			</div>
		{/if}
		{#if row.evidence_urls && row.evidence_urls.length > 0}
			<div class="pt-4 border-t" style="border-color: var(--color-border)">
				<p class="text-xs font-medium uppercase tracking-wider mb-2" style="color: var(--color-muted)">{t("reports.evidence", "Evidence")}</p>
				<ul class="space-y-1">
					{#each row.evidence_urls as url (url)}
						<li><a href={url} target="_blank" rel="noopener" class="text-sm text-primary underline-offset-4 hover:underline">{url}</a></li>
					{/each}
				</ul>
			</div>
		{/if}
	</div>
{/snippet}
