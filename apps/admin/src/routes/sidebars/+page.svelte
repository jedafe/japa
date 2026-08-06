<script lang="ts">
	// /sidebars — migrated to the EntityPage scaffold.
	//
	// The sidebar list + sidebar CRUD use EntityPage. The sidebar create/edit
	// form uses the `formSnippet` escape hatch because it includes a
	// required_group_ids multi-select (autocomplete) that EntityForm cannot
	// render. The rich WIDGET editor is kept as a page-local `<SlideOver>`
	// sub-flow (mirrors the /tags moderators SlideOver pattern).
	// Ref: Section 59 (Sidebar Management System), ADR-066.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { Select } from "$lib/components/ui/select";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { sidebarsAdminApi, userManagementApi, type UserGroup } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Eye, Pencil, Trash2, Plus, ArrowUp, ArrowDown, LayoutGrid } from "lucide-svelte";

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
		widget_count?: number;
	};

	const canManage = $derived(hasPermission("sidebars.manage"));

	// ── User groups + widget types (loaded on mount for the sub-flows) ──────
	let userGroups = $state<UserGroup[]>([]);
	let widgetTypes = $state<WidgetType[]>([]);

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

	$effect(() => {
		void loadUserGroups();
		void loadWidgetTypes();
	});

	// ── EntityPage controller + bulk delete ─────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = "";
		try {
			await Promise.all(idArr.map((id) => sidebarsAdminApi.delete(id)));
			// EntityPage auto-reloads + clears selection after this returns.
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to delete sidebars");
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Sidebar form: group multi-select state ──────────────────────────────
	// The formSnippet binds to `value`; the group chips + autocomplete operate
	// directly on `value.required_group_ids` (an array) so no separate sync is
	// needed. Only the search input + dropdown open state are page-local.
	let sidebarGroupSearch = $state("");
	let sidebarGroupDropdownOpen = $state(false);
	let sidebarGroupDebounce: ReturnType<typeof setTimeout> | null = null;

	function handleSidebarGroupInput(e: Event) {
		const target = e.target as HTMLInputElement;
		sidebarGroupSearch = target.value;
		if (sidebarGroupDebounce) clearTimeout(sidebarGroupDebounce);
		sidebarGroupDebounce = setTimeout(() => {
			sidebarGroupDropdownOpen = true;
		}, 200);
	}

	function addSidebarGroup(value: Record<string, any>, groupId: string) {
		const arr = Array.isArray(value.required_group_ids) ? value.required_group_ids : [];
		if (!arr.includes(groupId)) {
			value.required_group_ids = [...arr, groupId];
		}
		sidebarGroupSearch = "";
	}

	function removeSidebarGroup(value: Record<string, any>, groupId: string) {
		const arr = Array.isArray(value.required_group_ids) ? value.required_group_ids : [];
		value.required_group_ids = arr.filter((id: string) => id !== groupId);
	}

	// ── Sidebar form normalization (mirrors the legacy saveSidebar body) ────
	function normalizeSidebarBody(formValue: Record<string, any>): Record<string, unknown> {
		return {
			app: String(formValue.app ?? "main"),
			position: String(formValue.position ?? "right"),
			slug: String(formValue.slug ?? "")
				.trim()
				.toLowerCase()
				.replace(/\s+/g, "-"),
			name: String(formValue.name ?? "").trim(),
			page_pattern: String(formValue.page_pattern ?? "").trim() || "*",
			visibility: String(formValue.visibility ?? "all"),
			required_group_ids: Array.isArray(formValue.required_group_ids)
				? formValue.required_group_ids
				: [],
			status: String(formValue.status ?? "active"),
		};
	}

	// `fields` seeds the EntityPage edit form value; the actual inputs are
	// rendered by the `formSnippet` below (the group multi-select needs custom UI).
	const fields: FieldDef[] = [
		{ key: "name", label: t("sidebars.name", "Name"), type: "text" },
		{ key: "slug", label: t("sidebars.slug", "Slug"), type: "text" },
		{ key: "app", label: t("sidebars.app", "App"), type: "select", options: [
			{ value: "main", label: t("sidebars.app_main", "Main") },
			{ value: "docs", label: t("sidebars.app_docs", "Docs") },
			{ value: "learn", label: t("sidebars.app_learn", "Learn") },
		] },
		{ key: "position", label: t("sidebars.position", "Position"), type: "select", options: [
			{ value: "left", label: t("sidebars.position_left", "Left") },
			{ value: "right", label: t("sidebars.position_right", "Right") },
		] },
		{ key: "page_pattern", label: t("sidebars.page_pattern", "Page Pattern"), type: "text" },
		{ key: "visibility", label: t("sidebars.visibility", "Visibility"), type: "select", options: [
			{ value: "all", label: t("sidebars.visibility_all", "All") },
			{ value: "authenticated", label: t("sidebars.visibility_authenticated", "Authenticated") },
			{ value: "guest", label: t("sidebars.visibility_guest", "Guest") },
		] },
		{ key: "status", label: t("common.status", "Status"), type: "select", options: [
			{ value: "active", label: t("common.status_active", "Active") },
			{ value: "inactive", label: t("common.status_inactive", "Inactive") },
		] },
		{ key: "required_group_ids", label: "Required Group IDs", type: "text" },
	];

	// ── Widget editor sub-flow (page-local SlideOver) ───────────────────────
	let showWidgetsSlideOver = $state(false);
	let widgetsSidebar = $state<Sidebar | null>(null);
	let loadingWidgets = $state(false);
	let widgetMessage = $state("");

	// Widget create/edit modal (inside the SlideOver)
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

	// Widget group multi-select state
	let groupSearch = $state("");
	let groupDropdownOpen = $state(false);
	let groupDebounce: ReturnType<typeof setTimeout> | null = null;

	const selectedGroupObjs = $derived(
		userGroups.filter((g) => widgetRequiredGroupIds.has(g.id))
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

	// Delete widget confirmation
	let deleteWidgetTarget = $state<string | null>(null);
	let showDeleteWidgetConfirm = $state(false);
	let deletingWidget = $state(false);

	async function openWidgets(sidebar: Sidebar) {
		widgetsSidebar = sidebar;
		showWidgetsSlideOver = true;
		widgetMessage = "";
		await reloadWidgets();
	}

	async function reloadWidgets() {
		if (!widgetsSidebar) return;
		loadingWidgets = true;
		widgetMessage = "";
		try {
			const result = await sidebarsAdminApi.get(widgetsSidebar.id);
			if (result.ok) {
				widgetsSidebar = result.data as Sidebar;
			}
		} catch {
			// keep existing sidebar data on error
		} finally {
			loadingWidgets = false;
		}
	}

	function openCreateWidget() {
		if (!widgetsSidebar) return;
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

	function onWidgetTypeChange(val: string) {
		widgetType = val;
		const wt = widgetTypes.find((w) => w.type === val);
		if (wt && !editingWidget) {
			widgetProps = JSON.stringify(wt.default_props ?? {}, null, 2);
		}
	}

	async function saveWidget() {
		if (!widgetType.trim() || !widgetsSidebar) return;
		savingWidget = true;
		widgetMessage = "";
		try {
			let props: Record<string, unknown> = {};
			try {
				props = JSON.parse(widgetProps || "{}");
			} catch {
				widgetMessage = t("sidebars.invalid_props_json", "Props must be valid JSON");
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
				status: widgetStatus,
			};
			const result = editingWidget
				? await sidebarsAdminApi.updateWidget(widgetsSidebar.id, editingWidget.id, body)
				: await sidebarsAdminApi.addWidget(widgetsSidebar.id, body);
			if (result.ok) {
				showWidgetModal = false;
				widgetMessage = editingWidget
					? t("sidebars.widget_updated", "Widget updated")
					: t("sidebars.widget_created", "Widget created");
				await reloadWidgets();
				entityController?.reload();
			} else {
				widgetMessage = result.error ?? t("common.error", "Failed to save widget");
			}
		} catch (e) {
			widgetMessage = e instanceof Error ? e.message : t("common.error", "Failed to save widget");
		} finally {
			savingWidget = false;
		}
	}

	function openDeleteWidget(widgetId: string) {
		deleteWidgetTarget = widgetId;
		showDeleteWidgetConfirm = true;
	}

	async function confirmDeleteWidget() {
		if (!deleteWidgetTarget || !widgetsSidebar || deletingWidget) return;
		deletingWidget = true;
		const result = await sidebarsAdminApi.deleteWidget(widgetsSidebar.id, deleteWidgetTarget);
		if (result.ok) {
			showDeleteWidgetConfirm = false;
			deleteWidgetTarget = null;
			widgetMessage = t("sidebars.widget_deleted", "Widget deleted");
			await reloadWidgets();
			entityController?.reload();
		} else {
			widgetMessage = result.error ?? t("common.error", "Failed to delete widget");
		}
		deletingWidget = false;
	}

	async function moveWidget(widgetId: string, dir: -1 | 1) {
		if (!widgetsSidebar) return;
		const widgets = [...(widgetsSidebar.widgets ?? [])].sort((a, b) => a.sort_order - b.sort_order);
		const idx = widgets.findIndex((w) => w.id === widgetId);
		const swap = idx + dir;
		if (idx < 0 || swap < 0 || swap >= widgets.length) return;
		[widgets[idx], widgets[swap]] = [widgets[swap], widgets[idx]];
		const reorderItems = widgets.map((w, i) => ({ id: w.id, sort_order: i }));
		const result = await sidebarsAdminApi.reorderWidgets(widgetsSidebar.id, { items: reorderItems });
		if (result.ok) {
			await reloadWidgets();
		} else {
			widgetMessage = result.error ?? t("common.error", "Failed to reorder widgets");
		}
	}

	// Widget group multi-select handlers
	function handleGroupInput(e: Event) {
		const target = e.target as HTMLInputElement;
		groupSearch = target.value;
		if (groupDebounce) clearTimeout(groupDebounce);
		groupDebounce = setTimeout(() => {
			groupDropdownOpen = true;
		}, 200);
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

	const sortedWidgets = $derived(
		[...(widgetsSidebar?.widgets ?? [])].sort((a, b) => a.sort_order - b.sort_order)
	);

	// ── EntityPage config ───────────────────────────────────────────────────
	const config: EntityPageConfig = {
		title: t("sidebars.title", "Sidebar Manager"),
		description: t("sidebars.description", "Manage sidebars across Main, Docs, and Learn apps with per-page and per-usergroup visibility"),
		createLabel: t("sidebars.create", "Create Sidebar"),
		createPermission: "sidebars.manage",
		pageSize: 25,
		columns: [
			{ key: "name", label: t("sidebars.name", "Name"), sortable: true },
			{
				key: "app",
				label: t("sidebars.app", "App"),
				sortable: true,
				render: (v: string) => t(`sidebars.app_${v}`, v),
			},
			{
				key: "position",
				label: t("sidebars.position", "Position"),
				sortable: true,
				render: (v: string) => t(`sidebars.position_${v}`, v),
			},
			{ key: "slug", label: t("sidebars.slug", "Slug"), sortable: true },
			{ key: "page_pattern", label: t("sidebars.page_pattern", "Page Pattern"), sortable: true },
			{ key: "status", label: t("common.status", "Status"), sortable: true, componentSnippet: StatusCell },
			{
				key: "widget_count",
				label: t("sidebars.widgets", "Widgets"),
				sortable: false,
				render: (_v: any, row: any) => String(row?.widget_count ?? row?.widgets?.length ?? 0),
			},
			{
				key: "updated_at",
				label: t("common.updated", "Updated"),
				sortable: true,
				render: (v: string) => (v ? new Date(v).toLocaleDateString() : "—"),
			},
		],
		filters: [
			{
				key: "app",
				label: t("sidebars.app", "App"),
				options: [
					{ value: "main", label: t("sidebars.app_main", "Main") },
					{ value: "docs", label: t("sidebars.app_docs", "Docs") },
					{ value: "learn", label: t("sidebars.app_learn", "Learn") },
				],
			},
			{
				key: "position",
				label: t("sidebars.position", "Position"),
				options: [
					{ value: "left", label: t("sidebars.position_left", "Left") },
					{ value: "right", label: t("sidebars.position_right", "Right") },
				],
			},
			{
				key: "status",
				label: t("common.status", "Status"),
				options: [
					{ value: "active", label: t("common.status_active", "Active") },
					{ value: "inactive", label: t("common.status_inactive", "Inactive") },
				],
			},
		],
		actions: [
			{ label: t("common.view", "View"), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
			{ label: t("common.edit", "Edit"), icon: Pencil, onclick: (_row, ctx) => ctx?.openEdit(_row) },
			{ label: t("sidebars.manage_widgets", "Manage Widgets"), icon: LayoutGrid, onclick: (row) => openWidgets(row) },
			{ label: t("common.delete", "Delete"), icon: Trash2, variant: "destructive", onclick: (_row, ctx) => ctx?.openDelete(_row) },
		],
		bulkActions: [
			{
				label: t("common.deleteSelected", "Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: bulkDelete,
			},
		],
		fields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (filter.app) params.set("app", filter.app);
			if (filter.position) params.set("position", filter.position);
			if (filter.status) params.set("status", filter.status);
			if (q) params.set("q", q);
			const result = await sidebarsAdminApi.list(params.toString() || undefined);
			if (result.ok) {
				const items = ((result.data as { items: Sidebar[] })?.items) ?? [];
				return { data: items, total: items.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t("common.error", "Failed to load sidebars"));
		},
		create: async (body) => {
			const res = await sidebarsAdminApi.create(normalizeSidebarBody(body));
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await sidebarsAdminApi.update(id, normalizeSidebarBody(body));
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await sidebarsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: SidebarView,
		formSnippet: SidebarForm,
	};
</script>

<svelte:head>
	<title>{t("sidebars.title", "Sidebar Manager")} — JapaConnect Admin</title>
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

<!-- ── Widget Editor SlideOver (page-local sub-flow) ─────────────────────── -->
<SlideOver bind:open={showWidgetsSlideOver} title={t("sidebars.widgets", "Widgets")} size="lg">
	{#if widgetsSidebar}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t("sidebars.title", "Sidebar")}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{widgetsSidebar.name}
					<span class="ml-2 text-sm font-normal" style="color: var(--color-muted)">
						({t("sidebars.app_" + widgetsSidebar.app, widgetsSidebar.app)} · {t("sidebars.position_" + widgetsSidebar.position, widgetsSidebar.position)} · {widgetsSidebar.page_pattern})
					</span>
				</p>
			</div>

			{#if widgetMessage}
				<div
					class="rounded-lg p-2 text-sm"
					style="background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)"
				>
					{widgetMessage}
				</div>
			{/if}

			{#if canManage}
				<Button onclick={openCreateWidget} size="sm">
					<Plus class="mr-1 h-3.5 w-3.5" />
					{t("sidebars.add_widget", "Add Widget")}
				</Button>
			{/if}

			{#if loadingWidgets}
				<TableSkeleton rows={4} columns={3} />
			{:else if sortedWidgets.length === 0}
				<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
					{t("sidebars.no_widgets", "No widgets in this sidebar. Add one to get started.")}
				</div>
			{:else}
				<div class="space-y-2">
					{#each sortedWidgets as widget (widget.id)}
						<div
							class="flex items-center justify-between rounded-lg border p-3"
							style="border-color: var(--color-border)"
						>
							<div class="min-w-0 flex-1">
								<p class="text-sm font-medium" style="color: var(--color-foreground)">
									{widget.label || "—"}
								</p>
								<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
									{widgetTypes.find((w) => w.type === widget.widget_type)?.name ?? widget.widget_type.replace(/_/g, " ")}
									· {t(`sidebars.visibility_${widget.visibility}`, widget.visibility)}
									· {t("sidebars.sort_order", "Order")}: {widget.sort_order}
								</p>
							</div>
							{#if canManage}
								<div class="flex items-center gap-1">
									<Button variant="ghost" size="sm" onclick={() => moveWidget(widget.id, -1)} aria-label={t("common.move_up", "Move Up")}>
										<ArrowUp class="h-3.5 w-3.5" />
									</Button>
									<Button variant="ghost" size="sm" onclick={() => moveWidget(widget.id, 1)} aria-label={t("common.move_down", "Move Down")}>
										<ArrowDown class="h-3.5 w-3.5" />
									</Button>
									<Button variant="ghost" size="sm" onclick={() => openEditWidget(widget)}>
										<Pencil class="h-3.5 w-3.5" />
									</Button>
									<Button variant="ghost" size="sm" onclick={() => openDeleteWidget(widget.id)} style="color: var(--color-danger)" aria-label={t("common.delete", "Delete")}>
										<Trash2 class="h-3.5 w-3.5" />
									</Button>
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		<Button variant="outline" onclick={() => (showWidgetsSlideOver = false)}>
			{t("common.close", "Close")}
		</Button>
	{/snippet}
</SlideOver>

<!-- ── Widget Create/Edit Modal ──────────────────────────────────────────── -->
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
						{ value: "guest", label: t("sidebars.visibility_guest", "Guest") },
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
						{ value: "inactive", label: t("common.status_inactive", "Inactive") },
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

<!-- ── Delete Widget Confirmation ────────────────────────────────────────── -->
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

<!-- ── Snippets ──────────────────────────────────────────────────────────── -->

<!-- Status badge column -->
{#snippet StatusCell({ value }: { value: string; row: Sidebar })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value === 'active' ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}"
	>
		{t(`common.status_${value}`, value)}
	</span>
{/snippet}

<!-- View slide-over body: sidebar basic fields + read-only widget list -->
{#snippet SidebarView({ row }: { row: Sidebar })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("sidebars.name", "Name")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("sidebars.slug", "Slug")}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("sidebars.app", "App")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t(`sidebars.app_${row.app}`, row.app)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("sidebars.position", "Position")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t(`sidebars.position_${row.position}`, row.position)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("sidebars.page_pattern", "Page Pattern")}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.page_pattern}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("sidebars.visibility", "Visibility")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t(`sidebars.visibility_${row.visibility}`, row.visibility)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.status", "Status")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t(`common.status_${row.status}`, row.status)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.updated", "Updated")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : "—"}</p>
			</div>
		</div>

		{#if row.required_group_ids && row.required_group_ids.length > 0}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("sidebars.required_groups", "Required User Groups")}</p>
				<div class="mt-1 flex flex-wrap gap-1">
					{#each row.required_group_ids as gid (gid)}
						{@const g = userGroups.find((ug) => ug.id === gid)}
						<span class="inline-block rounded px-2 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-foreground)">
							{g?.name ?? gid}
						</span>
					{/each}
				</div>
			</div>
		{/if}

		<div class="border-t pt-3" style="border-color: var(--color-border)">
			<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t("sidebars.widgets", "Widgets")} ({row.widgets?.length ?? row.widget_count ?? 0})
			</p>
			{#if row.widgets && row.widgets.length > 0}
				<div class="space-y-1">
					{#each [...row.widgets].sort((a, b) => a.sort_order - b.sort_order) as widget (widget.id)}
						<div class="flex items-center justify-between rounded border px-2 py-1.5" style="border-color: var(--color-border)">
							<span class="text-sm" style="color: var(--color-foreground)">{widget.label || "—"}</span>
							<span class="text-xs" style="color: var(--color-muted)">
								{widgetTypes.find((w) => w.type === widget.widget_type)?.name ?? widget.widget_type}
							</span>
						</div>
					{/each}
				</div>
			{:else}
				<p class="text-sm" style="color: var(--color-muted)">{t("sidebars.no_widgets", "No widgets in this sidebar.")}</p>
			{/if}
		</div>
	</div>
{/snippet}

<!-- Create/Edit form body (rich form: group multi-select) -->
{#snippet SidebarForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	{@const selectedIds = new Set(Array.isArray(value.required_group_ids) ? value.required_group_ids : [])}
	{@const selectedSidebarGroupObjs = userGroups.filter((g) => selectedIds.has(g.id))}
	{@const sidebarGroupSuggestions = (
		sidebarGroupSearch.trim()
			? userGroups.filter(
					(g) =>
						!selectedIds.has(g.id) &&
						(g.name.toLowerCase().includes(sidebarGroupSearch.toLowerCase()) ||
							g.slug.toLowerCase().includes(sidebarGroupSearch.toLowerCase()))
			)
			: userGroups.filter((g) => !selectedIds.has(g.id))
	).slice(0, 10)}
	<div class="space-y-4">
		<div>
			<label for="sidebar-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.name", "Name")}</label>
			<input id="sidebar-name" type="text" bind:value={value.name} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="e.g., Dashboard Right" />
		</div>
		<div>
			<label for="sidebar-slug" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.slug", "Slug")}</label>
			<input id="sidebar-slug" type="text" bind:value={value.slug} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="dashboard-right" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="sidebar-app" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.app", "App")}</label>
				<Select
					options={[
						{ value: "main", label: t("sidebars.app_main", "Main") },
						{ value: "docs", label: t("sidebars.app_docs", "Docs") },
						{ value: "learn", label: t("sidebars.app_learn", "Learn") },
					]}
					value={value.app}
					onchange={(val) => (value.app = val)}
				/>
			</div>
			<div>
				<label for="sidebar-position" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.position", "Position")}</label>
				<Select
					options={[
						{ value: "left", label: t("sidebars.position_left", "Left") },
						{ value: "right", label: t("sidebars.position_right", "Right") },
					]}
					value={value.position}
					onchange={(val) => (value.position = val)}
				/>
			</div>
		</div>
		<div>
			<label for="sidebar-pattern" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.page_pattern", "Page Pattern")}</label>
			<input id="sidebar-pattern" type="text" bind:value={value.page_pattern} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="*" />
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t("sidebars.page_pattern_hint", "Glob pattern. * matches all pages. /dashboard matches only that page. /u/* matches all profile pages.")}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="sidebar-visibility" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("sidebars.visibility", "Visibility")}</label>
				<Select
					options={[
						{ value: "all", label: t("sidebars.visibility_all", "All") },
						{ value: "authenticated", label: t("sidebars.visibility_authenticated", "Authenticated") },
						{ value: "guest", label: t("sidebars.visibility_guest", "Guest") },
					]}
					value={value.visibility}
					onchange={(val) => (value.visibility = val)}
				/>
			</div>
			<div>
				<label for="sidebar-status" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("common.status", "Status")}</label>
				<Select
					options={[
						{ value: "active", label: t("common.status_active", "Active") },
						{ value: "inactive", label: t("common.status_inactive", "Inactive") },
					]}
					value={value.status}
					onchange={(val) => (value.status = val)}
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
							<button type="button" class="ml-0.5 opacity-60 hover:opacity-100" style="color: var(--color-foreground)" onclick={() => removeSidebarGroup(value, group.id)} aria-label={t("common.remove", "Remove")}>&times;</button>
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
							else if (e.key === "Enter" && sidebarGroupSuggestions.length > 0) { e.preventDefault(); addSidebarGroup(value, sidebarGroupSuggestions[0].id); }
							else if (e.key === "Backspace" && !sidebarGroupSearch && selectedSidebarGroupObjs.length > 0) { removeSidebarGroup(value, selectedSidebarGroupObjs[selectedSidebarGroupObjs.length - 1].id); }
						}}
						class="flex-1 min-w-[8rem] bg-transparent border-0 outline-none text-sm py-1 px-1"
						style="color: var(--color-foreground)"
						placeholder={selectedIds.size === 0 ? t("sidebars.search_groups", "Search groups...") : ""}
					/>
				</div>
				{#if sidebarGroupDropdownOpen && sidebarGroupSuggestions.length > 0}
					<ul class="absolute z-[60] mt-1 max-h-48 w-full overflow-auto rounded-lg border shadow-lg" style="border-color: var(--color-border); background: var(--color-surface)">
						{#each sidebarGroupSuggestions as group (group.id)}
							<li>
								<button type="button" class="w-full px-3 py-2 text-left text-sm hover:bg-black/5 dark:hover:bg-white/10 flex items-center gap-2" style="color: var(--color-foreground)" onmousedown={(e) => { e.preventDefault(); addSidebarGroup(value, group.id); }}>
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
{/snippet}
