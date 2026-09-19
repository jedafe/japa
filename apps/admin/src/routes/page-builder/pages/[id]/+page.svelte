<script lang="ts">
	// Ref: japaconnect-engineering/sections/16-page-builder.md
	// Unified Page Builder editor — handles all page types (landing, about,
	// service, event, custom) on the shared @japaconnect/landing-blocks library.
	// dnd-kit drag-and-drop + live LandingRenderer preview + per-type content
	// editor + BlockStylePanel + motion/visibility settings + geo preview.
	import { page } from '$app/state';
	import { goto, beforeNavigate } from '$app/navigation';
	import { browser } from '$app/environment';
	import { onMount, untrack, type Component } from 'svelte';
	import { PUBLIC_JC_SITE_URL, PUBLIC_JC_API_URL } from '$env/static/public';
	import { api, pageBuilderAdminApiExtended } from '$lib/api';
	import { getAccessToken } from '$lib/api/token';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { DragDropProvider, DragOverlay } from '@dnd-kit/svelte';
	import { isSortable } from '@dnd-kit/svelte/sortable';
	import {
		LandingRenderer,
		blockRegistry,
		contentEditorRegistry,
		BlockStylePanel,
		getGeoTokens,
		resolveGeoCopy,
		setCustomBlockDefs,
		getCustomBlockMetas,
		getCustomBlockDef,
		setAssetBaseUrl,
		resolveAssetUrl,
		containsGuestBlocks,
		type Block,
		type BlockStyle,
		type MotionConfig,
		type BlockMeta,
		type GeoContext,
		type ContainerProps,
		type CustomBlockDef,
		type RuntimeBlockMeta,
		type VisibilityRules,
		type GuestHomeData,
		type GuestHomeStat,
		type GuestHomeCategory,
		type GuestHomeOpportunity,
		type GuestHomeTool,
		type GuestHomeStory,
		type GuestHomeDestination,
		type FetchContext,
	} from '@japaconnect/landing-blocks';
	import { DraggableBlockType, SortableSection, ContainerChildren, ResizeHandle, WidgetPicker } from '$lib/components/landing-editor';
	import { isContainerLike, isChildOnlyType, childrenOf, childSlotsOf, mapChildSlots, defaultSlotKey, getContainerLayout, getContainerColumns, seedAllContainerDefaults, seedBlockDefaults, defaultChildrenFor } from '$lib/utils/blocks';
	import { Button } from '$lib/components/ui/button';

	import { Tabs, Dialog } from 'bits-ui';
	import * as LucideIcons from 'lucide-svelte';
	import { ArrowLeft, Save, Plus, Search, LayoutTemplate, Rocket, Archive, PanelLeft, PanelRight, PanelTop, Maximize2, Minimize2, Settings } from 'lucide-svelte';
	import MediaPickerModal from '$lib/components/ui/MediaPickerModal.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	// Cast for DI — the editors expect Component<any>, MediaPickerModal has bindable props
	const MediaPicker = MediaPickerModal as unknown as Component;
	// WidgetPicker is used by the widget block content editor for live search.
	const WidgetPickerComponent = WidgetPicker as unknown as Component;

	const canManagePages = $derived(hasPermission('pages.manage'));
	const canPublishPages = $derived(hasPermission('pages.publish'));
	const countryUtils = useCountries('geo_code');

	interface Theme { id: string; name: string; is_default?: boolean }
	interface PageData {
		id: string; title: string; slug: string; status: string;
		page_type?: string; theme_id?: string; blocks: Block[] | string;
		description?: string | null; meta?: Record<string, unknown> | null;
	}

	let pageData = $state<{ id: string; title: string; slug: string; status: string; page_type: string; theme_id?: string } | null>(null);
	let blocks = $state<Block[]>([]);
	let themes = $state<Theme[]>([]);
	let guestHomePreview = $state<GuestHomeData | undefined>(undefined);
	let loading = $state(true);
	let saving = $state(false);
	let publishing = $state(false);
	let showPublishDialog = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let saveError = $state('');
	let publishError = $state('');
	let selectedBlockId = $state<string | null>(null);
	let paletteSearch = $state('');
	// Active header category: 'info' | 'preview' | 'actions'.
	let activeHeaderPanel = $state<'info' | 'preview' | 'actions'>('info');
	// Resizable pane widths (in px). Drag the handles between panes to adjust.
	let leftPaneWidth = $state(256);
	let rightPaneWidth = $state(384);
	// Palette height within the left pane (px). Drag the horizontal handle
	// between the palette and the sections list to adjust.
	let paletteHeight = $state(300);

	// Section visibility toggles.
	let showHeader = $state(true);
	let showLeftPane = $state(true);
	let showRightPane = $state(true);
	// Distraction-free mode hides everything except the center editor.
	let distractionFree = $state(false);

	function handlePaneResize(paneId: string, newSize: number): void {
		if (paneId === 'left') leftPaneWidth = newSize;
		else if (paneId === 'right') rightPaneWidth = newSize;
		else if (paneId === 'palette') paletteHeight = newSize;
	}

	function toggleDistractionFree(): void {
		distractionFree = !distractionFree;
		if (distractionFree) {
			showHeader = false;
			showLeftPane = false;
			showRightPane = false;
		} else {
			showHeader = true;
			showLeftPane = true;
			showRightPane = true;
		}
	}
	// Track which containers are expanded in the sections sidebar.
	let expandedContainers = $state<Set<string>>(new Set());
	// DB-driven custom block definitions fetched from the API.
	let customBlockDefs = $state<CustomBlockDef[]>([]);

	// Auto-expand a container-like block when it's selected so children are visible.
	$effect(() => {
		if (!selectedBlockId) return;
		const block = findBlockInTree(blocks, selectedBlockId);
		if (block && isContainerLike(block) && !expandedContainers.has(block.id)) {
			expandedContainers = new Set(expandedContainers).add(block.id);
		}
	});
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let activeDragData = $state<{ blockType?: string; meta?: BlockMeta; id?: string } | null>(null);

	// Preview controls
	let previewCountry = $state('global');
	let deviceMode = $state<'desktop' | 'tablet' | 'mobile'>('desktop');
	let activeTab = $state<'content' | 'style' | 'settings'>('content');
	let unsavedChanges = $state(false);
	let pageTitle = $state('');

	// Editable page metadata (Page Settings dialog — the list page has no
	// metadata edit form, so this is the only place these can be changed).
	let pageSlug = $state('');
	let pageDescription = $state('');
	let pageType = $state('custom');
	let seoTitle = $state('');
	let pageMeta = $state<Record<string, unknown>>({});
	let pageTypeOptions = $state<Array<{ value: string; label: string }>>([]);
	let showPageSettings = $state(false);

	// Transient palette hint (e.g. dropping a child-only block at top level).
	let paletteMessage = $state('');

	// Theme + save-as-template wiring
	let selectedThemeId = $state<string>('');
	let showSaveAsTemplate = $state(false);
	let templateName = $state('');
	let savingTemplate = $state(false);

	const selectedBlock = $derived(selectedBlockId ? findBlockInTree(blocks, selectedBlockId) : null);
	const visibleBlocks = $derived(blocks.filter((b) => b.visible));

	// Published pages render on the main site (apps/main). Block props and
	// preview data may carry root-relative asset paths (/brand/…, /logos/…)
	// that only exist on that host — the shared assetBase makes the block
	// defaults resolve them against the configured site URL, and
	// resolveSiteAssets rewrites data-provided URLs (story avatars, seeded
	// member images) so the editor preview doesn't 404 them.
	setAssetBaseUrl(PUBLIC_JC_SITE_URL);
	function resolveSiteAssets<T>(value: T): T {
		if (value == null) return value;
		if (typeof value === 'string') return resolveAssetUrl(value) as T;
		if (Array.isArray(value)) return value.map(resolveSiteAssets) as T;
		if (typeof value === 'object') {
			const out: Record<string, unknown> = {};
			for (const [k, v] of Object.entries(value as Record<string, unknown>)) out[k] = resolveSiteAssets(v);
			return out as T;
		}
		return value;
	}

	const previewCtx: GeoContext = $derived({
		contentCountry: previewCountry,
		tokens: getGeoTokens(previewCountry, 'en'),
		locale: 'en',
	});

	// Fetch context for data-driven blocks (widget, entity_collection,
	// entity_feature, live_rate_ticker, map). Without it they render their
	// empty state, so the preview diverges from the published page.
	const fetchCtx: FetchContext | undefined = $derived(
		browser ? { fetch, apiBase: PUBLIC_JC_API_URL, token: getAccessToken() } : undefined
	);
	const resolvedBlocks = $derived(resolveSiteAssets(resolveGeoCopy(visibleBlocks, previewCtx)));
	const deviceWidth = $derived(deviceMode === 'desktop' ? 1280 : deviceMode === 'tablet' ? 768 : 375);

	// Block palette — grouped by category from the canonical blockRegistry,
	// merged with DB-driven custom block definitions.
	// Custom blocks appear as `custom:{blockDefId}` entries in the palette so
	// each one is draggable independently. `makeBlock` parses this prefix.
	const filteredBlockTypes = $derived.by(() => {
		const q = paletteSearch.toLowerCase().trim();
		// Static blocks
		const staticEntries = Object.entries(blockRegistry).filter(([type]) => type !== 'custom') as [string, BlockMeta][];
		// Custom blocks from the DB — each gets a synthetic palette key.
		const customEntries: [string, BlockMeta][] = customBlockDefs.map((def) => [
			`custom:${def.id}`,
			{ component: blockRegistry.custom.component, icon: 'Puzzle', label: def.name, category: def.category || 'custom' },
		]);
		const all = [...staticEntries, ...customEntries];
		if (!q) return all;
		return all.filter(([type, meta]) => meta.label.toLowerCase().includes(q) || type.toLowerCase().includes(q));
	});
	const groupedBlockTypes = $derived.by(() => {
		const groups: Record<string, [string, BlockMeta][]> = {};
		for (const [type, meta] of filteredBlockTypes) (groups[meta.category] ??= []).push([type, meta]);
		return groups;
	});

	// Content editor component for the selected block.
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const ContentEditor = $derived(selectedBlock ? (contentEditorRegistry as Record<string, Component<any>>)[selectedBlock.type] : null);

	// parseBlocks normalises the blocks payload (JSON string or parsed array) into Block[].
	function parseBlocks(raw: Block[] | string): Block[] {
		if (Array.isArray(raw)) return raw as Block[];
		if (typeof raw === 'string') {
			try {
				const parsed = JSON.parse(raw);
				return Array.isArray(parsed) ? (parsed as Block[]) : [];
			} catch { return []; }
		}
		return [];
	}

	/** Recursively collect all container-like ids under any `guest_*` container block. */
	function collectGuestContainerIds(tree: Block[]): Set<string> {
		const ids = new Set<string>();
		function collect(b: Block) {
			if (!isContainerLike(b)) return;
			ids.add(b.id);
			for (const child of childrenOf(b)) collect(child);
		}
		function walk(b: Block) {
			if (b.type.startsWith('guest_') && isContainerLike(b)) {
				collect(b);
			} else if (isContainerLike(b)) {
				for (const child of childrenOf(b)) walk(child);
			}
		}
		for (const b of tree) walk(b);
		return ids;
	}

	// Track only the route param — reactive reads inside load() (e.g. the
	// shared countries store's `loading`/`countries` $state) must not re-fire
	// this effect, or every state flip duplicates the page/themes/blocks
	// fetches (observed 3× per mount).
	$effect(() => {
		page.params.id;
		untrack(() => { void load(); });
	});

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const pageId = page.params.id;
			// Pre-load the 250 primary geo space countries via the shared utility
			// while the page data loads.
			countryUtils.load();
			const [pageRes, themesRes, blocksRes, typesRes] = await Promise.all([
				api<PageData>(`/admin/pages/${pageId}`),
				// page_size=100 — the default page size is 20, which would
				// silently truncate the theme dropdown and custom block palette.
				pageBuilderAdminApiExtended.listThemes('page_size=100'),
				pageBuilderAdminApiExtended.listBlocks('page_size=100'),
				pageBuilderAdminApiExtended.getPageTypes(),
			]);
			if (!pageRes.ok) {
				errorMessage = pageRes.error || t('common.error', 'Failed to load page');
			} else if (pageRes.data) {
				const d = pageRes.data;
				pageData = { id: d.id, title: d.title, slug: d.slug, status: d.status, page_type: d.page_type ?? 'custom', theme_id: d.theme_id };
				// Seed the editor's working copy once, so subsequent server-side
				// changes don't clobber in-progress edits.
				blocks = untrack(() => seedAllContainerDefaults(parseBlocks(d.blocks)));
				// Default-expand any Guest Home block and its container descendants.
				expandedContainers = new Set([...expandedContainers, ...collectGuestContainerIds(blocks)]);
				pageTitle = untrack(() => d.title);
				pageSlug = untrack(() => d.slug);
				pageDescription = d.description ?? '';
				pageType = d.page_type ?? 'custom';
				const meta = (d.meta ?? {}) as Record<string, unknown>;
				pageMeta = meta;
				seoTitle = typeof meta.title === 'string' ? meta.title : '';
				selectedThemeId = d.theme_id ?? '';
				savedFingerprint = saveFingerprint();
				// Live preview data for the guest homepage block.
				void loadGuestHome(blocks);
			}
			if (themesRes.ok) themes = ((themesRes.data as any[]) ?? []).map((th) => ({ id: th.id, name: th.name, is_default: th.is_default }));
			if (typesRes.ok && Array.isArray(typesRes.data)) pageTypeOptions = typesRes.data;
			// Load custom block definitions into the runtime registry.
			if (blocksRes.ok && Array.isArray(blocksRes.data)) {
				customBlockDefs = blocksRes.data as CustomBlockDef[];
				setCustomBlockDefs(customBlockDefs);
			}
		} catch {
			errorMessage = t('common.error', 'Failed to load page');
		} finally {
			loading = false;
		}
	}

	async function loadGuestHome(blockList: Block[]) {
		// Any guest_* block (not just guest_home) can consume context data in
		// the preview — stats, categories, opportunities, stories, tools.
		if (!containsGuestBlocks(blockList)) {
			guestHomePreview = undefined;
			return;
		}

		const settingsRes = await api<Record<string, unknown>>('/site/settings/public');
		if (!settingsRes.ok) {
			guestHomePreview = undefined;
			return;
		}
		const settings = settingsRes.data ?? {};

		const stats = asArray<GuestHomeStat>(settings['landing.stats']);
		const categories = asArray<GuestHomeCategory>(settings['landing.categories']);
		const opportunities = asArray<GuestHomeOpportunity>(settings['landing.featured_opportunities']);
		const tools = asArray<GuestHomeTool>(settings['landing.tools']);
		let topDestinations = asArray<GuestHomeDestination>(settings['landing.top_destinations']);

		// If no curated top destinations are configured, try the directory API.
		if (topDestinations.length === 0) {
			try {
				const spaceRes = await api<GuestHomeDestination[]>('/spaces?level=country&sort=members&page_size=4');
				if (spaceRes.ok) topDestinations = (spaceRes.data ?? []).slice(0, 4);
			} catch {
				// Ignore — preview degrades to empty list.
			}
		}

		let stories: GuestHomeStory[] = [];
		try {
			const storyRes = await api<Record<string, unknown>[]>('/stories?limit=3&sort=top');
			if (storyRes.ok) {
				stories = (storyRes.data ?? []).slice(0, 3).map(mapStoryToGuestHome);
			}
		} catch {
			// Ignore — preview degrades to empty list.
		}

		guestHomePreview = resolveSiteAssets({
			topDestinations,
			opportunities,
			stories,
			tools,
			stats,
			categories
		});
	}

	function asArray<T>(value: unknown): T[] {
		if (Array.isArray(value)) return value as T[];
		if (typeof value === 'string') {
			try {
				const parsed = JSON.parse(value);
				if (Array.isArray(parsed)) return parsed as T[];
			} catch {
				return [];
			}
		}
		if (value && typeof value === 'object' && !Array.isArray(value)) {
			const maybe = (value as Record<string, unknown>).data;
			if (Array.isArray(maybe)) return maybe as T[];
		}
		return [];
	}

	function mapStoryToGuestHome(story: Record<string, unknown>): GuestHomeStory {
		const bestAdvice = typeof story.best_advice === 'string' ? story.best_advice.trim() : '';
		const mostUsefulResource = typeof story.most_useful_resource === 'string' ? story.most_useful_resource.trim() : '';
		const rawBody = typeof story.body === 'string' ? story.body.replace(/<[^>]+>/g, '').slice(0, 160) : '';
		const title = typeof story.title === 'string' ? story.title : '';
		const quote = bestAdvice || mostUsefulResource || rawBody || title;

		const profession = typeof story.profession === 'string' ? story.profession : '';
		const destinationSpaceName = typeof story.destination_space_name === 'string' ? story.destination_space_name : '';
		const role = [profession, destinationSpaceName].filter(Boolean).join(' • ');

		return {
			id: typeof story.id === 'string' ? story.id : '',
			slug: typeof story.slug === 'string' ? story.slug : undefined,
			title,
			quote,
			author_name: typeof story.author_display_name === 'string' ? story.author_display_name : (typeof story.author_name === 'string' ? story.author_name : 'A JapaConnect Member'),
			author_role: role || undefined,
			author_avatar_url: (typeof story.author_avatar === 'string' ? story.author_avatar : (typeof story.featured_image_url === 'string' ? story.featured_image_url : null)) as string | null,
			destination: destinationSpaceName || undefined
		};
	}

	function getIcon(name: string): Component | null {
		const found = (LucideIcons as Record<string, unknown>)[name];
		return (found as Component) ?? null;
	}

	// ─── Block CRUD ───────────────────────────────────────────────
	// `type` can be a real Block type or a `custom:{blockDefId}` palette key.
	function makeBlock(type: string): Block {
		// Seed nested container defaults so every guest block arrives with its
		// editable children already in place (or absent for context-driven lists).
		return seedBlockDefaults(buildBlock(type));
	}
	function buildBlock(type: string): Block {
		// Custom block — parse the block def ID and seed props from defaults.
		if (type.startsWith('custom:')) {
			const blockDefId = type.slice('custom:'.length);
			const def = getCustomBlockDef(blockDefId);
			const defaultProps = def?.default_config ?? {};
			// Also apply field-level defaults from the schema.
			const schemaDefaults: Record<string, unknown> = {};
			for (const field of def?.config_schema?.fields ?? []) {
				if (field.default !== undefined) schemaDefaults[field.key] = field.default;
			}
			return {
				id: crypto.randomUUID(),
				type: 'custom',
				visible: true,
				props: { ...schemaDefaults, ...defaultProps },
				block_def_id: blockDefId,
			} as Block;
		}
		// Container blocks need sensible defaults so the editor and renderer
		// don't crash on an empty children array.
		if (type === 'container') {
			return { id: crypto.randomUUID(), type: 'container', visible: true, props: { layout: 'grid', columns: 2, gap: 'md', children: [] } } as Block;
		}
		// Widget blocks need defaults for layout/columns so the renderer
		// doesn't crash on an empty widget_id.
		if (type === 'widget') {
			return { id: crypto.randomUUID(), type: 'widget', visible: true, props: { widget_id: '', layout: 'grid', columns: 3, heading: '', view_all_href: '' } } as Block;
		}
		if (type === 'guest_home') {
			const id = crypto.randomUUID();
			return {
				id,
				type: 'guest_home',
				visible: true,
				label: 'Guest Home',
				props: {
					layout: 'stack',
					children: defaultChildrenFor('guest_home') ?? []
				}
			} as Block;
		}
		if (type === 'guest_hero') {
			const id = crypto.randomUUID();
			return {
				id,
				type: 'guest_hero',
				visible: true,
				label: 'Hero',
				props: {
					layout: 'grid',
					columns: 2,
					responsive_columns: { tablet: 1, mobile: 1 },
					gap: 'md',
					children: defaultChildrenFor('guest_hero') ?? []
				}
			} as Block;
		}
		if (type === 'guest_hero_image') {
			const id = crypto.randomUUID();
			return {
				id,
				type: 'guest_hero_image',
				visible: true,
				label: 'Hero Image',
				props: {
					children: defaultChildrenFor('guest_hero_image') ?? []
				}
			} as Block;
		}
		return { id: crypto.randomUUID(), type: type as Block['type'], visible: true, props: {} } as Block;
	}
	function addBlock(type: string): void {
		// Child-only guest blocks (footer links, hero items, …) only render
		// inside their parent container — never as top-level sections.
		if (isChildOnlyType(type)) {
			paletteMessage = t('pageBuilder.child_only_block', 'This block only works inside a guest container — drag it onto a container instead.');
			return;
		}
		paletteMessage = '';
		const nb = makeBlock(type);
		blocks = [...blocks, nb];
		selectedBlockId = nb.id;
		if (isContainerLike(nb)) {
			expandedContainers = new Set([...expandedContainers, nb.id, ...collectGuestContainerIds([nb])]);
		}
		unsavedChanges = true;
	}
	function removeBlock(id: string): void {
		// Try top-level first, then search nested containers.
		const topLevel = blocks.find((b) => b.id === id);
		if (topLevel) {
			blocks = blocks.filter((b) => b.id !== id);
		} else {
			const { tree } = removeBlockFromTree(blocks, id);
			blocks = tree;
		}
		if (selectedBlockId === id) selectedBlockId = null;
		unsavedChanges = true;
	}
	/** Regenerate ids recursively so duplicated container subtrees don't share
	 *  child ids with the source (all tree ops match on id). */
	function regenBlockIds(b: Block): Block {
		const withNewId = {
			...b,
			id: crypto.randomUUID(),
			props: b.props ? JSON.parse(JSON.stringify(b.props)) : b.props,
		} as Block;
		if (!isContainerLike(withNewId)) return withNewId;
		return mapChildSlots(withNewId, (children) => children.map(regenBlockIds));
	}
	/** Insert `copy` immediately after the block with `id`, at whatever depth
	 *  and in whichever child slot that block lives. */
	function insertAfterInTree(tree: Block[], id: string, copy: Block): Block[] {
		return tree.flatMap((b) => {
			if (b.id === id) return [b, copy];
			if (isContainerLike(b)) {
				return [mapChildSlots(b, (children) => insertAfterInTree(children, id, copy))];
			}
			return [b];
		});
	}
	function duplicateBlock(id: string): void {
		const block = findBlockInTree(blocks, id);
		if (!block) return;
		const copy = regenBlockIds(block);
		blocks = insertAfterInTree(blocks, id, copy);
		selectedBlockId = copy.id;
		unsavedChanges = true;
	}
	function toggleBlockVisibility(id: string): void {
		blocks = updateBlockInTree(blocks, id, (b) => ({ ...b, visible: !b.visible }));
		unsavedChanges = true;
	}
	function reorderBlocks(fromIdx: number, toIdx: number): void {
		if (fromIdx === toIdx) return;
		const next = [...blocks];
		const [moved] = next.splice(fromIdx, 1);
		next.splice(toIdx, 0, moved);
		blocks = next;
		unsavedChanges = true;
	}

	// ─── Container tree manipulation ──────────────────────────────
	// These helpers walk the block tree (top-level array + nested
	// container children) to find, remove, and insert blocks anywhere.

	/** Find a block anywhere in the tree (top-level or nested in containers). */
	function findBlockInTree(tree: Block[], id: string): Block | null {
		for (const b of tree) {
			if (b.id === id) return b;
			if (isContainerLike(b)) {
				const found = findBlockInTree(childrenOf(b), id);
				if (found) return found;
			}
		}
		return null;
	}

	/** Apply a transform to a block anywhere in the tree (top-level or nested). */
	function updateBlockInTree(tree: Block[], id: string, fn: (b: Block) => Block): Block[] {
		return tree.map((b) => {
			if (b.id === id) return fn(b);
			if (isContainerLike(b)) {
				return mapChildSlots(b, (children) => updateBlockInTree(children, id, fn));
			}
			return b;
		});
	}

	/** Remove a block from anywhere in the tree. Returns the new tree and the removed block. */
	function removeBlockFromTree(tree: Block[], id: string): { tree: Block[]; removed: Block | null } {
		for (let i = 0; i < tree.length; i++) {
			if (tree[i].id === id) {
				return { tree: [...tree.slice(0, i), ...tree.slice(i + 1)], removed: tree[i] };
			}
			if (isContainerLike(tree[i])) {
				let removed: Block | null = null;
				const newBlock = mapChildSlots(tree[i], (children) => {
					if (removed) return children;
					const result = removeBlockFromTree(children, id);
					if (result.removed) removed = result.removed;
					return result.tree;
				});
				if (removed) {
					const newTree = [...tree];
					newTree[i] = newBlock;
					return { tree: newTree, removed };
				}
			}
		}
		return { tree, removed: null };
	}

	/** Insert a block into a container's child slot (found by ID, anywhere in the tree). */
	function insertIntoContainer(tree: Block[], containerId: string, slotKey: string, block: Block): Block[] {
		return tree.map((b) => {
			if (b.id === containerId && isContainerLike(b)) {
				const target = slotKey || defaultSlotKey(b);
				return mapChildSlots(b, (children, slot) => (slot.key === target ? [...children, block] : children));
			}
			if (isContainerLike(b)) {
				return mapChildSlots(b, (children) => insertIntoContainer(children, containerId, slotKey, block));
			}
			return b;
		});
	}

	/** Toggle container expansion in the sections sidebar. */
	function toggleContainerExpand(id: string): void {
		const next = new Set(expandedContainers);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		expandedContainers = next;
	}

	/** Reorder a child within a container slot (by container ID + slot + indices). */
	function reorderContainerChild(containerId: string, slotKey: string, fromIdx: number, toIdx: number): void {
		if (fromIdx === toIdx) return;
		blocks = updateBlockInTree(blocks, containerId, (b) => {
			if (!isContainerLike(b)) return b;
			return mapChildSlots(b, (children, slot) => {
				if (slot.key !== slotKey || toIdx < 0 || toIdx >= children.length) return children;
				const next = [...children];
				const [moved] = next.splice(fromIdx, 1);
				next.splice(toIdx, 0, moved);
				return next;
			});
		});
		unsavedChanges = true;
	}

	/** Reorder a child by ID within a container slot (by container ID + slot + child IDs). */
	function reorderContainerChildByIds(containerId: string, slotKey: string, fromChildId: string, toChildId: string): void {
		blocks = updateBlockInTree(blocks, containerId, (b) => {
			if (!isContainerLike(b)) return b;
			return mapChildSlots(b, (children, slot) => {
				if (slot.key !== slotKey) return children;
				const fromIdx = children.findIndex((c) => c.id === fromChildId);
				const toIdx = children.findIndex((c) => c.id === toChildId);
				if (fromIdx < 0 || toIdx < 0 || fromIdx === toIdx) return children;
				const next = [...children];
				const [moved] = next.splice(fromIdx, 1);
				next.splice(toIdx, 0, moved);
				return next;
			});
		});
		unsavedChanges = true;
	}

	/** Insert a block into a container slot at a specific index (before the target child). */
	function insertIntoContainerAtIndex(containerId: string, slotKey: string, block: Block, beforeChildId: string | null): void {
		blocks = updateBlockInTree(blocks, containerId, (b) => {
			if (!isContainerLike(b)) return b;
			return mapChildSlots(b, (children, slot) => {
				if (slot.key !== slotKey) return children;
				const next = [...children];
				if (beforeChildId) {
					const idx = next.findIndex((c) => c.id === beforeChildId);
					if (idx >= 0) next.splice(idx, 0, block);
					else next.push(block);
				} else {
					next.push(block);
				}
				return next;
			});
		});
	}

	/** Remove a child from a container slot. */
	function removeContainerChild(containerId: string, slotKey: string, childId: string): void {
		blocks = updateBlockInTree(blocks, containerId, (b) => {
			if (!isContainerLike(b)) return b;
			return mapChildSlots(b, (children, slot) => (slot.key === slotKey ? children.filter((c) => c.id !== childId) : children));
		});
		if (selectedBlockId === childId) selectedBlockId = null;
		unsavedChanges = true;
	}

	/** Move a child out of a container to the top-level blocks array. */
	function moveChildToTopLevel(_containerId: string, _slotKey: string, childId: string): void {
		const { tree, removed } = removeBlockFromTree(blocks, childId);
		if (!removed) return;
		blocks = [...tree, removed];
		if (selectedBlockId === childId) selectedBlockId = null;
		unsavedChanges = true;
	}

	/** Update a child's grid column span (1–12). */
	function updateChildSpan(_containerId: string, _slotKey: string, childId: string, span: number): void {
		const clamped = Math.max(1, Math.min(12, span));
		blocks = updateBlockInTree(blocks, childId, (b) => ({ ...b, span: clamped }));
		unsavedChanges = true;
	}
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function updateBlockProps(id: string, props: any): void {
		blocks = updateBlockInTree(blocks, id, (b) => ({ ...b, props } as Block));
		unsavedChanges = true;
	}
	function updateBlockStyle(id: string, style: BlockStyle): void {
		blocks = updateBlockInTree(blocks, id, (b) => ({ ...b, style } as Block));
		unsavedChanges = true;
	}
	/** Merge a motion patch into a block anywhere in the tree. */
	function updateBlockMotion(id: string, patch: Partial<MotionConfig> | null): void {
		blocks = updateBlockInTree(blocks, id, (b) =>
			patch === null
				? ({ ...b, motion: undefined } as Block)
				: ({ ...b, motion: { ...(b.motion ?? {}), ...patch } } as Block)
		);
		unsavedChanges = true;
	}
	/** Toggle per-device visibility for a block (desktop/tablet/mobile). */
	function updateResponsiveVisibility(id: string, device: 'desktop' | 'tablet' | 'mobile', visible: boolean): void {
		blocks = updateBlockInTree(blocks, id, (b) => {
			const rv = { ...(b.responsive_visibility ?? {}) };
			if (visible) {
				// Setting to visible = remove the override (fall back to default visible)
				delete rv[device];
			} else {
				rv[device] = false;
			}
			const hasKeys = Object.keys(rv).length > 0;
			return { ...b, responsive_visibility: hasKeys ? rv : undefined } as Block;
		});
		unsavedChanges = true;
	}

	/** Update the `hide_if_empty` visibility rule for a block. */
	function updateVisibilityRuleHideIfEmpty(id: string, checked: boolean): void {
		blocks = updateBlockInTree(blocks, id, (b) => {
			const rules = { ...(b.visibility_rules ?? {}) } as VisibilityRules;
			if (checked) {
				rules.hide_if_empty = true;
			} else {
				delete rules.hide_if_empty;
			}
			const hasKeys = Object.keys(rules).length > 0;
			return { ...b, visibility_rules: hasKeys ? rules : undefined } as Block;
		});
		unsavedChanges = true;
	}

	/** Toggle a country code in the `geo_countries` visibility rule. */
	function updateVisibilityRuleGeo(id: string, geoCode: string, checked: boolean): void {
		blocks = updateBlockInTree(blocks, id, (b) => {
			const rules = { ...(b.visibility_rules ?? {}) } as VisibilityRules;
			const set = new Set(rules.geo_countries ?? []);
			if (checked) {
				set.add(geoCode);
			} else {
				set.delete(geoCode);
			}
			const list = Array.from(set).sort();
			if (list.length > 0) {
				rules.geo_countries = list;
			} else {
				delete rules.geo_countries;
			}
			const hasKeys = Object.keys(rules).length > 0;
			return { ...b, visibility_rules: hasKeys ? rules : undefined } as Block;
		});
		unsavedChanges = true;
	}

	// ─── dnd-kit handlers ─────────────────────────────────────────
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function handleDragStart(event: any) {
		activeDragData = event?.operation?.source?.data ?? null;
	}
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function handleDragEnd(event: any) {
		activeDragData = null;
		if (event?.canceled) return;
		const { source, target } = event.operation ?? {};
		if (!source) return;
		const sourceData = source.data;
		const targetData = target?.data;

		// Drop onto a container drop zone → insert into that container slot (append).
		if (targetData?.source === 'container-dropzone') {
			const containerId = targetData.containerId as string;
			const slotKey = (targetData.slotKey as string) ?? '';
			if (sourceData?.source === 'palette') {
				const nb = makeBlock(sourceData.blockType as string);
				blocks = insertIntoContainer(blocks, containerId, slotKey, nb);
				selectedBlockId = nb.id;
				expandedContainers = new Set(expandedContainers).add(containerId);
				unsavedChanges = true;
				return;
			}
			if (sourceData?.source === 'canvas' || sourceData?.source === 'container-child') {
				const { tree: newTree, removed } = removeBlockFromTree(blocks, source.id as string);
				if (!removed) return;
				blocks = insertIntoContainer(newTree, containerId, slotKey, removed);
				selectedBlockId = removed.id;
				expandedContainers = new Set(expandedContainers).add(containerId);
				unsavedChanges = true;
				return;
			}
		}

		// Drop onto a container child (sortable target) → reorder or insert.
		if (targetData?.source === 'container-child' && target && isSortable(target)) {
			const targetContainerId = targetData.containerId as string;
			const targetSlotKey = (targetData.slotKey as string) ?? '';
			const targetChildId = target.id as string;

			// Palette → insert before the target child.
			if (sourceData?.source === 'palette') {
				const nb = makeBlock(sourceData.blockType as string);
				insertIntoContainerAtIndex(targetContainerId, targetSlotKey, nb, targetChildId);
				selectedBlockId = nb.id;
				expandedContainers = new Set(expandedContainers).add(targetContainerId);
				unsavedChanges = true;
				return;
			}

			// Container child → reorder within the same slot.
			if (sourceData?.source === 'container-child' && sourceData.containerId === targetContainerId && sourceData.slotKey === targetSlotKey) {
				reorderContainerChildByIds(targetContainerId, targetSlotKey, source.id as string, targetChildId);
				return;
			}

			// Canvas section → move into container slot before target child.
			if (sourceData?.source === 'canvas') {
				const { tree: newTree, removed } = removeBlockFromTree(blocks, source.id as string);
				if (!removed) return;
				blocks = newTree;
				insertIntoContainerAtIndex(targetContainerId, targetSlotKey, removed, targetChildId);
				selectedBlockId = removed.id;
				expandedContainers = new Set(expandedContainers).add(targetContainerId);
				unsavedChanges = true;
				return;
			}

			// Container child from a different slot/container → move before target child.
			if (sourceData?.source === 'container-child' && (sourceData.containerId !== targetContainerId || sourceData.slotKey !== targetSlotKey)) {
				const { tree: newTree, removed } = removeBlockFromTree(blocks, source.id as string);
				if (!removed) return;
				blocks = newTree;
				insertIntoContainerAtIndex(targetContainerId, targetSlotKey, removed, targetChildId);
				selectedBlockId = removed.id;
				expandedContainers = new Set(expandedContainers).add(targetContainerId);
				unsavedChanges = true;
				return;
			}
		}

		// Drop from palette → add new block at target position (or append).
		if (sourceData?.source === 'palette') {
			const paletteType = sourceData.blockType as string;
			// Child-only guest blocks can't be top-level sections — they must
			// land in a container drop zone instead.
			if (isChildOnlyType(paletteType)) {
				paletteMessage = t('pageBuilder.child_only_block', 'This block only works inside a guest container — drag it onto a container instead.');
				return;
			}
			paletteMessage = '';
			const nb = makeBlock(paletteType);
			if (target && isSortable(target)) {
				const targetIdx = blocks.findIndex((b) => b.id === target.id);
				if (targetIdx >= 0) blocks = [...blocks.slice(0, targetIdx), nb, ...blocks.slice(targetIdx)];
				else blocks = [...blocks, nb];
			} else {
				blocks = [...blocks, nb];
			}
			selectedBlockId = nb.id;
			unsavedChanges = true;
			return;
		}
		// Reorder within the top-level sections list.
		if (sourceData?.source === 'canvas' && target && isSortable(target)) {
			const fromIdx = blocks.findIndex((b) => b.id === source.id);
			const toIdx = blocks.findIndex((b) => b.id === target.id);
			if (fromIdx < 0 || toIdx < 0) return;
			reorderBlocks(fromIdx, toIdx);
		}
	}

	// ─── Save / Publish ───────────────────────────────────────────
	async function saveDraft(): Promise<boolean> {
		if (!pageData) return false;
		saving = true;
		saveError = '';
		// When nothing changed but block order, persist via the dedicated
		// reorder endpoint (PATCH /admin/pages/{id}/blocks/reorder) instead of
		// a full-page PATCH. The fingerprint sorts block serialisations so a
		// pure reorder produces an identical fingerprint.
		const pureReorder = savedFingerprint !== '' && saveFingerprint() === savedFingerprint;
		// Merge the SEO title into meta, preserving any other stored keys.
		const meta: Record<string, unknown> = { ...pageMeta };
		if (seoTitle.trim()) meta.title = seoTitle.trim(); else delete meta.title;
		const body: Record<string, unknown> = pureReorder
			? { blocks }
			: {
					title: pageTitle,
					slug: pageSlug || pageData.slug,
					description: pageDescription,
					page_type: pageType || pageData.page_type,
					status: pageData.status,
					meta,
					// Send blocks as a parsed array — api() already JSON.stringifies
					// the body. Double-encoding (JSON.stringify here) produced a JSON
					// string that the backend's json.RawMessage + sanitizeBlocksJSON
					// mishandled, silently emptying the page on save.
					blocks,
				};
		if (!pureReorder && selectedThemeId) body.theme_id = selectedThemeId;
		const endpoint = pureReorder
			? `/admin/pages/${pageData.id}/blocks/reorder`
			: `/admin/pages/${pageData.id}`;
		const result = await api(endpoint, { method: 'PATCH', body });
		saving = false;
		if (result.ok) {
			const d = result.data as PageData | undefined;
			if (d?.slug) {
				pageData = { ...pageData, slug: d.slug, page_type: d.page_type ?? pageData.page_type };
				pageSlug = d.slug;
			}
			pageMeta = meta;
			unsavedChanges = false;
			savedFingerprint = saveFingerprint();
			return true;
		}
		saveError = result.error ?? t('pageBuilder.save_failed', 'Failed to save draft');
		return false;
	}

	// Fingerprint of the last-saved saveable state (title + theme + block
	// contents, order-insensitive). Used by saveDraft() to detect pure block
	// reorders.
	let savedFingerprint = $state('');

	function saveFingerprint(): string {
		const blockKey = blocks.map((b) => JSON.stringify(b)).sort().join(' ');
		return `${pageTitle} ${pageSlug} ${pageDescription} ${pageType} ${seoTitle} ${selectedThemeId} ${blockKey}`;
	}

	async function publish(): Promise<void> {
		publishError = '';
		const saved = await saveDraft();
		if (!saved) { publishError = saveError || t('pageBuilder.save_failed', 'Failed to save draft'); return; }
		if (!pageData) return;
		publishing = true;
		const result = await api(`/admin/pages/${pageData.id}/publish`, { method: 'POST' });
		publishing = false;
		if (result.ok) {
			showPublishDialog = false;
			pageData = { ...pageData, status: 'published' };
			successMessage = t('pageBuilder.published', 'Published!');
			setTimeout(() => (successMessage = ''), 2500);
			return;
		}
		publishError = result.error ?? t('pageBuilder.publish_failed', 'Failed to publish page');
	}

	async function archive(): Promise<void> {
		if (!pageData) return;
		const result = await api(`/admin/pages/${pageData.id}/archive`, { method: 'POST' });
		if (result.ok) {
			pageData = { ...pageData, status: 'archived' };
			successMessage = t('pageBuilder.archived', 'Archived');
			setTimeout(() => (successMessage = ''), 2500);
		} else {
			errorMessage = result.error ?? t('common.error', 'Failed to archive page');
		}
	}

	async function changeTheme(themeId: string): Promise<void> {
		if (!pageData) return;
		selectedThemeId = themeId;
		unsavedChanges = true;
	}

	async function saveAsTemplate(): Promise<void> {
		if (!pageData || !templateName.trim()) return;
		savingTemplate = true;
		try {
			const result = await api(`/admin/pages/${pageData.id}/save-as-template`, { method: 'POST', body: { name: templateName.trim() } });
			if (result.ok) {
				showSaveAsTemplate = false;
				templateName = '';
				successMessage = t('pageBuilder.templateSaved', 'Saved as template!');
				setTimeout(() => (successMessage = ''), 3000);
			} else {
				errorMessage = result.error ?? t('common.error', 'Failed to save template');
			}
		} catch {
			errorMessage = t('common.error', 'Failed to save template');
		} finally {
			savingTemplate = false;
		}
	}

	// Warn before navigating away with unsaved edits — covers both full-page
	// unloads (beforeunload) and in-app SvelteKit navigation (beforeNavigate).
	$effect(() => {
		if (!unsavedChanges) return;
		const handler = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = ''; };
		window.addEventListener('beforeunload', handler);
		return () => window.removeEventListener('beforeunload', handler);
	});

	beforeNavigate((nav) => {
		if (!unsavedChanges) return;
		if (!window.confirm(t('pageBuilder.leave_unsaved', 'You have unsaved changes. Leave anyway?'))) {
			nav.cancel();
		}
	});
