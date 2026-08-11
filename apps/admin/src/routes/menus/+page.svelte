<script lang="ts">
	// /menus — migrated to the EntityPage scaffold.
	//
	// The menu list + menu CRUD use the shared EntityPage scaffold (declarative
	// `fields`). The rich menu-ITEM editor (add/edit/delete/reorder items with
	// icon picker, route suggestions, permission node, group selection, nesting,
	// visibility, target) is kept as a page-local `<SlideOver>` sub-flow that
	// mirrors the tags moderators SlideOver pattern.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Select } from '$lib/components/ui/select';
	import IconPicker from '$lib/components/ui/icon-picker.svelte';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { menusAdminApi, userManagementApi, permissionsApi, type UserGroup } from '$lib/api';
	import { suggestRoutes } from '$lib/data/routes';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2, ListTree, ArrowUp, ArrowDown, Plus } from 'lucide-svelte';

	type MenuItem = {
		id: string;
		tenant_id: string;
		menu_id: string;
		parent_id?: string | null;
		label: string;
		url: string;
		icon?: string | null;
		sort_order: number;
		permission_node?: string | null;
		required_group_ids?: string[];
		visibility: string;
		target: string;
		is_ad_slot: boolean;
		ad_slot_config?: Record<string, unknown> | null;
		status: string;
		children?: MenuItem[];
	};

	type Menu = {
		id: string;
		tenant_id: string;
		slug: string;
		name: string;
		placement: string;
		status: string;
		created_at: string;
		updated_at: string;
		items?: MenuItem[];
		item_count?: number;
	};

	const canManageMenus = $derived(hasPermission('menus.manage'));

	// ── Placement / status option sets (mirror the legacy form) ───────────────
	const placementOptions = [
		{ value: 'sidebar', label: t('menus.placement_sidebar', 'Sidebar') },
		{ value: 'bottom_bar', label: t('menus.placement_bottom_bar', 'Bottom Bar') },
		{ value: 'dropdown', label: t('menus.placement_dropdown', 'Dropdown') },
		{ value: 'footer', label: t('menus.placement_footer', 'Footer') },
		{ value: 'header', label: t('menus.placement_header', 'Header') }
	];

	const statusOptions = [
		{ value: 'active', label: t('common.status_active', 'Active') },
		{ value: 'inactive', label: t('common.status_inactive', 'Inactive') }
	];

	// ── Bulk delete (in-flight guard) ─────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => menusAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete menus');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering + edit-form seeding for the MENU entity.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('menus.name', 'Name'), type: 'text', placeholder: 'e.g., Primary Nav', required: true },
		{ key: 'slug', label: t('menus.slug', 'Slug'), type: 'text', placeholder: 'primary-nav', required: true, readonlyInEdit: true, help: t('menus.slug_immutable', 'Slug cannot be changed after creation') },
		{ key: 'placement', label: t('menus.placement', 'Placement'), type: 'select', options: placementOptions },
		{ key: 'status', label: t('common.status', 'Status'), type: 'select', options: statusOptions }
	];

	const config: EntityPageConfig = {
		title: t('menus.title', 'Menus'),
		description: t('menus.description', 'Manage navigation menus and their items'),
		createLabel: t('menus.create', 'Create Menu'),
		createPermission: 'menus.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('menus.name', 'Name'), sortable: true },
			{ key: 'slug', label: t('menus.slug', 'Slug'), sortable: true },
			{
				key: 'placement',
				label: t('menus.placement', 'Placement'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-foreground)">${t('menus.placement_' + v, v)}</span>`
			},
			{
				key: 'status',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const active = v === 'active';
					return `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${active ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">${t('common.status_' + v, v)}</span>`;
				}
			},
			{
				key: 'items',
				label: t('menus.items', 'Items'),
				sortable: false,
				render: (_v: any, row: Menu) => String(row.item_count ?? row.items?.length ?? 0)
			},
			{
				key: 'updated_at',
				label: t('common.updated', 'Updated'),
				sortable: true,
				render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—')
			}
		],
		filters: [
			{
				key: 'placement',
				label: t('menus.placement', 'Placement'),
				options: placementOptions
			},
			{
				key: 'status',
				label: t('common.status', 'Status'),
				options: statusOptions
			}
		],
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
			{ label: t('menus.manage_items', 'Manage Items'), icon: ListTree, onclick: (row) => openItems(row) },
			{
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row, ctx) => ctx?.openEdit(row),
				show: () => canManageMenus
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row, ctx) => ctx?.openDelete(row),
				show: () => canManageMenus
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('q', q);
			if (filter.placement) params.set('placement', filter.placement);
			if (filter.status) params.set('status', filter.status);
			// EntityPage emits sort as "key:dir"; the API expects separate sort
			// (bare key) and sort_dir (asc/desc) params.
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				params.set('sort', sortKey);
				params.set('sort_dir', sortDir === 'desc' ? 'desc' : 'asc');
			}
			const result = await menusAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as {
					items: Menu[];
					page: number;
					total_pages: number;
					total: number;
				};
				return {
					data: d.items ?? [],
					total: d.total ?? 0,
					totalPages: d.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load menus'));
		},
		create: async (body) => {
			const res = await menusAdminApi.create({
				name: String(body.name ?? '').trim(),
				slug: String(body.slug ?? '').trim().toLowerCase().replace(/\s+/g, '-'),
				placement: String(body.placement ?? 'sidebar'),
				status: String(body.status ?? 'active')
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		// Slug is immutable after creation (backend UpdateMenu does not
		// accept slug — it is the public URL identifier per ADR-054).
		update: async (id, body) => {
			const res = await menusAdminApi.update(id, {
				name: String(body.name ?? '').trim(),
				placement: String(body.placement ?? 'sidebar'),
				status: String(body.status ?? 'active')
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await menusAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: MenuView
	};

	// ── Menu-ITEM editor (page-local SlideOver sub-flow) ──────────────────────
	// Mirrors the tags moderators SlideOver pattern. Loads the full items tree via
	// `menusAdminApi.get(id)` and supports add/edit/delete/reorder through the
	// item endpoints, calling `entityController?.reload()` after every mutation.
	let showItemsModal = $state(false);
	let itemsMenu = $state<Menu | null>(null);
	let menuItems = $state<MenuItem[]>([]);
	let loadingItems = $state(false);
	let itemsError = $state('');

	// Item create/edit modal state
	let showItemModal = $state(false);
	let editingItem = $state<MenuItem | null>(null);
	let savingItem = $state(false);
	let itemLabel = $state('');
	let itemUrl = $state('');
	let itemParent = $state('');
	let itemVisibility = $state<string>('all');
	let itemTarget = $state<string>('_self');
	let itemIcon = $state('');
	let itemPermissionNode = $state('');
	let itemRequiredGroupIds = $state<Set<string>>(new Set());

	// User groups for the required_group_ids multi-select
	let userGroups = $state<UserGroup[]>([]);

	// URL combobox state
	let urlSearch = $state('');
	let urlDropdownOpen = $state(false);

	// Delete item confirm state
	let deleteItemTarget = $state<string | null>(null);
	let showDeleteItemConfirm = $state(false);

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

	// Selected group objects for chip rendering
	const selectedGroupObjs = $derived(userGroups.filter((g) => itemRequiredGroupIds.has(g.id)));

	// ── Permission-node autocomplete data ────────────────────────────────
	// Permission nodes are sourced from GET /admin/permissions (RBAC catalog).
	// Loaded once on mount and cached client-side; the fetcher filters the
	// cached list by the debounced query. Ref: ADR-054 permission_node field.
	let permissionNodes = $state<string[]>([]);
	let permissionNodesLoaded = $state(false);

	async function loadPermissionNodes() {
		if (permissionNodesLoaded) return;
		try {
			const result = await permissionsApi.list();
			if (result.ok) {
				permissionNodes = (result.data as string[]) ?? [];
				permissionNodesLoaded = true;
			}
		} catch {
			permissionNodes = [];
		}
	}

	// Fetcher for the Permission Node Autocomplete. Filters the cached node
	// list by the query (case-insensitive substring). The user may still type
	// a custom node not in the catalog (free-text is preserved by binding).
	function permissionNodeFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.trim().toLowerCase();
		const matched = needle
			? permissionNodes.filter((n) => n.toLowerCase().includes(needle))
			: permissionNodes;
		return Promise.resolve(
			matched.slice(0, 50).map((n) => ({ id: n, label: n }))
		);
	}

	// Initial suggestions for the Permission Node Autocomplete when focused
	// with an empty query — shows the first 50 catalog nodes.
	function permissionNodeInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(
			permissionNodes.slice(0, 50).map((n) => ({ id: n, label: n }))
		);
	}

	// Fetcher for the Required User Groups Autocomplete. Filters the cached
	// userGroups list by name/slug and excludes already-selected groups.
	function groupFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.trim().toLowerCase();
		const matched = needle
			? userGroups.filter(
					(g) =>
						!itemRequiredGroupIds.has(g.id) &&
						(g.name.toLowerCase().includes(needle) ||
							g.slug.toLowerCase().includes(needle))
				)
			: userGroups.filter((g) => !itemRequiredGroupIds.has(g.id));
		return Promise.resolve(
			matched.slice(0, 20).map((g) => ({
				id: g.id,
				label: g.name,
				sublabel: g.is_system ? 'system' : `${g.member_count ?? 0} members`
			}))
		);
	}

	// Initial suggestions for the Required User Groups Autocomplete.
	function groupInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(
			userGroups
				.filter((g) => !itemRequiredGroupIds.has(g.id))
				.slice(0, 20)
				.map((g) => ({
					id: g.id,
					label: g.name,
					sublabel: g.is_system ? 'system' : `${g.member_count ?? 0} members`
				}))
		);
	}

	function selectGroup(item: AutocompleteItem) {
		const next = new Set(itemRequiredGroupIds);
		next.add(item.id);
		itemRequiredGroupIds = next;
	}

	function clearGroupSelection() {
		itemRequiredGroupIds = new Set();
	}

	// Collect all existing URLs from the managed menu's items (to merge with route list)
	const existingUrlSet = $derived.by(() => {
		const urls = new Set<string>();
		for (const item of menuItems) {
			if (item.url) urls.add(item.url);
		}
		return urls;
	});

	// URL suggestions: all JapaConnect routes + any custom URLs from existing items
	const urlSuggestions = $derived(
		suggestRoutes(urlSearch, new Set([itemUrl]))
			.concat(
				[...existingUrlSet].filter(
					(u) =>
						!suggestRoutes('', new Set()).includes(u) &&
						u !== itemUrl &&
						(!urlSearch || u.toLowerCase().includes(urlSearch.toLowerCase()))
				)
			)
			.slice(0, 20)
	);

	function removeGroup(groupId: string) {
		const next = new Set(itemRequiredGroupIds);
		next.delete(groupId);
		itemRequiredGroupIds = next;
	}

	async function openItems(menu: Menu) {
		itemsMenu = menu;
		showItemsModal = true;
		itemsError = '';
		await reloadItems();
	}

	async function reloadItems() {
		if (!itemsMenu) return;
		loadingItems = true;
		itemsError = '';
		try {
			const result = await menusAdminApi.get(itemsMenu.id);
			if (result.ok) {
				const full = result.data as Menu;
				menuItems = full.items ?? [];
				itemsMenu = { ...itemsMenu, items: menuItems };
			} else {
				itemsError = result.error ?? t('common.error', 'Failed to load menu items');
				menuItems = itemsMenu.items ?? [];
			}
		} catch (e) {
			itemsError = e instanceof Error ? e.message : t('common.error', 'Failed to load menu items');
			menuItems = itemsMenu.items ?? [];
		} finally {
			loadingItems = false;
		}
	}

	// Sorted flat list of items for display + reorder
	const sortedItems = $derived([...menuItems].sort((a, b) => a.sort_order - b.sort_order));

	// Item CRUD
	function openCreateItem() {
		if (!itemsMenu) return;
		editingItem = null;
		itemLabel = '';
		itemUrl = '';
		urlSearch = '';
		itemParent = '';
		itemVisibility = 'all';
		itemTarget = '_self';
		itemIcon = '';
		itemPermissionNode = '';
		itemRequiredGroupIds = new Set();
		showItemModal = true;
	}

	function openEditItem(itemId: string) {
		const item = menuItems.find((i) => i.id === itemId);
		if (!item) return;
		editingItem = item;
		itemLabel = item.label;
		itemUrl = item.url;
		urlSearch = '';
		itemParent = item.parent_id ?? '';
		itemVisibility = item.visibility || 'all';
		itemTarget = item.target || '_self';
		itemIcon = item.icon ?? '';
		itemPermissionNode = item.permission_node ?? '';
		itemRequiredGroupIds = new Set(item.required_group_ids ?? []);
		showItemModal = true;
	}

	async function saveItem() {
		if (!itemLabel.trim() || !itemUrl.trim() || !itemsMenu) return;
		savingItem = true;
		itemsError = '';
		try {
			const body: Record<string, unknown> = {
				label: itemLabel.trim(),
				url: itemUrl.trim(),
				parent_id: itemParent || null,
				visibility: itemVisibility,
				target: itemTarget,
				icon: itemIcon.trim() || null,
				permission_node: itemPermissionNode.trim() || null,
				required_group_ids: [...itemRequiredGroupIds],
				// Preserve fields with no UI control on edit so the backend
				// doesn't overwrite them with zero values (status="" violates
				// the menu_items.status CHECK constraint → 500).
				status: editingItem?.status ?? 'active',
				sort_order: editingItem?.sort_order ?? 0,
				is_ad_slot: editingItem?.is_ad_slot ?? false,
				ad_slot_config: editingItem?.ad_slot_config ?? null
			};
			const result = editingItem
				? await menusAdminApi.updateItem(itemsMenu.id, editingItem.id, body)
				: await menusAdminApi.addItem(itemsMenu.id, body);

			if (result.ok) {
				showItemModal = false;
				await reloadItems();
				entityController?.reload();
			} else {
				itemsError = result.error ?? t('common.error', 'Failed to save item');
			}
		} catch (e) {
			itemsError = e instanceof Error ? e.message : t('common.error', 'Failed to save item');
		} finally {
			savingItem = false;
		}
	}

	function openDeleteItem(itemId: string) {
		deleteItemTarget = itemId;
		showDeleteItemConfirm = true;
	}

	async function confirmDeleteItem() {
		if (!deleteItemTarget || !itemsMenu) return;
		const result = await menusAdminApi.deleteItem(itemsMenu.id, deleteItemTarget);
		if (result.ok) {
			showDeleteItemConfirm = false;
			deleteItemTarget = null;
			await reloadItems();
			entityController?.reload();
		} else {
			itemsError = result.error ?? t('common.error', 'Failed to delete item');
		}
	}

	async function moveItem(itemId: string, direction: number) {
		if (!itemsMenu) return;
		const items = [...sortedItems];
		const idx = items.findIndex((i) => i.id === itemId);
		const swapIdx = idx + direction;
		if (idx === -1 || swapIdx < 0 || swapIdx >= items.length) return;

		const reordered = [...items];
		[reordered[idx], reordered[swapIdx]] = [reordered[swapIdx], reordered[idx]];
		const reorderItems = reordered.map((i, sortIdx) => ({ id: i.id, sort_order: sortIdx }));

		const result = await menusAdminApi.reorderItems(itemsMenu.id, { items: reorderItems });
		if (result.ok) {
			await reloadItems();
			entityController?.reload();
		} else {
			itemsError = result.error ?? t('common.error', 'Failed to reorder items');
		}
	}

	// Build a nested tree from the flat items list for the read-only view snippet.
	function buildItemTree(items: MenuItem[]): MenuItem[] {
		const byId = new Map<string, MenuItem & { children: MenuItem[] }>();
		for (const it of items) byId.set(it.id, { ...it, children: [] });
		const roots: MenuItem[] = [];
		for (const it of byId.values()) {
			if (it.parent_id && byId.has(it.parent_id)) {
				byId.get(it.parent_id)!.children.push(it);
			} else {
				roots.push(it);
			}
		}
		return roots;
	}

	loadUserGroups();
loadPermissionNodes();
</script>

<svelte:head>
	<title>{t('menus.title', 'Menus')} — JapaConnect Admin</title>
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

<!-- Menu Items editor (page-local SlideOver) -->
<SlideOver bind:open={showItemsModal} title={t('menus.items', 'Items')} size="lg">
	{#if itemsMenu}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('menus.title', 'Menus')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{itemsMenu.name}
				</p>
			</div>

			{#if itemsError}
				<div
					class="rounded-lg p-2 text-sm"
					style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
				>
					{itemsError}
				</div>
			{/if}

			{#if canManageMenus}
				<Button onclick={openCreateItem} disabled={loadingItems}>
					<Plus class="mr-1 h-3.5 w-3.5" />
					{t('menus.add_item', 'Add Item')}
				</Button>
			{/if}

			{#if loadingItems}
				<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
					{t('common.loading', 'Loading...')}
				</div>
			{:else if sortedItems.length === 0}
				<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
					{t('menus.no_items', 'No items in this menu')}
				</div>
			{:else}
				<div class="space-y-2">
					{#each sortedItems as item (item.id)}
						<div
							class="flex items-center justify-between rounded-lg border p-3"
							style="border-color: var(--color-border)"
						>
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2">
									{#if item.icon}
										<span class="text-xs" style="color: var(--color-muted)">{item.icon}</span>
									{/if}
									<span class="truncate text-sm font-medium" style="color: var(--color-foreground)">
										{item.label}
									</span>
									{#if item.parent_id}
										<span class="text-xs" style="color: var(--color-muted)">
											↳ {t('menus.parent', 'Parent')}: {menuItems.find((p) => p.id === item.parent_id)?.label ?? '—'}
										</span>
									{/if}
								</div>
								<p class="mt-0.5 truncate text-xs font-mono" style="color: var(--color-muted)">
									{item.url}
								</p>
							</div>
							{#if canManageMenus}
								<div class="flex items-center gap-1">
									<Button variant="ghost" size="sm" onclick={() => moveItem(item.id, -1)} aria-label={t('common.move_up', 'Move up')}>
										<ArrowUp class="h-3.5 w-3.5" />
									</Button>
									<Button variant="ghost" size="sm" onclick={() => moveItem(item.id, 1)} aria-label={t('common.move_down', 'Move down')}>
										<ArrowDown class="h-3.5 w-3.5" />
									</Button>
									<Button variant="ghost" size="sm" onclick={() => openEditItem(item.id)} aria-label={t('common.edit', 'Edit')}>
										<Pencil class="h-3.5 w-3.5" />
									</Button>
									<Button
										variant="ghost"
										size="sm"
										onclick={() => openDeleteItem(item.id)}
										style="color: var(--color-danger)"
										aria-label={t('common.delete', 'Delete')}
									>
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
		<Button variant="outline" onclick={() => (showItemsModal = false)}>
			{t('common.close', 'Close')}
		</Button>
	{/snippet}
</SlideOver>

<!-- Item Create/Edit Modal -->
<Modal bind:open={showItemModal} title={editingItem ? t('menus.edit_item', 'Edit Item') : t('menus.add_item', 'Add Item')}>
	<div class="grid grid-cols-2 gap-4">
		<div>
			<label for="item-label" class="block text-sm font-medium" style="color: var(--color-foreground)">{t('menus.item_label', 'Label')}</label>
			<input id="item-label" type="text" bind:value={itemLabel} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t('menus.ph_home', 'Home')} />
		</div>
		<label class="block">
			<span class="block text-sm font-medium" style="color: var(--color-foreground)">{t('menus.icon', 'Icon')}</span>
			<div class="mt-1">
				<IconPicker bind:value={itemIcon} />
			</div>
		</label>
		<div class="col-span-2">
			<label for="item-url" class="block text-sm font-medium" style="color: var(--color-foreground)">{t('menus.item_url', 'URL')}</label>
			<div class="relative mt-1">
				<input
					id="item-url"
					type="text"
					bind:value={itemUrl}
					oninput={() => { urlSearch = itemUrl; urlDropdownOpen = true; }}
					onfocus={() => { urlSearch = itemUrl; urlDropdownOpen = true; }}
					onblur={() => setTimeout(() => (urlDropdownOpen = false), 200)}
					class="w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="/" />
				{#if urlDropdownOpen && urlSuggestions.length > 0}
					<div class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border shadow-lg" style="border-color: var(--color-border); background: var(--color-surface)">
						{#each urlSuggestions.slice(0, 10) as suggestedUrl (suggestedUrl)}
							<button
								type="button"
								class="block w-full px-3 py-1.5 text-left text-sm hover:opacity-80"
								style="color: var(--color-foreground)"
								onclick={() => { itemUrl = suggestedUrl; urlDropdownOpen = false; }}
							>
								{suggestedUrl}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		</div>
		<div class="col-span-2">
			<label for="item-permission-node" class="block text-sm font-medium" style="color: var(--color-foreground)">{t('menus.permission_node', 'Permission Node')}</label>
			<div class="mt-1">
				<Autocomplete
					id="item-permission-node"
					placeholder="content.view"
					emptyMessage={t('menus.permission_node_no_match', 'No matching permission nodes. Type a custom node.')}
					fetcher={permissionNodeFetcher}
					initialFetcher={permissionNodeInitialFetcher}
					selectedId={itemPermissionNode ? itemPermissionNode : null}
					selectedLabel={itemPermissionNode}
					onSelect={(item) => (itemPermissionNode = item.id)}
					onClear={() => (itemPermissionNode = '')}
					onInput={(q) => (itemPermissionNode = q)}
					freeText={true}
					inputClass="font-mono"
				/>
			</div>
			<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{t('menus.permission_node_hint', 'Optional. Restrict this item to users with the given permission node.')}</p>
		</div>
		{#if menuItems.length > 0}
			<div class="col-span-2">
				<label for="item-parent" class="block text-sm font-medium" style="color: var(--color-foreground)">{t('menus.parent', 'Parent')}</label>
				<Select
					options={[
						{ value: '', label: t('menus.no_parent', 'No parent') },
						...menuItems.filter((i) => !editingItem || i.id !== editingItem.id).map((item) => ({
							value: item.id,
							label: item.label
						}))
					]}
					value={itemParent}
					onchange={(val) => (itemParent = val)}
				/>
			</div>
		{/if}
		<div>
			<label for="item-visibility" class="block text-sm font-medium" style="color: var(--color-foreground)">{t('menus.visibility', 'Visibility')}</label>
			<Select
				options={[
					{ value: 'all', label: t('menus.visibility_all', 'All') },
					{ value: 'authenticated', label: t('menus.visibility_authenticated', 'Authenticated') },
					{ value: 'guest', label: t('menus.visibility_guest', 'Guest') }
				]}
				value={itemVisibility}
				onchange={(val) => (itemVisibility = val)}
			/>
		</div>
		<div>
			<label for="item-target" class="block text-sm font-medium" style="color: var(--color-foreground)">{t('menus.target', 'Target')}</label>
			<Select
				options={[
					{ value: '_self', label: t('menus.target_self', 'Same tab') },
					{ value: '_blank', label: t('menus.target_blank', 'New tab') }
				]}
				value={itemTarget}
				onchange={(val) => (itemTarget = val)}
			/>
		</div>
		<label class="block col-span-2">
			<span class="block text-sm font-medium" style="color: var(--color-foreground)">{t('menus.required_groups', 'Required User Groups')}</span>
			<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{t('menus.required_groups_hint', 'Item is visible only to users in at least one selected group. Leave empty for everyone.')}</p>
			<div class="mt-1 space-y-2">
				{#if selectedGroupObjs.length > 0}
					<div class="flex flex-wrap items-center gap-1">
						{#each selectedGroupObjs as group (group.id)}
							<span class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-foreground)">
								{group.name}
								<button
									type="button"
									class="ml-0.5 opacity-60 hover:opacity-100"
									style="color: var(--color-foreground)"
									onclick={() => removeGroup(group.id)}
									aria-label={t('common.remove', 'Remove')}
								>
									&times;
								</button>
							</span>
						{/each}
						{#if selectedGroupObjs.length > 0}
							<button
								type="button"
								class="text-xs underline"
								style="color: var(--color-muted)"
								onclick={clearGroupSelection}
							>
								{t('common.clear_all', 'Clear all')}
							</button>
						{/if}
					</div>
				{/if}
				<Autocomplete
					placeholder={t('menus.search_groups', 'Search groups...')}
					emptyMessage={t('menus.no_groups_match', 'No matching groups.')}
					fetcher={groupFetcher}
					initialFetcher={groupInitialFetcher}
					selectedId={null}
					onSelect={selectGroup}
				/>
			</div>
		</label>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showItemModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={saveItem} disabled={savingItem || !itemLabel.trim() || !itemUrl.trim()}>
			{savingItem ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
		</Button>
	</div>
</Modal>

<!-- Delete Item Confirm -->
<Modal bind:open={showDeleteItemConfirm} title={t('common.confirmDelete', 'Confirm Delete')}>
	<p class="text-sm" style="color: var(--color-foreground)">{t('menus.delete_item_confirm', 'Delete this menu item?')}</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteItemConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmDeleteItem}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View slide-over body: menu basic fields + read-only tree of its items -->
{#snippet MenuView({ row }: { row: Menu })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('menus.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('menus.slug', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('menus.placement', 'Placement')}</p>
				<div class="mt-1">
					<Badge>{t(`menus.placement_${row.placement}`, row.placement)}</Badge>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
				<div class="mt-1">
					<Badge variant={row.status === 'active' ? 'default' : 'secondary'}>{t(`common.status_${row.status}`, row.status)}</Badge>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('menus.items', 'Items')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.item_count ?? row.items?.length ?? 0}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.updated', 'Updated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
			</div>
		</div>

		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('menus.items', 'Items')}</p>
			<div class="mt-2">
				{#await menusAdminApi.get(row.id).then((r) => (r.ok ? ((r.data as Menu).items ?? []) : []))}
					<p class="text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
				{:then items}
					{#if items.length === 0}
						<p class="text-sm" style="color: var(--color-muted)">{t('menus.no_items', 'No items in this menu')}</p>
					{:else}
						{@render ItemTree({ nodes: buildItemTree(items), depth: 0 })}
					{/if}
				{:catch}
					<p class="text-sm" style="color: var(--color-danger)">{t('common.error', 'Failed to load items')}</p>
				{/await}
			</div>
		</div>
	</div>
{/snippet}

<!-- Recursive read-only item tree -->
{#snippet ItemTree({ nodes, depth }: { nodes: MenuItem[]; depth: number })}
	<ul class="space-y-1">
		{#each nodes.sort((a, b) => a.sort_order - b.sort_order) as node (node.id)}
			<li>
				<div class="flex items-center gap-2 rounded px-2 py-1" style="margin-left: {depth * 1}rem">
					{#if node.icon}
						<span class="text-xs" style="color: var(--color-muted)">{node.icon}</span>
					{/if}
					<span class="text-sm font-medium" style="color: var(--color-foreground)">{node.label}</span>
					<span class="truncate text-xs font-mono" style="color: var(--color-muted)">{node.url}</span>
				</div>
				{#if node.children && node.children.length > 0}
					{@render ItemTree({ nodes: node.children, depth: depth + 1 })}
				{/if}
			</li>
		{/each}
	</ul>
{/snippet}
