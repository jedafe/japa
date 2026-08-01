<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw, ArrowUp, ArrowDown, ArrowLeft } from "lucide-svelte";
	import { Select } from "$lib/components/ui/select";
	import { sidebarsAdminApi, userManagementApi, type UserGroup } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	// Ref: Section 59 (Sidebar Management System), ADR-066.
	// Sidebar widgets use a dedicated widget type registry (search, theme_toggle,
	// trending_tags, etc.) — NOT pagebuilder block types.

	type WidgetType = {
		type: string;
		name: string;
		description: string;
		icon: string;
		slot: string;
		category: string;
		default_props: Record<string, unknown>;
	};

	type SidebarWidget = {
		id: string;
		tenant_id: string;
		sidebar_id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
		visibility: string;
		required_group_ids: string[];
		permission_node?: string | null;
		status: string;
	};

	type Sidebar = {
		id: string;
		tenant_id: string;
		app: string;
		position: string;
		slug: string;
		name: string;
		page_pattern: string;
		visibility: string;
		required_group_ids: string[];
		status: string;
		created_at: string;
		updated_at: string;
		widgets?: SidebarWidget[];
	};

	let sidebars = $state<Sidebar[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");

	// Sidebar modal state
	let showSidebarModal = $state(false);
	let sidebarModalMode = $state<"create" | "edit">("create");
	let editingSidebar = $state<Sidebar | null>(null);
	let savingSidebar = $state(false);
	let sidebarName = $state("");
	let sidebarSlug = $state("");
	let sidebarApp = $state<string>("main");
	let sidebarPosition = $state<string>("right");
	let sidebarPagePattern = $state<string>("*");
	let sidebarVisibility = $state<string>("all");
	let sidebarStatus = $state<string>("active");
	let sidebarRequiredGroupIds = $state<Set<string>>(new Set());

	// Widget modal state
	let showWidgetModal = $state(false);
	let editingWidget = $state<SidebarWidget | null>(null);
	let savingWidget = $state(false);
	let widgetLabel = $state("");
	let widgetType = $state<string>("");
	let widgetProps = $state<string>("");
	let widgetVisibility = $state<string>("all");
	let widgetStatus = $state<string>("active");
	let widgetPermissionNode = $state("");
	let widgetRequiredGroupIds = $state<Set<string>>(new Set());

	// Widget types from the sidebar widget type registry
	let widgetTypes = $state<WidgetType[]>([]);

	// User groups for the required_group_ids multi-select
	let userGroups = $state<UserGroup[]>([]);
	let groupSearch = $state("");
	let groupDropdownOpen = $state(false);
	let groupDebounce: ReturnType<typeof setTimeout> | null = null;

	// Sidebar group multi-select state (separate from widget)
	let sidebarGroupSearch = $state("");
	let sidebarGroupDropdownOpen = $state(false);
	let sidebarGroupDebounce: ReturnType<typeof setTimeout> | null = null;

	// Selected sidebar for viewing widgets
	let selectedSidebar = $state<Sidebar | null>(null);

	let selectedSidebarIds = $state<Set<string>>(new Set());
	let selectedWidgetIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Delete confirmation state
	let deleteSidebarTarget = $state<Sidebar | null>(null);
	let showDeleteSidebarConfirm = $state(false);
	let deletingSidebar = $state(false);
	let deleteWidgetTarget = $state<string | null>(null);
	let showDeleteWidgetConfirm = $state(false);
	let deletingWidget = $state(false);

	const canManage = $derived(hasPermission("sidebars.manage"));

	const selectedGroupObjs = $derived(
		userGroups.filter((g) => widgetRequiredGroupIds.has(g.id))
	);
	const selectedSidebarGroupObjs = $derived(
		userGroups.filter((g) => sidebarRequiredGroupIds.has(g.id))
	);

	const groupSuggestions = $derived(
		groupSearch.trim()
			? userGroups.filter(
					(g) =>
						!widgetRequiredGroupIds.has(g.id) &&
						(g.name.toLowerCase().includes(groupSearch.toLowerCase()) ||
							g.slug.toLowerCase().includes(groupSearch.toLowerCase()))
				).slice(0, 10)
			: userGroups.filter((g) => !widgetRequiredGroupIds.has(g.id)).slice(0, 10)
	);

	const sidebarGroupSuggestions = $derived(
		sidebarGroupSearch.trim()
			? userGroups.filter(
					(g) =>
						!sidebarRequiredGroupIds.has(g.id) &&
						(g.name.toLowerCase().includes(sidebarGroupSearch.toLowerCase()) ||
							g.slug.toLowerCase().includes(sidebarGroupSearch.toLowerCase()))
				).slice(0, 10)
			: userGroups.filter((g) => !sidebarRequiredGroupIds.has(g.id)).slice(0, 10)
	);

	const sidebarColumns = $derived([
		{ key: "name", label: t("sidebars.name", "Name"), sortable: true },
		{ key: "app", label: t("sidebars.app", "App"), sortable: true,
			render: (v: string) => t(`sidebars.app_${v}`, v)
		},
		{ key: "position", label: t("sidebars.position", "Position"), sortable: true,
			render: (v: string) => t(`sidebars.position_${v}`, v)
		},
		{ key: "slug", label: t("sidebars.slug", "Slug"), sortable: true },
		{ key: "page_pattern", label: t("sidebars.page_pattern", "Page Pattern"), sortable: true },
		{ key: "visibility", label: t("sidebars.visibility", "Visibility"), sortable: true,
			render: (v: string) => t(`sidebars.visibility_${v}`, v)
		},
		{ key: "status", label: t("common.status", "Status"), sortable: true,
			render: (v: string) => t(`common.status_${v}`, v)
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-40",
			componentSnippet: SidebarActionsCell
		}
	]);

	const widgetColumns = $derived([
		{ key: "label", label: t("sidebars.widget_label", "Label"), sortable: true },
		{ key: "widget_type", label: t("sidebars.widget_type", "Widget Type"), sortable: true,
			render: (v: string) => {
				const wt = widgetTypes.find((w) => w.type === v);
				return wt ? wt.name : v.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
			}
		},
		{ key: "visibility", label: t("sidebars.visibility", "Visibility"), sortable: true,
			render: (v: string) => t(`sidebars.visibility_${v}`, v)
		},
		{ key: "sort_order", label: t("sidebars.sort_order", "Order"), sortable: true },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-48",
			componentSnippet: WidgetActionsCell
		}
	]);

	async function loadSidebars(searchQuery?: string) {
		loading = true;
		error = "";
		try {
			const params = searchQuery ? `q=${encodeURIComponent(searchQuery)}` : undefined;
			const result = await sidebarsAdminApi.list(params);
			if (result.ok) {
				sidebars = ((result.data as { items: Sidebar[] })?.items) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load sidebars");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load sidebars");
		} finally {
			loading = false;
		}
	}

	async function loadUserGroups() {
		try {
			const result = await userManagementApi.listGroups();
			if (result.ok) {
				userGroups = ((result.data as { groups: UserGroup[] })?.groups) ?? [];
			}
		} catch {
			userGroups = [];
		}
	}

	async function loadWidgetTypes() {
		try {
			const result = await sidebarsAdminApi.widgetTypes();
			if (result.ok) {
				widgetTypes = ((result.data as { widget_types: WidgetType[] })?.widget_types) ?? [];
			}
		} catch {
			widgetTypes = [];
		}
	}

	// ── Sidebar CRUD ──────────────────────────────────────────────────

	function openCreateSidebar() {
		sidebarModalMode = "create";
		editingSidebar = null;
		sidebarName = "";
		sidebarSlug = "";
		sidebarApp = "main";
		sidebarPosition = "right";
		sidebarPagePattern = "*";
		sidebarVisibility = "all";
		sidebarStatus = "active";
		sidebarRequiredGroupIds = new Set();
		sidebarGroupSearch = "";
		sidebarGroupDropdownOpen = false;
		showSidebarModal = true;
	}

	function openEditSidebar(sidebar: Sidebar | null) {
		if (!sidebar) return;
		sidebarModalMode = "edit";
		editingSidebar = sidebar;
		sidebarName = sidebar.name;
		sidebarSlug = sidebar.slug;
		sidebarApp = sidebar.app;
		sidebarPosition = sidebar.position;
		sidebarPagePattern = sidebar.page_pattern;
		sidebarVisibility = sidebar.visibility;
		sidebarStatus = sidebar.status;
		sidebarRequiredGroupIds = new Set(sidebar.required_group_ids ?? []);
		sidebarGroupSearch = "";
		sidebarGroupDropdownOpen = false;
		showSidebarModal = true;
	}

	async function saveSidebar() {
		if (!sidebarName.trim() || !sidebarSlug.trim()) return;
		savingSidebar = true;
		error = "";
		try {
			const body = {
				app: sidebarApp,
				position: sidebarPosition,
				slug: sidebarSlug.trim().toLowerCase().replace(/\s+/g, "-"),
				name: sidebarName.trim(),
				page_pattern: sidebarPagePattern.trim() || "*",
				visibility: sidebarVisibility,
				required_group_ids: [...sidebarRequiredGroupIds],
				status: sidebarStatus
			};
			const result = editingSidebar
				? await sidebarsAdminApi.update(editingSidebar.id, body)
				: await sidebarsAdminApi.create(body);
			if (result.ok) {
				showSidebarModal = false;
				success = editingSidebar ? t("sidebars.updated", "Sidebar updated") : t("sidebars.created", "Sidebar created");
				loadSidebars();
			} else {
				error = result.error ?? t("common.error", "Failed to save sidebar");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save sidebar");
		} finally {
			savingSidebar = false;
		}
	}

	function openDeleteSidebar(sidebar: Sidebar | null) {
		if (!sidebar) return;
		deleteSidebarTarget = sidebar;
		showDeleteSidebarConfirm = true;
	}

	async function confirmDeleteSidebar() {
		if (!deleteSidebarTarget || deletingSidebar) return;
		deletingSidebar = true;
		const deletedId = deleteSidebarTarget.id;
		const result = await sidebarsAdminApi.delete(deletedId);
		if (result.ok) {
			showDeleteSidebarConfirm = false;
			deleteSidebarTarget = null;
			success = t("sidebars.deleted", "Sidebar deleted");
			if (selectedSidebar?.id === deletedId) selectedSidebar = null;
			loadSidebars();
		} else {
			error = result.error ?? t("common.error", "Failed to delete sidebar");
		}
		deletingSidebar = false;
	}

	async function bulkDeleteSidebars() {
		if (selectedSidebarIds.size === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			const ids = [...selectedSidebarIds];
			await Promise.all(ids.map((id) => sidebarsAdminApi.delete(id)));
			success = t("sidebars.bulk_deleted", "Sidebars deleted");
			selectedSidebarIds = new Set();
			loadSidebars();
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to delete sidebars");
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Drill-down to widgets ───────────────────────────────────────────

	async function viewWidgets(sidebar: Sidebar) {
		loading = true;
		error = "";
		try {
			const result = await sidebarsAdminApi.get(sidebar.id);
			if (result.ok) {
				selectedSidebar = result.data as Sidebar;
			} else {
				selectedSidebar = sidebar;
				error = result.error ?? t("common.error", "Failed to load widgets");
			}
		} catch (e) {
			selectedSidebar = sidebar;
			error = e instanceof Error ? e.message : t("common.error", "Failed to load widgets");
		} finally {
			loading = false;
		}
	}

	function backToSidebars() {
		selectedSidebar = null;
		selectedWidgetIds = new Set();
	}

	// ── Widget CRUD ────────────────────────────────────────────────────

	function openCreateWidget() {
		if (!selectedSidebar) return;
		editingWidget = null;
		widgetLabel = "";
		widgetType = "";
		widgetProps = "{}";
		widgetVisibility = "all";
		widgetStatus = "active";
		widgetPermissionNode = "";
		widgetRequiredGroupIds = new Set();
		groupSearch = "";
		groupDropdownOpen = false;
		showWidgetModal = true;
	}

	function openEditWidget(widget: SidebarWidget) {
		editingWidget = widget;
		widgetLabel = widget.label;
		widgetType = widget.widget_type;
		widgetProps = JSON.stringify(widget.props ?? {}, null, 2);
		widgetVisibility = widget.visibility || "all";
		widgetStatus = widget.status || "active";
		widgetPermissionNode = widget.permission_node ?? "";
		widgetRequiredGroupIds = new Set(widget.required_group_ids ?? []);
		groupSearch = "";
		groupDropdownOpen = false;
		showWidgetModal = true;
	}

	async function saveWidget() {
		if (!widgetType.trim() || !selectedSidebar) return;
		savingWidget = true;
		error = "";
		try {
			let props: Record<string, unknown> = {};
			try {
				props = JSON.parse(widgetProps || "{}");
			} catch {
				error = t("sidebars.invalid_props_json", "Props must be valid JSON");
				savingWidget = false;
				return;
			}
			const body: Record<string, unknown> = {
				widget_type: widgetType.trim(),
				label: widgetLabel.trim(),
				props,
				sort_order: editingWidget?.sort_order ?? 0,
				visibility: widgetVisibility,
				required_group_ids: [...widgetRequiredGroupIds],
				permission_node: widgetPermissionNode.trim() || null,
				status: widgetStatus
			};
			const result = editingWidget
				? await sidebarsAdminApi.updateWidget(selectedSidebar.id, editingWidget.id, body)
				: await sidebarsAdminApi.addWidget(selectedSidebar.id, body);
			if (result.ok) {
				showWidgetModal = false;
				success = editingWidget ? t("sidebars.widget_updated", "Widget updated") : t("sidebars.widget_created", "Widget created");
				const refreshed = await sidebarsAdminApi.get(selectedSidebar.id);
				if (refreshed.ok) {
					selectedSidebar = refreshed.data as Sidebar;
				}
				await loadSidebars();
			} else {
				error = result.error ?? t("common.error", "Failed to save widget");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save widget");
		} finally {
			savingWidget = false;
		}
	}

	function openDeleteWidget(widgetId: string) {
		deleteWidgetTarget = widgetId;
		showDeleteWidgetConfirm = true;
	}

	async function confirmDeleteWidget() {
		if (!deleteWidgetTarget || !selectedSidebar || deletingWidget) return;
		deletingWidget = true;
		const result = await sidebarsAdminApi.deleteWidget(selectedSidebar.id, deleteWidgetTarget);
		if (result.ok) {
			showDeleteWidgetConfirm = false;
			deleteWidgetTarget = null;
			success = t("sidebars.widget_deleted", "Widget deleted");
			const refreshed = await sidebarsAdminApi.get(selectedSidebar.id);
			if (refreshed.ok) {
				selectedSidebar = refreshed.data as Sidebar;
			}
			await loadSidebars();
		} else {
			error = result.error ?? t("common.error", "Failed to delete widget");
		}
		deletingWidget = false;
	}

	async function moveWidget(widgetId: string, dir: -1 | 1) {
		if (!selectedSidebar) return;
		const widgets = [...(selectedSidebar.widgets ?? [])].sort((a, b) => a.sort_order - b.sort_order);
		const idx = widgets.findIndex((w) => w.id === widgetId);
		const swap = idx + dir;
		if (idx < 0 || swap < 0 || swap >= widgets.length) return;
		[widgets[idx], widgets[swap]] = [widgets[swap], widgets[idx]];
		const reorderItems = widgets.map((w, i) => ({ id: w.id, sort_order: i }));
		const result = await sidebarsAdminApi.reorderWidgets(selectedSidebar.id, { items: reorderItems });
		if (result.ok) {
			const refreshed = await sidebarsAdminApi.get(selectedSidebar.id);
			if (refreshed.ok) {
				selectedSidebar = refreshed.data as Sidebar;
			}
		} else {
			error = result.error ?? t("common.error", "Failed to reorder widgets");
		}
	}

	async function bulkDeleteWidgets() {
		if (!selectedSidebar || selectedWidgetIds.size === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			const ids = [...selectedWidgetIds];
			await Promise.all(ids.map((id) => sidebarsAdminApi.deleteWidget(selectedSidebar!.id, id)));
			success = t("sidebars.widgets_deleted", "Widgets deleted");
			selectedWidgetIds = new Set();
			const refreshed = await sidebarsAdminApi.get(selectedSidebar!.id);
			if (refreshed.ok) {
				selectedSidebar = refreshed.data as Sidebar;
			}
			await loadSidebars();
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to delete widgets");
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Group multi-select handlers ───────────────────────────────────

	function handleGroupInput(e: Event) {
		const target = e.target as HTMLInputElement;
		groupSearch = target.value;
		if (groupDebounce) clearTimeout(groupDebounce);
		groupDebounce = setTimeout(() => { groupDropdownOpen = true; }, 200);
	}

	function addGroup(groupId: string) {
		const next = new Set(widgetRequiredGroupIds);
		next.add(groupId);
		widgetRequiredGroupIds = next;
		groupSearch = "";
	}

	function removeGroup(groupId: string) {
		const next = new Set(widgetRequiredGroupIds);
		next.delete(groupId);
		widgetRequiredGroupIds = next;
	}

	function handleSidebarGroupInput(e: Event) {
		const target = e.target as HTMLInputElement;
		sidebarGroupSearch = target.value;
		if (sidebarGroupDebounce) clearTimeout(sidebarGroupDebounce);
		sidebarGroupDebounce = setTimeout(() => { sidebarGroupDropdownOpen = true; }, 200);
	}

	function addSidebarGroup(groupId: string) {
		const next = new Set(sidebarRequiredGroupIds);
		next.add(groupId);
		sidebarRequiredGroupIds = next;
		sidebarGroupSearch = "";
	}

	function removeSidebarGroup(groupId: string) {
		const next = new Set(sidebarRequiredGroupIds);
		next.delete(groupId);
		sidebarRequiredGroupIds = next;
	}

	function onWidgetTypeChange(val: string) {
		widgetType = val;
		const wt = widgetTypes.find((w) => w.type === val);
		if (wt && !editingWidget) {
			widgetProps = JSON.stringify(wt.default_props ?? {}, null, 2);
		}
	}

	// ── Init ─────────────────────────────────────────────────────────

	loadSidebars();
	loadUserGroups();
	loadWidgetTypes();
</script>

<svelte:head>
	<title>{t("sidebars.title", "Sidebar Manager")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("sidebars.title", "Sidebar Manager")}
	description={t("sidebars.description", "Manage sidebars across Main, Docs, and Learn apps with per-page and per-usergroup visibility")}
	actions={canManage ? [
		...(selectedSidebar
			? [{ label: t("sidebars.add_widget", "Add Widget"), icon: Plus, onclick: openCreateWidget }]
			: [{ label: t("sidebars.create", "Create Sidebar"), icon: Plus, onclick: openCreateSidebar }]
		),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, onclick: () => (selectedSidebar ? viewWidgets(selectedSidebar) : loadSidebars()) }
	] : []}
/>

{#if !canManage}
	<div class="rounded-lg p-4 text-sm" style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)">
		{t("sidebars.no_permission", "You do not have permission to manage sidebars.")}
	</div>
{:else}
	{#if error}
		<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}
	{#if success}
		<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
			{success}
		</div>
	{/if}

	{#if !selectedSidebar}
		<Card>
			<CardContent>
				{#if loading}
					<TableSkeleton rows={6} columns={sidebarColumns.length} />
				{:else if sidebars.length === 0}
					<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
						{t("sidebars.empty", "No sidebars found. Create one to get started.")}
					</div>
				{:else}
					<DataTable columns={sidebarColumns} data={sidebars} pageSize={10} searchable onsearch={(q) => loadSidebars(q)} selectable bind:selectedIds={selectedSidebarIds}>
						{#snippet bulkActions()}
							<Button variant="destructive" size="sm" onclick={bulkDeleteSidebars} disabled={bulkDeleting || selectedSidebarIds.size === 0}>
								<Trash2 class="h-3.5 w-3.5 mr-1" />
								{bulkDeleting ? t("common.deleting", "Deleting...") : t("common.deleteSelected", "Delete Selected")} ({selectedSidebarIds.size})
							</Button>
						{/snippet}
					</DataTable>
				{/if}
			</CardContent>
		</Card>
	{:else}
		<div class="mb-4">
			<Button variant="ghost" size="sm" onclick={backToSidebars}>
				<ArrowLeft class="h-3.5 w-3.5 mr-1" />
				{t("sidebars.back_to_sidebars", "Back to Sidebars")}
			</Button>
		</div>
		<Card>
			<CardHeader>
				<h3 class="text-lg font-medium" style="color: var(--color-foreground)">
					{selectedSidebar.name} — {t("sidebars.widgets", "Widgets")}
					<span class="ml-2 text-sm font-normal" style="color: var(--color-muted)">
						({t("sidebars.app_" + selectedSidebar.app, selectedSidebar.app)} · {t("sidebars.position_" + selectedSidebar.position, selectedSidebar.position)} · {selectedSidebar.page_pattern})
					</span>
				</h3>
			</CardHeader>
			<CardContent>
				{#if loading}
					<TableSkeleton rows={6} columns={widgetColumns.length} />
				{:else if (selectedSidebar.widgets ?? []).length === 0}
					<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
						{t("sidebars.no_widgets", "No widgets in this sidebar. Add one to get started.")}
					</div>
				{:else}
					<DataTable columns={widgetColumns} data={[...(selectedSidebar.widgets ?? [])].sort((a, b) => a.sort_order - b.sort_order)} pageSize={(selectedSidebar.widgets ?? []).length} selectable bind:selectedIds={selectedWidgetIds}>
						{#snippet bulkActions()}
							<Button variant="destructive" size="sm" onclick={bulkDeleteWidgets} disabled={bulkDeleting || selectedWidgetIds.size === 0}>
								<Trash2 class="h-3.5 w-3.5 mr-1" />
								{bulkDeleting ? t("common.deleting", "Deleting...") : t("common.deleteSelected", "Delete Selected")} ({selectedWidgetIds.size})
							</Button>
						{/snippet}
					</DataTable>
				{/if}
			</CardContent>
		</Card>
	{/if}
{/if}

<!-- ── Sidebar Create/Edit Modal ──────────────────────────────────── -->
<Modal bind:open={showSidebarModal} title={sidebarModalMode === "create" ? t("sidebars.create", "Create Sidebar") : t("sidebars.edit", "Edit Sidebar")}>
	<div class="space-y-4">
		<div>
			<label for="sidebar-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.name", "Name")}</label>
			<input id="sidebar-name" type="text" bind:value={sidebarName} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="e.g., Dashboard Right" />
		</div>
		<div>
			<label for="sidebar-slug" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.slug", "Slug")}</label>
			<input id="sidebar-slug" type="text" bind:value={sidebarSlug} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="dashboard-right" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="sidebar-app" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.app", "App")}</label>
				<Select
					options={[
						{ value: "main", label: t("sidebars.app_main", "Main") },
						{ value: "docs", label: t("sidebars.app_docs", "Docs") },
						{ value: "learn", label: t("sidebars.app_learn", "Learn") }
					]}
					value={sidebarApp}
					onchange={(val) => (sidebarApp = val)}
				/>
			</div>
			<div>
				<label for="sidebar-position" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.position", "Position")}</label>
				<Select
					options={[
						{ value: "left", label: t("sidebars.position_left", "Left") },
						{ value: "right", label: t("sidebars.position_right", "Right") }
					]}
					value={sidebarPosition}
					onchange={(val) => (sidebarPosition = val)}
				/>
			</div>
		</div>
		<div>
			<label for="sidebar-pattern" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.page_pattern", "Page Pattern")}</label>
			<input id="sidebar-pattern" type="text" bind:value={sidebarPagePattern} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="*" />
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t("sidebars.page_pattern_hint", "Glob pattern. * matches all pages. /dashboard matches only that page. /u/* matches all profile pages.")}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="sidebar-visibility" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.visibility", "Visibility")}</label>
				<Select
					options={[
						{ value: "all", label: t("sidebars.visibility_all", "All") },
						{ value: "authenticated", label: t("sidebars.visibility_authenticated", "Authenticated") },
						{ value: "guest", label: t("sidebars.visibility_guest", "Guest") }
					]}
					value={sidebarVisibility}
					onchange={(val) => (sidebarVisibility = val)}
				/>
			</div>
			<div>
				<label for="sidebar-status" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("common.status", "Status")}</label>
				<Select
					options={[
						{ value: "active", label: t("common.status_active", "Active") },
						{ value: "inactive", label: t("common.status_inactive", "Inactive") }
					]}
					value={sidebarStatus}
					onchange={(val) => (sidebarStatus = val)}
				/>
			</div>
		</div>
		<div>
			<label for="sidebar-required-groups" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.required_groups", "Required User Groups")}</label>
			<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{t("sidebars.required_groups_hint", "Sidebar is visible only to users in at least one selected group. Leave empty for everyone.")}</p>
			<div class="relative mt-1">
				<div class="flex flex-wrap items-center gap-1 w-full rounded-lg border p-1.5 min-h-[2.5rem]" style="border-color: var(--color-border); background: var(--color-surface)">
					{#each selectedSidebarGroupObjs as group (group.id)}
						<span class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-foreground)">
							{group.name}
							<button type="button" class="ml-0.5 opacity-60 hover:opacity-100" style="color: var(--color-foreground)" onclick={() => removeSidebarGroup(group.id)} aria-label={t("common.remove", "Remove")}>&times;</button>
						</span>
					{/each}
					<input
						id="sidebar-required-groups"
						type="text"
						value={sidebarGroupSearch}
						oninput={handleSidebarGroupInput}
						onfocus={() => { sidebarGroupDropdownOpen = true; }}
						onblur={() => setTimeout(() => (sidebarGroupDropdownOpen = false), 200)}
						onkeydown={(e) => {
							if (e.key === "Escape") sidebarGroupDropdownOpen = false;
							else if (e.key === "Enter" && sidebarGroupSuggestions.length > 0) { e.preventDefault(); addSidebarGroup(sidebarGroupSuggestions[0].id); }
							else if (e.key === "Backspace" && !sidebarGroupSearch && selectedSidebarGroupObjs.length > 0) { removeSidebarGroup(selectedSidebarGroupObjs[selectedSidebarGroupObjs.length - 1].id); }
						}}
						class="flex-1 min-w-[8rem] bg-transparent border-0 outline-none text-sm py-1 px-1"
						style="color: var(--color-foreground)"
						placeholder={sidebarRequiredGroupIds.size === 0 ? t("sidebars.search_groups", "Search groups...") : ""}
					/>
				</div>
				{#if sidebarGroupDropdownOpen && sidebarGroupSuggestions.length > 0}
					<ul class="absolute z-[60] mt-1 max-h-48 w-full overflow-auto rounded-lg border shadow-lg" style="border-color: var(--color-border); background: var(--color-surface)">
						{#each sidebarGroupSuggestions as group (group.id)}
							<li>
								<button type="button" class="w-full px-3 py-2 text-left text-sm hover:bg-black/5 dark:hover:bg-white/10 flex items-center gap-2" style="color: var(--color-foreground)" onmousedown={(e) => { e.preventDefault(); addSidebarGroup(group.id); }}>
									<span>{group.name}</span>
									{#if group.is_system ?? false}<span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--color-muted); color: var(--color-surface)">system</span>{/if}
									<span class="ml-auto text-xs" style="color: var(--color-muted)">{group.member_count ?? 0}</span>
								</button>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showSidebarModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveSidebar} disabled={savingSidebar || !sidebarName.trim() || !sidebarSlug.trim()}>
			{savingSidebar ? t("common.saving", "Saving...") : t("common.save", "Save")}
		</Button>
	</div>
</Modal>

<!-- ── Widget Create/Edit Modal ─────────────────────────────────────── -->
<Modal bind:open={showWidgetModal} title={editingWidget ? t("sidebars.edit_widget", "Edit Widget") : t("sidebars.add_widget", "Add Widget")}>
	<div class="space-y-4">
		<div>
			<label for="widget-label" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.widget_label", "Label")}</label>
			<input id="widget-label" type="text" bind:value={widgetLabel} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="e.g., Trending Tags" />
		</div>
		<div>
			<label for="widget-type" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.widget_type", "Widget Type")}</label>
			<Select
				options={widgetTypes.map((wt) => ({ value: wt.type, label: wt.name }))}
				value={widgetType}
				onchange={onWidgetTypeChange}
			/>
			{#if widgetType}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">
					{widgetTypes.find((w) => w.type === widgetType)?.description ?? ""}
				</p>
			{/if}
		</div>
		<div>
			<label for="widget-props" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.widget_props", "Props (JSON)")}</label>
			<textarea id="widget-props" bind:value={widgetProps} rows="6" class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={'{}'}></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="widget-visibility" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.visibility", "Visibility")}</label>
				<Select
					options={[
						{ value: "all", label: t("sidebars.visibility_all", "All") },
						{ value: "authenticated", label: t("sidebars.visibility_authenticated", "Authenticated") },
						{ value: "guest", label: t("sidebars.visibility_guest", "Guest") }
					]}
					value={widgetVisibility}
					onchange={(val) => (widgetVisibility = val)}
				/>
			</div>
			<div>
				<label for="widget-status" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("common.status", "Status")}</label>
				<Select
					options={[
						{ value: "active", label: t("common.status_active", "Active") },
						{ value: "inactive", label: t("common.status_inactive", "Inactive") }
					]}
					value={widgetStatus}
					onchange={(val) => (widgetStatus = val)}
				/>
			</div>
		</div>
		<div>
			<label for="widget-permission" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.permission_node", "Permission Node (optional)")}</label>
			<input id="widget-permission" type="text" bind:value={widgetPermissionNode} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="e.g., moderation.tools" />
		</div>
		<div>
			<label for="widget-required-groups" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.required_groups", "Required User Groups")}</label>
			<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{t("sidebars.required_groups_hint", "Widget is visible only to users in at least one selected group. Leave empty for everyone.")}</p>
			<div class="relative mt-1">
				<div class="flex flex-wrap items-center gap-1 w-full rounded-lg border p-1.5 min-h-[2.5rem]" style="border-color: var(--color-border); background: var(--color-surface)">
					{#each selectedGroupObjs as group (group.id)}
						<span class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-foreground)">
							{group.name}
							<button type="button" class="ml-0.5 opacity-60 hover:opacity-100" style="color: var(--color-foreground)" onclick={() => removeGroup(group.id)} aria-label={t("common.remove", "Remove")}>&times;</button>
						</span>
					{/each}
					<input
						id="widget-required-groups"
						type="text"
						value={groupSearch}
						oninput={handleGroupInput}
						onfocus={() => { groupDropdownOpen = true; }}
						onblur={() => setTimeout(() => (groupDropdownOpen = false), 200)}
						onkeydown={(e) => {
							if (e.key === "Escape") groupDropdownOpen = false;
							else if (e.key === "Enter" && groupSuggestions.length > 0) { e.preventDefault(); addGroup(groupSuggestions[0].id); }
							else if (e.key === "Backspace" && !groupSearch && selectedGroupObjs.length > 0) { removeGroup(selectedGroupObjs[selectedGroupObjs.length - 1].id); }
						}}
						class="flex-1 min-w-[8rem] bg-transparent border-0 outline-none text-sm py-1 px-1"
						style="color: var(--color-foreground)"
						placeholder={widgetRequiredGroupIds.size === 0 ? t("sidebars.search_groups", "Search groups...") : ""}
					/>
				</div>
				{#if groupDropdownOpen && groupSuggestions.length > 0}
					<ul class="absolute z-[60] mt-1 max-h-48 w-full overflow-auto rounded-lg border shadow-lg" style="border-color: var(--color-border); background: var(--color-surface)">
						{#each groupSuggestions as group (group.id)}
							<li>
								<button type="button" class="w-full px-3 py-2 text-left text-sm hover:bg-black/5 dark:hover:bg-white/10 flex items-center gap-2" style="color: var(--color-foreground)" onmousedown={(e) => { e.preventDefault(); addGroup(group.id); }}>
									<span>{group.name}</span>
									{#if group.is_system ?? false}<span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--color-muted); color: var(--color-surface)">system</span>{/if}
									<span class="ml-auto text-xs" style="color: var(--color-muted)">{group.member_count ?? 0}</span>
								</button>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showWidgetModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveWidget} disabled={savingWidget || !widgetType.trim()}>
			{savingWidget ? t("common.saving", "Saving...") : t("common.save", "Save")}
		</Button>
	</div>
</Modal>

<!-- ── Delete Sidebar Confirmation ────────────────────────────────── -->
<Modal bind:open={showDeleteSidebarConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteSidebarTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("sidebars.delete_confirm", "Delete sidebar")} <strong>{deleteSidebarTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone. All widgets in this sidebar will be deleted.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteSidebarConfirm = false)} disabled={deletingSidebar}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDeleteSidebar} disabled={deletingSidebar}>
			{deletingSidebar ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}
		</Button>
	</div>
</Modal>

<!-- ── Delete Widget Confirmation ───────────────────────────────────── -->
<Modal bind:open={showDeleteWidgetConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	<p class="text-sm" style="color: var(--color-foreground)">{t("sidebars.delete_widget_confirm", "Delete this widget?")}</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteWidgetConfirm = false)} disabled={deletingWidget}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDeleteWidget} disabled={deletingWidget}>
			{deletingWidget ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}
		</Button>
	</div>
</Modal>

<!-- ── Action Cell Snippets ────────────────────────────────────────── -->
{#snippet SidebarActionsCell({ value, row }: { value: any; row: Sidebar })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => viewWidgets(row)} style="color: var(--color-primary)">
			{t("sidebars.widgets", "Widgets")}
		</Button>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => openEditSidebar(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDeleteSidebar(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

{#snippet WidgetActionsCell({ value, row }: { value: any; row: SidebarWidget })}
	<div class="flex items-center gap-1">
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => moveWidget(row.id, -1)}>
				<ArrowUp class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => moveWidget(row.id, 1)}>
				<ArrowDown class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openEditWidget(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDeleteWidget(row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
