<script lang="ts">
	// /content — migrated to the EntityPage scaffold; read-only moderation list
	// (content is created by users, not from this page) with Remove/Restore/Feature
	// state actions driven by a custom `actions` array + page-local confirm Modals.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { contentAdminApi, apiRaw } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { onMount } from "svelte";
	import { Eye, Trash2, RotateCcw, Star } from "lucide-svelte";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";

	type ContentItem = {
		id: string;
		title: string;
		content_type: string;
		status: string;
		author_id?: string;
		space_id?: string;
		author_username?: string;
		author_display_name?: string;
		space_name?: string;
		created_at: string;
		removed_at?: string;
		view_count?: number;
		url?: string;
		slug?: string;
		body?: string;
		is_featured?: boolean;
	};

	// Backend gates content mutations on content.moderate (remove/restore) and
	// content.edit (feature/unfeature metadata). Content types are admin-configurable
	// (Section 14) so the Type filter options are fetched from GET /content-types.
	const canModerate = $derived(hasPermission("content.moderate"));
	const canEditContent = $derived(hasPermission("content.edit"));

	// ── Filter options ──────────────────────────────────────────────────────
	// Status values are a fixed enum from the contents table CHECK constraint
	// (draft, in_review, scheduled, published, archived, removed) — schema-defined.
	const statusOptions = [
		{ value: "published", label: t("content.status.published", "Published") },
		{ value: "draft", label: t("content.status.draft", "Draft") },
		{ value: "in_review", label: t("content.status.in_review", "In Review") },
		{ value: "scheduled", label: t("content.status.scheduled", "Scheduled") },
		{ value: "archived", label: t("content.status.archived", "Archived") },
		{ value: "removed", label: t("content.status.removed", "Removed") }
	];

	// Content types are admin-configurable — fetch from GET /content-types (Rule 16).
	let contentTypeOptions = $state<{ value: string; label: string }[]>([]);
	let filterDefs = $state<FilterDef[]>([
		{ key: "content_type", label: t("content.type", "Type"), options: [] },
		{ key: "status", label: t("common.status", "Status"), options: statusOptions }
	]);

	onMount(async () => {
		try {
			const res = await contentAdminApi.listContentTypes();
			if (res.ok) {
				contentTypeOptions = (res.data as { value: string; label: string }[]).map((ct) => ({
					value: ct.value,
					label: ct.label
				}));
				filterDefs = filterDefs.map((def) =>
					def.key === "content_type" ? { ...def, options: contentTypeOptions } : def
				);
			}
		} catch {
			// Keep empty fallback on failure.
		}
	});

	// ── Bulk remove ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let bulkRemoving = $state(false);

	async function bulkRemove(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkRemoving) return; // in-flight guard
		bulkRemoving = true;
		pageError = "";
		try {
			await Promise.all(idArr.map((id) => contentAdminApi.remove(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to remove content");
		} finally {
			bulkRemoving = false;
		}
	}

	// ── State-action confirmation modals ────────────────────────────────────
	// EntityPage's confirm Modal only covers default delete; state actions keep
	// page-local Modals with richer copy, confirm calls API + controller.reload().
	let acting = $state(false);
	let actionTarget = $state<ContentItem | null>(null);

	let showRemoveModal = $state(false);
	let removeReason = $state("");
	let showRestoreModal = $state(false);

	function openRemove(item: ContentItem) {
		actionTarget = item;
		removeReason = "";
		showRemoveModal = true;
	}

	function openRestore(item: ContentItem) {
		actionTarget = item;
		showRestoreModal = true;
	}

	async function confirmRemove() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await contentAdminApi.remove(actionTarget.id, { reason: removeReason || undefined });
			if (result.ok) {
				showRemoveModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to remove content");
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to remove content");
		} finally {
			acting = false;
		}
	}

	async function confirmRestore() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await contentAdminApi.restore(actionTarget.id);
			if (result.ok) {
				showRestoreModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to restore content");
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to restore content");
		} finally {
			acting = false;
		}
	}

	async function toggleFeature(item: ContentItem) {
		acting = true;
		try {
			const isFeatured = item.is_featured === true;
			const result = isFeatured
				? await contentAdminApi.unfeature(item.id)
				: await contentAdminApi.feature(item.id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to toggle feature");
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to toggle feature");
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	// Custom actions replace defaults, so View is included explicitly. Remove/Restore
	// are gated by content.moderate; Feature/Unfeature by content.edit (blog content only).
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t("common.view", "View"), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canEditContent) {
			acts.push({
				label: t("content.feature", "Feature"),
				icon: Star,
				onclick: (row: ContentItem) => toggleFeature(row),
				show: (row: ContentItem) => row.content_type === "blog_post" && row.is_featured !== true
			});
			acts.push({
				label: t("content.unfeature", "Unfeature"),
				icon: Star,
				onclick: (row: ContentItem) => toggleFeature(row),
				show: (row: ContentItem) => row.content_type === "blog_post" && row.is_featured === true
			});
		}
		if (canModerate) {
			acts.push({
				label: t("content.remove", "Remove"),
				icon: Trash2,
				variant: "destructive",
				onclick: openRemove,
				show: (row: ContentItem) => row.status !== "removed" && !row.removed_at
			});
			acts.push({
				label: t("content.restore", "Restore"),
				icon: RotateCcw,
				onclick: openRestore,
				show: (row: ContentItem) => row.status === "removed" || !!row.removed_at
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig even on read-only pages (used to seed
	// the edit form); this page is read-only so the form is never opened.
	const fields: FieldDef[] = [
		{ key: "title", label: t("content.title", "Title"), type: "text" },
		{ key: "slug", label: t("content.slug", "Slug"), type: "text" },
		{ key: "status", label: t("common.status", "Status"), type: "text" },
		{ key: "body", label: t("content.body", "Body"), type: "textarea" }
	];

	// Config built as a $derived so filterDefs updates (loaded on mount) propagate.
	const config = $derived<EntityPageConfig>({
		title: t("content.all", "All Content"),
		description: t("content.manage", "Manage all platform content"),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: "title", label: t("content.column.title", "Title"), sortable: true },
			{
				key: "content_type",
				label: t("content.type", "Type"),
				sortable: true,
				render: (v: string) => (v ? v.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase()) : "—")
			},
			{
				key: "author_display_name",
				label: t("content.author", "Author"),
				sortable: true,
				render: (v: string, row: ContentItem) => v || row.author_username || "anonymous"
			},
			{
				key: "space_name",
				label: t("content.space", "Space"),
				sortable: true,
				render: (v: string, row: ContentItem) =>
					v || (row.space_id ? row.space_id.substring(0, 8) + "..." : "—")
			},
			{
				key: "status",
				label: t("common.status", "Status"),
				sortable: true,
				render: (v: string) => {
					if (!v) return "—";
					const colors =
						v === "published"
							? "bg-success/15 text-success dark:text-success"
							: v === "removed"
								? "bg-danger/15 text-danger dark:text-danger"
								: v === "archived"
									? "bg-muted/30 text-muted dark:text-muted"
									: v === "in_review"
										? "bg-warning/15 text-warning dark:text-warning"
										: "bg-info/15 text-info dark:text-info";
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v.charAt(0).toUpperCase() + v.slice(1)}</span>`;
				}
			},
			{
				key: "view_count",
				label: t("content.views", "Views"),
				sortable: true,
				render: (v: number) => (v ?? 0).toLocaleString()
			},
			{
				key: "created_at",
				label: t("common.created", "Created"),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString()
			}
		],
		filters: filterDefs,
		actions: rowActions,
		bulkActions: canModerate
			? [
					{
						label: t("content.remove_selected", "Remove Selected"),
						icon: Trash2,
						variant: "destructive",
						onclick: bulkRemove
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
			if (filter.content_type) params.set("type", filter.content_type);
			if (filter.status) params.set("status", filter.status);
			// Use apiRaw to access top-level total/total_pages alongside data.
			const result = await apiRaw<{ data?: ContentItem[]; items?: ContentItem[]; total?: number; total_pages?: number }>(`/admin/content?${params.toString()}`);
			if (result.ok) {
				const d = result.data;
				// API returns null when no items match the filter.
				const allItems = d?.data ?? d?.items ?? [];
				return {
					data: allItems as ContentItem[],
					total: d?.total ?? allItems.length,
					totalPages: d?.total_pages ?? Math.max(1, Math.ceil((d?.total ?? allItems.length) / pageSize))
				};
			}
			throw new Error(result.error ?? t("common.error", "Failed to load content"));
		},
		viewSnippet: ContentView
	});
</script>

<svelte:head>
	<title>{t("content.title", "Content")} — JapaConnect Admin</title>
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

<!-- ── State-action confirmation modals ─────────────────────────────────── -->

<!-- Remove (with optional reason) -->
<Modal bind:open={showRemoveModal} title={t("content.remove_title", "Remove Content")}>
	{#if actionTarget}
		<p class="mb-2 text-sm" style="color: var(--color-foreground)">
			{t("content.remove_confirm", "Remove this content?")} <strong>{actionTarget.title || "Untitled"}</strong>
		</p>
		<textarea
			class="w-full rounded-lg border p-3 text-sm"
			style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			rows="3"
			placeholder={t("content.remove_reason", "Reason (optional)...")}
			bind:value={removeReason}
		></textarea>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRemoveModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmRemove} disabled={acting}>{t("content.remove", "Remove")}</Button>
	</div>
</Modal>

<!-- Restore -->
<Modal bind:open={showRestoreModal} title={t("content.restore_title", "Restore Content")}>
	{#if actionTarget}
		<p class="mb-2 text-sm" style="color: var(--color-foreground)">
			{t("content.restore_confirm", "Restore this content?")} <strong>{actionTarget.title || "Untitled"}</strong>
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRestoreModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="default" onclick={confirmRestore} disabled={acting}>{t("content.restore", "Restore")}</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View slide-over body (mirrors the legacy view modal) -->
{#snippet ContentView({ row }: { row: ContentItem })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.column.title", "Title")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title || "Untitled"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.type", "Type")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.content_type ? row.content_type.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase()) : "—"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.author", "Author")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.author_display_name || row.author_username || (row.author_id ? row.author_id.substring(0, 8) + "..." : "—")}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.space", "Space")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.space_name || (row.space_id ? row.space_id.substring(0, 8) + "..." : "—")}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.status", "Status")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status ? row.status.charAt(0).toUpperCase() + row.status.slice(1) : "—"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.views", "Views")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(row.view_count ?? 0).toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.created", "Created")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
			{#if row.removed_at}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.removed_at", "Removed")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-danger)">{new Date(row.removed_at).toLocaleString()}</p>
				</div>
			{/if}
			{#if row.slug}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.slug", "Slug")}</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
				</div>
			{/if}
			{#if row.body}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.body", "Body")}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.body.substring(0, 200)}{row.body.length > 200 ? "..." : ""}</p>
				</div>
			{/if}
		</div>
	</div>
{/snippet}