</script>

<svelte:head>
	<title>{pageData?.title ?? t('page_builder.page_editor', 'Page Editor')} — JapaConnect Admin</title>
</svelte:head>

{#if loading}
	<div class="flex h-[calc(100vh-4rem)] items-center justify-center">
		<p style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
	</div>
{:else if errorMessage && !pageData}
	<div class="flex h-[calc(100vh-4rem)] flex-col items-center justify-center gap-4">
		<p class="text-sm" style="color: var(--color-danger)">{errorMessage}</p>
		<Button variant="ghost" onclick={() => goto('/page-builder/pages')}><ArrowLeft class="mr-2 h-4 w-4" />{t('common.back', 'Back')}</Button>
	</div>
{:else}
<div class="relative flex h-screen flex-col overflow-hidden">
	<!-- Floating restore tab for hidden header -->
	{#if !showHeader}
		<button
			type="button"
			onclick={() => (showHeader = true)}
			class="absolute left-1/2 top-0 z-50 -translate-x-1/2 rounded-b-md border border-t-0 px-3 py-0.5 text-[10px] font-medium transition-colors hover:bg-[var(--color-surface)]"
			style="border-color: var(--color-border); background: var(--color-background); color: var(--color-muted)"
			aria-label="Show top bar"
		><PanelTop class="inline h-3 w-3" /> Show top bar</button>
	{/if}
	<!-- Top bar — single row with 3 zones:
	     Left: back button.
	     Center: active category controls (switched by the right-side toggle).
	     Right: 3-button segmented switch + section toggles + distraction-free. -->
	{#if showHeader}
	<header class="flex items-center gap-3 border-b px-4 py-2" style="border-color: var(--color-border); background: var(--color-background)">
		<!-- Left: back button -->
		<Button variant="ghost" size="sm" onclick={() => goto('/page-builder/pages')} class="shrink-0"><ArrowLeft class="mr-1 h-4 w-4" />{t('common.back', 'Back')}</Button>

		<!-- Center: active category controls -->
		<div class="flex flex-1 items-center justify-center gap-2 overflow-hidden">
			{#if activeHeaderPanel === 'info'}
				<!-- Info: title + metadata + status -->
				<input type="text" bind:value={pageTitle} oninput={() => (unsavedChanges = true)} class="w-48 rounded border px-2 py-1 text-sm font-medium" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" aria-label={t('page_builder.title', 'Title')} />
				<button type="button" onclick={() => (showPageSettings = true)} class="shrink-0 rounded p-1 transition-colors hover:bg-[var(--color-surface)]" title={t('page_builder.page_settings', 'Page settings')} aria-label={t('page_builder.page_settings', 'Page settings')}><Settings class="h-4 w-4" style="color: var(--color-muted)" /></button>
				{#if pageData?.page_type}<span class="shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider" style="background: var(--color-surface); color: var(--color-muted)">{pageData.page_type}</span>{/if}
				<span class="shrink-0 text-xs" style="color: var(--color-muted)">/{pageData?.slug} · {blocks.length} {t('page_builder.blocks', 'blocks')}</span>
				{#if unsavedChanges}<span class="h-2 w-2 shrink-0 rounded-full" style="background: var(--color-warning)" title={t('pageBuilder.unsaved_changes', 'Unsaved changes')}></span>{/if}
				{#if successMessage}<span class="truncate text-sm" style="color: var(--color-success)">{successMessage}</span>{/if}
				{#if errorMessage}<span class="truncate text-sm" style="color: var(--color-danger)">{errorMessage}</span>{/if}
			{:else if activeHeaderPanel === 'preview'}
				<!-- Preview: geo country + device toggle + theme -->
				<Autocomplete
					id="preview-country"
					placeholder={t('landing.preview_country', 'Preview country')}
					emptyMessage={t('common.no_results', 'No results found')}
					selectedId={previewCountry}
					selectedLabel={previewCountry === 'global' ? t('landing.global', 'Global') : countryUtils.getSelectedLabel(previewCountry)}
					fetcher={(q, limit = 50) => {
						const globalItem = { id: 'global', label: t('landing.global', 'Global') };
						const needle = q.toLowerCase().trim();
						const matchesGlobal = !needle || 'global'.includes(needle) || globalItem.label.toLowerCase().includes(needle);
						return countryUtils.fetcher(q, limit - (matchesGlobal ? 1 : 0)).then((items) => matchesGlobal ? [globalItem, ...items].slice(0, limit) : items);
					}}
					initialFetcher={() => {
						const globalItem = { id: 'global', label: t('landing.global', 'Global') };
						return countryUtils.initialFetcher().then((items) => [globalItem, ...items]);
					}}
					onSelect={(item: AutocompleteItem) => { previewCountry = item.id; }}
					onClear={() => { previewCountry = 'global'; }}
					inputClass="text-xs"
				/>
				<div class="flex shrink-0 gap-1">
					<button onclick={() => (deviceMode = 'desktop')} class="rounded px-2 py-1 text-xs {deviceMode === 'desktop' ? 'text-white' : 'border'}" style={deviceMode === 'desktop' ? 'background: var(--color-primary)' : 'border-color: var(--color-border)'}>{t('landing.device_desktop', 'Desktop')}</button>
					<button onclick={() => (deviceMode = 'tablet')} class="rounded px-2 py-1 text-xs {deviceMode === 'tablet' ? 'text-white' : 'border'}" style={deviceMode === 'tablet' ? 'background: var(--color-primary)' : 'border-color: var(--color-border)'}>{t('landing.device_tablet', 'Tablet')}</button>
					<button onclick={() => (deviceMode = 'mobile')} class="rounded px-2 py-1 text-xs {deviceMode === 'mobile' ? 'text-white' : 'border'}" style={deviceMode === 'mobile' ? 'background: var(--color-primary)' : 'border-color: var(--color-border)'}>{t('landing.device_mobile', 'Mobile')}</button>
				</div>
				<select value={selectedThemeId} onchange={(e) => changeTheme(e.currentTarget.value)} class="shrink-0 rounded-md border px-2 py-1.5 text-xs" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" aria-label={t('page_builder.theme', 'Theme')}>
					<option value="">{t('page_builder.default_theme', 'Default Theme')}</option>
					{#each themes as th (th)}<option value={th.id} selected={selectedThemeId === th.id}>{th.name}{th.is_default ? ' (default)' : ''}</option>{/each}
				</select>
			{:else if activeHeaderPanel === 'actions'}
				<!-- Actions: save template + save draft + publish/archive -->
				{#if canManagePages}
					<Button variant="outline" size="sm" class="shrink-0" onclick={() => { templateName = `${pageData?.title ?? 'Page'} Template`; showSaveAsTemplate = true; }}>
						<LayoutTemplate class="mr-1 h-3 w-3" />{t('page_builder.save_as_template', 'Save as Template')}
					</Button>
				{/if}
				<Button variant="outline" size="sm" class="shrink-0" onclick={saveDraft} disabled={saving || !unsavedChanges}>
					<Save class="mr-1 h-3 w-3" />{saving ? t('common.saving', 'Saving...') : t('page_builder.save_draft', 'Save Draft')}
				</Button>
				{#if pageData?.status === 'published' && canPublishPages}
					<Button variant="outline" size="sm" class="shrink-0" onclick={archive} disabled={publishing}>
						<Archive class="mr-1 h-3 w-3" />{t('page_builder.archive', 'Archive')}
					</Button>
				{:else if pageData?.status !== 'published' && canPublishPages}
					<Button size="sm" class="shrink-0" onclick={() => { publishError = ''; showPublishDialog = true; }} disabled={publishing}>
						<Rocket class="mr-1 h-3 w-3" />{t('page_builder.publish', 'Publish')}
					</Button>
				{/if}
			{/if}
		</div>

		<!-- Right: 3-button segmented switch + section toggles + distraction-free -->
		<div class="flex shrink-0 items-center gap-2">
			<div class="flex gap-0.5 rounded-md border p-0.5" style="border-color: var(--color-border)">
				<button
					type="button"
					onclick={() => (activeHeaderPanel = 'info')}
					class="rounded px-3 py-1 text-xs font-medium transition-colors"
					style={activeHeaderPanel === 'info' ? 'background: var(--color-primary); color: white' : 'color: var(--color-muted)'}
				>Info</button>
				<button
					type="button"
					onclick={() => (activeHeaderPanel = 'preview')}
					class="rounded px-3 py-1 text-xs font-medium transition-colors"
					style={activeHeaderPanel === 'preview' ? 'background: var(--color-primary); color: white' : 'color: var(--color-muted)'}
				>Preview</button>
				<button
					type="button"
					onclick={() => (activeHeaderPanel = 'actions')}
					class="rounded px-3 py-1 text-xs font-medium transition-colors"
					style={activeHeaderPanel === 'actions' ? 'background: var(--color-primary); color: white' : 'color: var(--color-muted)'}
				>Actions</button>
			</div>
			<!-- Section visibility toggles -->
			<div class="flex gap-0.5 rounded-md border p-0.5" style="border-color: var(--color-border)">
				<button
					type="button"
					onclick={() => (showLeftPane = !showLeftPane)}
					class="rounded p-1 transition-colors hover:bg-[var(--color-surface)]"
					style={showLeftPane ? 'color: var(--color-foreground)' : 'color: var(--color-muted); opacity: 0.5'}
					title={showLeftPane ? 'Hide left panel' : 'Show left panel'}
					aria-label="Toggle left panel"
				><PanelLeft class="h-4 w-4" /></button>
				<button
					type="button"
					onclick={() => (showRightPane = !showRightPane)}
					class="rounded p-1 transition-colors hover:bg-[var(--color-surface)]"
					style={showRightPane ? 'color: var(--color-foreground)' : 'color: var(--color-muted); opacity: 0.5'}
					title={showRightPane ? 'Hide right panel' : 'Show right panel'}
					aria-label="Toggle right panel"
				><PanelRight class="h-4 w-4" /></button>
				<button
					type="button"
					onclick={() => (showHeader = !showHeader)}
					class="rounded p-1 transition-colors hover:bg-[var(--color-surface)]"
					style={showHeader ? 'color: var(--color-foreground)' : 'color: var(--color-muted); opacity: 0.5'}
					title={showHeader ? 'Hide top bar' : 'Show top bar'}
					aria-label="Toggle top bar"
				><PanelTop class="h-4 w-4" /></button>
			</div>
			<!-- Distraction-free / full-width editor -->
			<button
				type="button"
				onclick={toggleDistractionFree}
				class="rounded-md border p-1.5 transition-colors hover:bg-[var(--color-surface)]"
				style="border-color: var(--color-border); color: var(--color-foreground)"
				title={distractionFree ? 'Exit full-width mode' : 'Full-width editor (hide all panels)'}
				aria-label="Toggle full-width mode"
			>
				{#if distractionFree}
					<Minimize2 class="h-4 w-4" />
				{:else}
					<Maximize2 class="h-4 w-4" />
				{/if}
			</button>
		</div>
	</header>
	{/if}
	{#if saveError}
		<div class="px-4 py-2 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">{saveError}</div>
	{/if}

	<DragDropProvider onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
		<div class="relative flex min-h-0 flex-1">
			<!-- Floating restore tab for hidden left pane -->
			{#if !showLeftPane}
				<button
					type="button"
					onclick={() => (showLeftPane = true)}
					class="absolute left-0 top-1/2 z-50 -translate-y-1/2 rounded-r-md border border-l-0 px-1 py-3 transition-colors hover:bg-[var(--color-surface)]"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-muted)"
					aria-label="Show left panel"
				><PanelLeft class="h-4 w-4" /></button>
			{/if}
			<!-- Floating restore tab for hidden right pane -->
			{#if !showRightPane}
				<button
					type="button"
					onclick={() => (showRightPane = true)}
					class="absolute right-0 top-1/2 z-50 -translate-y-1/2 rounded-l-md border border-r-0 px-1 py-3 transition-colors hover:bg-[var(--color-surface)]"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-muted)"
					aria-label="Show right panel"
				><PanelRight class="h-4 w-4" /></button>
			{/if}
			<!-- Floating exit button for distraction-free mode -->
			{#if distractionFree}
				<button
					type="button"
					onclick={toggleDistractionFree}
					class="absolute right-3 top-3 z-50 flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium shadow-lg transition-colors hover:bg-[var(--color-surface)]"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
					aria-label="Exit full-width mode"
				><Minimize2 class="h-3.5 w-3.5" /> Exit full-width</button>
			{/if}

			{#if showLeftPane}
			<!-- Left pane: palette + sections list (resizable) -->
			<aside class="flex shrink-0 flex-col" style="width: {leftPaneWidth}px; background: var(--color-background)">
				<!-- Palette (resizable height) -->
				<div class="shrink-0 overflow-y-auto p-2" style="height: {paletteHeight}px">
					<h2 class="mb-2 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('pageBuilder.blockTypes', 'Block Types')}</h2>
					<div class="relative mb-2">
						<Search class="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2" style="color: var(--color-muted)" />
						<input type="text" bind:value={paletteSearch} placeholder={t('common.search', 'Search')} class="w-full rounded-md border py-1.5 pl-8 pr-2 text-xs" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
					</div>
					{#if paletteMessage}
						<p class="mb-2 rounded border px-2 py-1.5 text-[11px]" style="border-color: var(--color-warning); color: var(--color-warning)">{paletteMessage}</p>
					{/if}
					{#each Object.entries(groupedBlockTypes) as [category, types] (category)}
						<p class="mb-1.5 mt-3 text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{category}</p>
						<div class="space-y-1.5">
							{#each types as [type, meta] (type)}
								<DraggableBlockType blockType={type} {meta} {getIcon} onAdd={addBlock} />
							{/each}
						</div>
					{/each}
				</div>

				<!-- Resize handle: palette ↔ sections (drag up/down) -->
				<ResizeHandle paneId="palette" size={paletteHeight} orientation="vertical" direction="right" minSize={120} maxSize={700} onResize={handlePaneResize} />

				<!-- Sections list (sortable) — fills remaining height -->
				<div class="flex min-h-0 flex-1 flex-col overflow-y-auto p-2">
					<h2 class="mb-2 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('landing.sections', 'Sections')}</h2>
					{#if blocks.length === 0}
						<p class="px-1 py-2 text-xs" style="color: var(--color-muted)">{t('pageBuilder.dropBlocks', 'Drag blocks here or click to add')}</p>
					{:else}
						{#each blocks as block, i (block.id)}
							{@const meta = blockRegistry[block.type]}
							<SortableSection {block} index={i} selected={selectedBlockId === block.id} {meta} {getIcon} onSelect={() => (selectedBlockId = block.id)} onRemove={() => removeBlock(block.id)} />
							{#if isContainerLike(block)}
								{@const slots = childSlotsOf(block)}
								<div class="ml-2">
									<button
										type="button"
										onclick={() => toggleContainerExpand(block.id)}
										class="text-[10px] text-[var(--muted)] hover:text-[var(--foreground)] px-2 py-0.5"
									>
										{expandedContainers.has(block.id) ? '▼' : '▶'} {slots.reduce((n, s) => n + s.blocks.length, 0)} children
									</button>
								</div>
								{#if expandedContainers.has(block.id)}
									{#each slots as slot (slot.key)}
										{#if slot.label}
											<div class="ml-6 mt-1 text-[10px] font-medium uppercase tracking-wider" style="color: var(--color-muted)">{slot.label}</div>
										{/if}
										<ContainerChildren
											containerId={block.id}
											slotKey={slot.key}
											children={slot.blocks}
											layout={getContainerLayout(block)}
											columns={getContainerColumns(block)}
											{selectedBlockId}
											{getIcon}
											onSelect={(id) => (selectedBlockId = id)}
											onReorder={reorderContainerChild}
											onRemove={removeContainerChild}
											onMoveOut={moveChildToTopLevel}
											onSpanChange={updateChildSpan} {expandedContainers} toggleExpand={toggleContainerExpand}
										/>
									{/each}
								{/if}
							{/if}
						{/each}
					{/if}
					<button onclick={() => addBlock('hero')} class="mt-2 w-full rounded border border-dashed py-1.5 text-xs hover:bg-[var(--surface)]" style="border-color: var(--color-border); color: var(--color-muted)">
						+ {t('landing.add_section', 'Add Section')}
					</button>
				</div>
			</aside>

			<!-- Resize handle: left ↔ center -->
			<ResizeHandle paneId="left" size={leftPaneWidth} minSize={180} maxSize={500} onResize={handlePaneResize} />
			{/if}

			<!-- Center pane: live preview -->
			<main class="flex-1 overflow-y-auto p-4" style="background: var(--color-surface)">
				<div class="mx-auto shadow-lg" style="width: {deviceWidth}px; max-width: 100%; background: var(--color-background)">
					{#if resolvedBlocks.length > 0}
						<LandingRenderer blocks={resolvedBlocks} {fetchCtx} geoCountry={previewCountry} guestHome={guestHomePreview} />
					{:else}
						<div class="flex h-full flex-col items-center justify-center rounded-xl border-2 border-dashed py-20" style="border-color: var(--color-border)">
							<Plus class="mb-3 h-8 w-8" style="color: var(--color-muted)" />
							<p class="text-sm font-medium" style="color: var(--color-foreground)">{t('pageBuilder.emptyCanvas', 'Build your page by adding blocks from the left panel')}</p>
						</div>
					{/if}
				</div>
			</main>

			{#if showRightPane}
			<!-- Resize handle: center ↔ right (dragging left grows the right pane) -->
			<ResizeHandle paneId="right" size={rightPaneWidth} direction="left" minSize={250} maxSize={700} onResize={handlePaneResize} />

			<!-- Right pane: inspector (resizable) -->
			<aside class="shrink-0 overflow-y-auto" style="width: {rightPaneWidth}px; border-left: 1px solid var(--color-border); background: var(--color-background)">
				{#if selectedBlock}
					{@const meta = blockRegistry[selectedBlock.type]}
					<div class="border-b p-3" style="border-color: var(--color-border)">
						<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">{meta?.label ?? selectedBlock.type}</h3>
					</div>
					<Tabs.Root bind:value={activeTab} class="flex h-full flex-col">
						<Tabs.List class="flex border-b" style="border-color: var(--color-border)">
							<Tabs.Trigger value="content" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'content' ? 'border-b-2 text-[var(--primary)]' : 'text-[var(--muted)]'}" style="border-color: var(--color-primary)">{t('landing.tab_content', 'Content')}</Tabs.Trigger>
							<Tabs.Trigger value="style" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'style' ? 'border-b-2 text-[var(--primary)]' : 'text-[var(--muted)]'}" style="border-color: var(--color-primary)">{t('landing.tab_style', 'Style')}{#if deviceMode !== 'desktop'} <span class="text-[9px] uppercase opacity-70">({deviceMode})</span>{/if}</Tabs.Trigger>
							<Tabs.Trigger value="settings" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'settings' ? 'border-b-2 text-[var(--primary)]' : 'text-[var(--muted)]'}" style="border-color: var(--color-primary)">{t('landing.tab_settings', 'Settings')}</Tabs.Trigger>
						</Tabs.List>
						<Tabs.Content value="content" class="flex-1 overflow-y-auto p-3">
							{#if ContentEditor}
								{@const Editor = ContentEditor}
								{#if selectedBlock.type === 'custom'}
									<Editor props={selectedBlock.props} blockDefId={selectedBlock.block_def_id} onchange={(v: unknown) => updateBlockProps(selectedBlock.id, v)} mediaPickerComponent={MediaPicker} geoCtx={previewCtx} />
								{:else}
									<Editor props={selectedBlock.props} onchange={(v: unknown) => updateBlockProps(selectedBlock.id, v)} mediaPickerComponent={MediaPicker} widgetPickerComponent={WidgetPickerComponent} geoCtx={previewCtx} />
								{/if}
							{:else}
								<p class="text-xs" style="color: var(--color-muted)">{t('pageBuilder.no_config', 'No content editor for this block type')}</p>
							{/if}
						</Tabs.Content>
						<Tabs.Content value="style" class="flex-1 overflow-y-auto p-3">
							<BlockStylePanel value={selectedBlock.style} onchange={(v: BlockStyle) => updateBlockStyle(selectedBlock.id, v)} mediaPickerComponent={MediaPicker} device={deviceMode} />
						</Tabs.Content>
						<Tabs.Content value="settings" class="flex-1 overflow-y-auto p-3">
							<div class="space-y-4">
								<label class="flex items-center justify-between text-sm">
									<span style="color: var(--color-foreground)">{t('landing.visible', 'Visible')}</span>
									<input type="checkbox" checked={selectedBlock.visible} onchange={() => toggleBlockVisibility(selectedBlock.id)} />
								</label>

								<!-- Per-device visibility -->
								<div class="space-y-2 rounded border p-3" style="border-color: var(--color-border)">
									<h4 class="text-xs font-semibold uppercase" style="color: var(--color-muted)">Show on device</h4>
									<div class="grid grid-cols-3 gap-2">
										<label class="flex items-center justify-center gap-1.5 rounded border px-2 py-1.5 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">
											<input
												type="checkbox"
												checked={selectedBlock.responsive_visibility?.desktop !== false}
												onchange={(e) => updateResponsiveVisibility(selectedBlock.id, 'desktop', e.currentTarget.checked)}
											/>
											Desktop
										</label>
										<label class="flex items-center justify-center gap-1.5 rounded border px-2 py-1.5 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">
											<input
												type="checkbox"
												checked={selectedBlock.responsive_visibility?.tablet !== false}
												onchange={(e) => updateResponsiveVisibility(selectedBlock.id, 'tablet', e.currentTarget.checked)}
											/>
											Tablet
										</label>
										<label class="flex items-center justify-center gap-1.5 rounded border px-2 py-1.5 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">
											<input
												type="checkbox"
												checked={selectedBlock.responsive_visibility?.mobile !== false}
												onchange={(e) => updateResponsiveVisibility(selectedBlock.id, 'mobile', e.currentTarget.checked)}
											/>
											Mobile
										</label>
									</div>
									<p class="text-[10px]" style="color: var(--color-muted)">Uncheck to hide this block on that device. The block stays in the DOM but is display:none.</p>
								</div>

								<!-- Conditional visibility rules -->
								<div class="space-y-3 rounded border p-3" style="border-color: var(--color-border)">
									<h4 class="text-xs font-semibold uppercase" style="color: var(--color-muted)">{t('landing.visibility_rules', 'Visibility rules')}</h4>
									<label class="flex items-center justify-between text-sm">
										<span style="color: var(--color-foreground)">{t('landing.hide_if_empty', 'Hide if empty')}</span>
										<input
											type="checkbox"
											checked={selectedBlock.visibility_rules?.hide_if_empty ?? false}
											onchange={(e) => updateVisibilityRuleHideIfEmpty(selectedBlock.id, e.currentTarget.checked)}
										/>
									</label>
									<div>
										<span class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.geo_countries', 'Show only in countries')}</span>
									<Autocomplete
										id="block-geo-countries"
										placeholder={t('landing.search_country', 'Search country...')}
										emptyMessage={t('common.no_results', 'No results found')}
										fetcher={countryUtils.fetcher}
										initialFetcher={countryUtils.initialFetcher}
										selectedId={null}
										selectedLabel={null}
										onSelect={(item: AutocompleteItem) => updateVisibilityRuleGeo(selectedBlock.id, item.id, true)}
										onClear={() => {}}
									/>
									{#if (selectedBlock.visibility_rules?.geo_countries ?? []).length > 0}
										<div class="mt-2 flex flex-wrap gap-2">
											{#each (selectedBlock.visibility_rules?.geo_countries ?? []) as code (code)}
												<span class="inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium" style="background-color: var(--color-primary); color: white">
													{countryUtils.getSelectedLabel(code)}
													<button onclick={() => updateVisibilityRuleGeo(selectedBlock.id, code, false)} class="hover:opacity-70" aria-label={t('common.remove', 'Remove')}>×</button>
												</span>
											{/each}
										</div>
									{/if}
								</div>
								</div>

								<!-- Motion / Animation -->
								<div class="space-y-3 rounded border p-3" style="border-color: var(--color-border)">
									<h4 class="text-xs font-semibold uppercase" style="color: var(--color-muted)">{t('landing.animation', 'Animation')}</h4>
									<div>
										<label for="motion-preset" class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.motion_preset', 'Preset')}</label>
										<select id="motion-preset" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.preset ?? 'none'} onchange={(e) => updateBlockMotion(selectedBlock.id, { preset: e.currentTarget.value as MotionConfig['preset'] })}>
											<optgroup label={t('landing.motion_none', 'None')}><option value="none">{t('landing.motion_none', 'None')}</option></optgroup>
											<optgroup label={t('landing.motion_fade', 'Fade')}>
												<option value="fade-in">{t('landing.motion_fade_in', 'Fade In')}</option>
												<option value="fade-up">{t('landing.motion_fade_up', 'Fade Up')}</option>
												<option value="fade-down">{t('landing.motion_fade_down', 'Fade Down')}</option>
											</optgroup>
											<optgroup label={t('landing.motion_fly_in', 'Fly In')}>
												<option value="fly-in-left">{t('landing.motion_fly_left', 'Fly In from Left')}</option>
												<option value="fly-in-right">{t('landing.motion_fly_right', 'Fly In from Right')}</option>
												<option value="fly-in-up">{t('landing.motion_fly_bottom', 'Fly In from Bottom')}</option>
												<option value="fly-in-down">{t('landing.motion_fly_top', 'Fly In from Top')}</option>
											</optgroup>
											<optgroup label={t('landing.motion_slide', 'Slide')}>
												<option value="slide-in">{t('landing.motion_slide_in', 'Slide In')}</option>
												<option value="slide-in-left">{t('landing.motion_slide_left', 'Slide In from Left')}</option>
												<option value="slide-in-right">{t('landing.motion_slide_right', 'Slide In from Right')}</option>
											</optgroup>
											<optgroup label={t('landing.motion_scale_zoom', 'Scale & Zoom')}>
												<option value="scale-in">{t('landing.motion_scale_in', 'Scale In')}</option>
												<option value="scale-out">{t('landing.motion_scale_out', 'Scale Out')}</option>
												<option value="zoom-in">{t('landing.motion_zoom_in', 'Zoom In')}</option>
												<option value="zoom-out">{t('landing.motion_zoom_out', 'Zoom Out')}</option>
											</optgroup>
											<optgroup label={t('landing.motion_blur_flip', 'Blur & Flip')}>
												<option value="blur-in">{t('landing.motion_blur_in', 'Blur In')}</option>
												<option value="blur-out">{t('landing.motion_blur_out', 'Blur Out')}</option>
												<option value="flip-up">{t('landing.motion_flip_up', 'Flip Up')}</option>
												<option value="flip-down">{t('landing.motion_flip_down', 'Flip Down')}</option>
											</optgroup>
											<optgroup label={t('landing.motion_special', 'Special')}><option value="bounce-in">{t('landing.motion_bounce_in', 'Bounce In')}</option></optgroup>
										</select>
									</div>
									{#if (selectedBlock.motion?.preset ?? 'none') !== 'none'}
										<div class="grid grid-cols-2 gap-2">
											<div>
												<label for="motion-delay" class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.motion_delay', 'Delay (ms)')}</label>
												<input id="motion-delay" type="number" min="0" step="50" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.delay ?? 0} oninput={(e) => updateBlockMotion(selectedBlock.id, { delay: Number(e.currentTarget.value) })} />
											</div>
											<div>
												<label for="motion-duration" class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.motion_duration', 'Duration (ms)')}</label>
												<input id="motion-duration" type="number" min="100" step="50" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.duration ?? 700} oninput={(e) => updateBlockMotion(selectedBlock.id, { duration: Number(e.currentTarget.value) })} />
											</div>
										</div>
										<div>
											<label for="motion-easing" class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.motion_easing', 'Easing')}</label>
											<select id="motion-easing" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.easing ?? 'cubic-bezier(0.16, 1, 0.3, 1)'} onchange={(e) => updateBlockMotion(selectedBlock.id, { easing: e.currentTarget.value })}>
												<option value="cubic-bezier(0.16, 1, 0.3, 1)">{t('landing.easing_smooth', 'Smooth (default)')}</option>
												<option value="ease-out">{t('landing.easing_out', 'Ease Out')}</option>
												<option value="ease-in-out">{t('landing.easing_in_out', 'Ease In Out')}</option>
												<option value="ease-in">{t('landing.easing_in', 'Ease In')}</option>
												<option value="linear">{t('landing.easing_linear', 'Linear')}</option>
												<option value="cubic-bezier(0.34, 1.56, 0.64, 1)">{t('landing.easing_bounce', 'Bounce')}</option>
												<option value="cubic-bezier(0.68, -0.55, 0.27, 1.55)">{t('landing.easing_elastic', 'Elastic')}</option>
											</select>
										</div>
										<div>
											<label for="motion-threshold" class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.motion_threshold', 'Viewport threshold')}: {selectedBlock.motion?.threshold ?? 0.1}</label>
											<input id="motion-threshold" type="range" min="0" max="1" step="0.05" class="w-full" value={selectedBlock.motion?.threshold ?? 0.1} oninput={(e) => updateBlockMotion(selectedBlock.id, { threshold: Number(e.currentTarget.value) })} />
										</div>
										<label class="flex items-center justify-between text-xs">
											<span style="color: var(--color-foreground)">{t('landing.motion_once', 'Animate only once')}</span>
											<input type="checkbox" checked={selectedBlock.motion?.once ?? true} onchange={(e) => updateBlockMotion(selectedBlock.id, { once: e.currentTarget.checked })} />
										</label>
										<label class="flex items-center justify-between text-xs">
											<span style="color: var(--color-foreground)">{t('landing.motion_stagger', 'Stagger children')}</span>
											<input type="checkbox" checked={selectedBlock.motion?.stagger_children ?? false} onchange={(e) => updateBlockMotion(selectedBlock.id, { stagger_children: e.currentTarget.checked })} />
										</label>
										{#if selectedBlock.motion?.stagger_children}
											<div>
												<label for="motion-stagger-delay" class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.motion_stagger_delay', 'Stagger delay (ms)')}</label>
												<input id="motion-stagger-delay" type="number" min="0" step="20" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.stagger_delay ?? 80} oninput={(e) => updateBlockMotion(selectedBlock.id, { stagger_delay: Number(e.currentTarget.value) })} />
											</div>
										{/if}
										<button type="button" onclick={() => updateBlockMotion(selectedBlock.id, null)} class="w-full rounded border px-2 py-1 text-xs hover:bg-[var(--surface)]" style="border-color: var(--color-border); color: var(--color-muted)">
											{t('landing.motion_reset', 'Reset animation')}
										</button>
									{/if}
								</div>

								<div class="flex gap-2 pt-2">
									<button onclick={() => duplicateBlock(selectedBlock.id)} class="flex-1 rounded border px-2 py-1 text-xs" style="border-color: var(--color-border)">{t('landing.duplicate', 'Duplicate')}</button>
									<button onclick={() => removeBlock(selectedBlock.id)} class="flex-1 rounded border px-2 py-1 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{t('common.delete', 'Delete')}</button>
								</div>
							</div>
						</Tabs.Content>
					</Tabs.Root>
				{:else}
					<div class="p-6 text-center text-sm" style="color: var(--color-muted)">
						{t('landing.select_section_hint', 'Select a section to edit its content, style, and settings.')}
					</div>
				{/if}
			</aside>
			{/if}
		</div>

		<DragOverlay>
			{#if activeDragData}
				<div class="rounded-lg border px-4 py-3 text-sm font-medium shadow-lg" style="border-color: var(--color-primary); background: var(--color-background); color: var(--color-foreground)">
					{activeDragData.meta?.label ?? (blockRegistry as Record<string, BlockMeta>)[activeDragData.blockType as string]?.label ?? activeDragData.blockType ?? 'Block'}
				</div>
			{/if}
		</DragOverlay>
	</DragDropProvider>
</div>
{/if}

<!-- Page settings dialog — the only UI where slug/page_type/description/SEO
     title can be edited after creation (the list page's Edit action opens
     this editor, which previously had no metadata form). -->
<Dialog.Root bind:open={showPageSettings}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-xl" style="border-color: var(--color-border); background: var(--color-background)">
			<Dialog.Title class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">{t('page_builder.page_settings', 'Page Settings')}</Dialog.Title>
			<div class="space-y-4">
				<div>
					<label for="page-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('page_builder.slug', 'Slug')}</label>
					<div class="flex items-center gap-1">
						<span class="text-sm" style="color: var(--color-muted)">/</span>
						<input id="page-slug" type="text" bind:value={pageSlug} oninput={() => (unsavedChanges = true)} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
					</div>
				</div>
				<div>
					<label for="page-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('page_builder.page_type', 'Page type')}</label>
					<select id="page-type" bind:value={pageType} onchange={() => (unsavedChanges = true)} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)">
						{#if pageTypeOptions.length === 0 && pageType}
							<option value={pageType}>{pageType}</option>
						{/if}
						{#each pageTypeOptions as opt (opt.value)}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="page-description" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('page_builder.description', 'Description')}</label>
					<textarea id="page-description" rows="2" bind:value={pageDescription} oninput={() => (unsavedChanges = true)} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"></textarea>
					<p class="mt-1 text-[11px]" style="color: var(--color-muted)">{t('page_builder.description_hint', 'Used as the meta description on the published page.')}</p>
				</div>
				<div>
					<label for="page-seo-title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('page_builder.seo_title', 'SEO title')}</label>
					<input id="page-seo-title" type="text" bind:value={seoTitle} oninput={() => (unsavedChanges = true)} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" placeholder={pageTitle} />
					<p class="mt-1 text-[11px]" style="color: var(--color-muted)">{t('page_builder.seo_title_hint', 'Overrides the browser/OG title (stored in meta.title).')}</p>
				</div>
			</div>
			<div class="mt-4 flex justify-end gap-2">
				<Button variant="ghost" onclick={() => { showPageSettings = false; }}>{t('common.done', 'Done')}</Button>
			</div>
			<Dialog.Close class="absolute right-4 top-4" style="color: var(--color-muted)" aria-label="Close">&#10005;</Dialog.Close>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Save-as-template dialog -->
<Dialog.Root bind:open={showSaveAsTemplate}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-xl" style="border-color: var(--color-border); background: var(--color-background)">
			<Dialog.Title class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">{t('page_builder.save_as_template', 'Save as Template')}</Dialog.Title>
			<div>
				<label for="tpl-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('page_builder.template_name', 'Template name')}</label>
				<input id="tpl-name" type="text" bind:value={templateName} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
			</div>
			<div class="mt-4 flex justify-end gap-2">
				<Button variant="ghost" onclick={() => { showSaveAsTemplate = false; }}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={saveAsTemplate} disabled={savingTemplate || !templateName.trim()}>{savingTemplate ? t('common.saving', 'Saving...') : t('common.save', 'Save')}</Button>
			</div>
			<Dialog.Close class="absolute right-4 top-4" style="color: var(--color-muted)" aria-label="Close">&#10005;</Dialog.Close>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Publish confirmation dialog -->
<Dialog.Root bind:open={showPublishDialog}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-xl" style="border-color: var(--color-border); background: var(--color-background)">
			<Dialog.Title class="mb-2 text-lg font-semibold" style="color: var(--color-foreground)">{t('pageBuilder.publish_confirm_title', 'Publish Page?')}</Dialog.Title>
			<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('pageBuilder.publish_confirm_body', 'This will make the current draft live. Visitors will see the published version immediately.')}</p>
			{#if publishError}<p class="mb-4 text-sm" style="color: var(--color-danger)">{publishError}</p>{/if}
			<div class="flex justify-end gap-2">
				<button onclick={() => (showPublishDialog = false)} class="rounded border px-4 py-2 text-sm" style="border-color: var(--color-border)">{t('common.cancel', 'Cancel')}</button>
				<button onclick={publish} disabled={publishing} class="rounded px-4 py-2 text-sm font-semibold text-white disabled:opacity-50" style="background: var(--color-primary)">
					{publishing ? t('pageBuilder.publishing', 'Publishing...') : t('page_builder.publish', 'Publish')}
				</button>
			</div>
			<Dialog.Close class="absolute right-4 top-4" style="color: var(--color-muted)" aria-label="Close">&#10005;</Dialog.Close>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
