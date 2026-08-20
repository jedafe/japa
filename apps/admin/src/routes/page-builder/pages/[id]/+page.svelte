<script lang="ts">
	// Ref: japaconnect-engineering/sections/16-page-builder.md
	// Unified Page Builder editor — handles all page types (landing, about,
	// service, event, custom) on the shared @japaconnect/landing-blocks library.
	// dnd-kit drag-and-drop + live LandingRenderer preview + per-type content
	// editor + BlockStylePanel + motion/visibility settings + geo preview.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, untrack, type Component } from 'svelte';
	import { api, pageBuilderAdminApiExtended } from '$lib/api';
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
		type Block,
		type BlockStyle,
		type BlockMeta,
		type GeoContext,
		type ContainerProps,
		type CustomBlockDef,
		type RuntimeBlockMeta,
		type VisibilityRules,
	} from '@japaconnect/landing-blocks';
	import { DraggableBlockType, SortableSection, ContainerChildren, ResizeHandle, WidgetPicker } from '$lib/components/landing-editor';
	import { Button } from '$lib/components/ui/button';
	import { Tabs, Dialog } from 'bits-ui';
	import * as LucideIcons from 'lucide-svelte';
	import { ArrowLeft, Save, Plus, Search, LayoutTemplate, Rocket, Archive, PanelLeft, PanelRight, PanelTop, Maximize2, Minimize2 } from 'lucide-svelte';
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
	}

	let pageData = $state<{ id: string; title: string; slug: string; status: string; page_type: string; theme_id?: string } | null>(null);
	let blocks = $state<Block[]>([]);
	let themes = $state<Theme[]>([]);
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

	// Auto-expand a container when it's selected so children are visible.
	$effect(() => {
		if (!selectedBlockId) return;
		const block = blocks.find((b) => b.id === selectedBlockId);
		if (block?.type === 'container' && !expandedContainers.has(block.id)) {
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

	// Theme + save-as-template wiring
	let selectedThemeId = $state<string>('');
	let showSaveAsTemplate = $state(false);
	let templateName = $state('');
	let savingTemplate = $state(false);

	const selectedBlock = $derived(selectedBlockId ? findBlockInTree(blocks, selectedBlockId) : null);
	const visibleBlocks = $derived(blocks.filter((b) => b.visible));

	const previewCtx: GeoContext = $derived({
		contentCountry: previewCountry,
		tokens: getGeoTokens(previewCountry, 'en'),
		locale: 'en',
	});
	const resolvedBlocks = $derived(resolveGeoCopy(visibleBlocks, previewCtx));
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

	$effect(() => { void load(); });

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const pageId = page.params.id;
			// Pre-load the 250 primary geo space countries via the shared utility
			// while the page data loads.
			countryUtils.load();
			const [pageRes, themesRes, blocksRes] = await Promise.all([
				api<PageData>(`/admin/pages/${pageId}`),
				pageBuilderAdminApiExtended.listThemes(),
				pageBuilderAdminApiExtended.listBlocks(),
			]);
			if (!pageRes.ok) {
				errorMessage = pageRes.error || t('common.error', 'Failed to load page');
			} else if (pageRes.data) {
				const d = pageRes.data;
				pageData = { id: d.id, title: d.title, slug: d.slug, status: d.status, page_type: d.page_type ?? 'custom', theme_id: d.theme_id };
				// Seed the editor's working copy once, so subsequent server-side
				// changes don't clobber in-progress edits.
				blocks = untrack(() => parseBlocks(d.blocks));
				pageTitle = untrack(() => d.title);
				selectedThemeId = d.theme_id ?? '';
			}
			if (themesRes.ok) themes = ((themesRes.data as any[]) ?? []).map((th) => ({ id: th.id, name: th.name, is_default: th.is_default }));
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

	function getIcon(name: string): Component | null {
		const found = (LucideIcons as Record<string, unknown>)[name];
		return (found as Component) ?? null;
	}

	// ─── Block CRUD ───────────────────────────────────────────────
	// `type` can be a real Block type or a `custom:{blockDefId}` palette key.
	function makeBlock(type: string): Block {
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
		return { id: crypto.randomUUID(), type: type as Block['type'], visible: true, props: {} } as Block;
	}
	function addBlock(type: string): void {
		const nb = makeBlock(type);
		blocks = [...blocks, nb];
		selectedBlockId = nb.id;
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
	function duplicateBlock(id: string): void {
		const block = findBlockInTree(blocks, id);
		if (!block) return;
		const copy = { ...block, id: crypto.randomUUID(), props: JSON.parse(JSON.stringify(block.props)) } as Block;
		// If the block is top-level, insert after it. Otherwise append to top level.
		const idx = blocks.findIndex((b) => b.id === id);
		if (idx >= 0) {
			blocks = [...blocks.slice(0, idx + 1), copy, ...blocks.slice(idx + 1)];
		} else {
			blocks = [...blocks, copy];
		}
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
			if (b.type === 'container') {
				const found = findBlockInTree((b.props as ContainerProps).children ?? [], id);
				if (found) return found;
			}
		}
		return null;
	}

	/** Apply a transform to a block anywhere in the tree (top-level or nested). */
	function updateBlockInTree(tree: Block[], id: string, fn: (b: Block) => Block): Block[] {
		return tree.map((b) => {
			if (b.id === id) return fn(b);
			if (b.type === 'container') {
				const props = b.props as ContainerProps;
				return { ...b, props: { ...props, children: updateBlockInTree(props.children ?? [], id, fn) } } as Block;
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
			if (tree[i].type === 'container') {
				const props = tree[i].props as ContainerProps;
				const result = removeBlockFromTree(props.children ?? [], id);
				if (result.removed) {
					const newTree = [...tree];
					newTree[i] = { ...tree[i], props: { ...props, children: result.tree } } as Block;
					return { tree: newTree, removed: result.removed };
				}
			}
		}
		return { tree, removed: null };
	}

	/** Insert a block into a container's children (found by ID, anywhere in the tree). */
	function insertIntoContainer(tree: Block[], containerId: string, block: Block): Block[] {
		return tree.map((b) => {
			if (b.id === containerId && b.type === 'container') {
				const props = b.props as ContainerProps;
				return { ...b, props: { ...props, children: [...(props.children ?? []), block] } } as Block;
			}
			if (b.type === 'container') {
				const props = b.props as ContainerProps;
				return { ...b, props: { ...props, children: insertIntoContainer(props.children ?? [], containerId, block) } } as Block;
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

	/** Reorder a child within a container (by container ID + indices). */
	function reorderContainerChild(containerId: string, fromIdx: number, toIdx: number): void {
		if (fromIdx === toIdx) return;
		blocks = blocks.map((b) => {
			if (b.id === containerId && b.type === 'container') {
				const props = b.props as ContainerProps;
				const children = [...(props.children ?? [])];
				if (toIdx < 0 || toIdx >= children.length) return b;
				const [moved] = children.splice(fromIdx, 1);
				children.splice(toIdx, 0, moved);
				return { ...b, props: { ...props, children } } as Block;
			}
			return b;
		});
		unsavedChanges = true;
	}

	/** Reorder a child by ID within a container (by container ID + child IDs). */
	function reorderContainerChildByIds(containerId: string, fromChildId: string, toChildId: string): void {
		blocks = blocks.map((b) => {
			if (b.id === containerId && b.type === 'container') {
				const props = b.props as ContainerProps;
				const children = [...(props.children ?? [])];
				const fromIdx = children.findIndex((c) => c.id === fromChildId);
				const toIdx = children.findIndex((c) => c.id === toChildId);
				if (fromIdx < 0 || toIdx < 0 || fromIdx === toIdx) return b;
				const [moved] = children.splice(fromIdx, 1);
				children.splice(toIdx, 0, moved);
				return { ...b, props: { ...props, children } } as Block;
			}
			return b;
		});
		unsavedChanges = true;
	}

	/** Insert a block into a container at a specific index (before the target child). */
	function insertIntoContainerAtIndex(containerId: string, block: Block, beforeChildId: string | null): void {
		blocks = blocks.map((b) => {
			if (b.id === containerId && b.type === 'container') {
				const props = b.props as ContainerProps;
				const children = [...(props.children ?? [])];
				if (beforeChildId) {
					const idx = children.findIndex((c) => c.id === beforeChildId);
					if (idx >= 0) children.splice(idx, 0, block);
					else children.push(block);
				} else {
					children.push(block);
				}
				return { ...b, props: { ...props, children } } as Block;
			}
			return b;
		});
	}

	/** Remove a child from a container. */
	function removeContainerChild(containerId: string, childId: string): void {
		blocks = blocks.map((b) => {
			if (b.id === containerId && b.type === 'container') {
				const props = b.props as ContainerProps;
				return { ...b, props: { ...props, children: (props.children ?? []).filter((c) => c.id !== childId) } } as Block;
			}
			return b;
		});
		if (selectedBlockId === childId) selectedBlockId = null;
		unsavedChanges = true;
	}

	/** Move a child out of a container to the top-level blocks array. */
	function moveChildToTopLevel(containerId: string, childId: string): void {
		const container = blocks.find((b) => b.id === containerId && b.type === 'container');
		if (!container) return;
		const childProps = container.props as ContainerProps;
		const blockToMove = (childProps.children ?? []).find((c) => c.id === childId);
		if (!blockToMove) return;
		// Remove from container, append to top level.
		blocks = blocks.map((b) => {
			if (b.id === containerId && b.type === 'container') {
				return { ...b, props: { ...childProps, children: (childProps.children ?? []).filter((c) => c.id !== childId) } } as Block;
			}
			return b;
		});
		blocks = [...blocks, blockToMove];
		unsavedChanges = true;
	}

	/** Update a child's grid column span (1–12). */
	function updateChildSpan(containerId: string, childId: string, span: number): void {
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

		// Drop onto a container drop zone → insert into that container (append).
		if (targetData?.source === 'container-dropzone') {
			const containerId = targetData.containerId as string;
			if (sourceData?.source === 'palette') {
				const nb = makeBlock(sourceData.blockType as string);
				blocks = insertIntoContainer(blocks, containerId, nb);
				selectedBlockId = nb.id;
				expandedContainers = new Set(expandedContainers).add(containerId);
				unsavedChanges = true;
				return;
			}
			if (sourceData?.source === 'canvas' || sourceData?.source === 'container-child') {
				const { tree: newTree, removed } = removeBlockFromTree(blocks, source.id as string);
				if (!removed) return;
				blocks = insertIntoContainer(newTree, containerId, removed);
				selectedBlockId = removed.id;
				expandedContainers = new Set(expandedContainers).add(containerId);
				unsavedChanges = true;
				return;
			}
		}

		// Drop onto a container child (sortable target) → reorder or insert.
		if (targetData?.source === 'container-child' && target && isSortable(target)) {
			const targetContainerId = targetData.containerId as string;
			const targetChildId = target.id as string;

			// Palette → insert before the target child.
			if (sourceData?.source === 'palette') {
				const nb = makeBlock(sourceData.blockType as string);
				insertIntoContainerAtIndex(targetContainerId, nb, targetChildId);
				selectedBlockId = nb.id;
				expandedContainers = new Set(expandedContainers).add(targetContainerId);
				unsavedChanges = true;
				return;
			}

			// Container child → reorder within the same container.
			if (sourceData?.source === 'container-child' && sourceData.containerId === targetContainerId) {
				reorderContainerChildByIds(targetContainerId, source.id as string, targetChildId);
				return;
			}

			// Canvas section → move into container before target child.
			if (sourceData?.source === 'canvas') {
				const { tree: newTree, removed } = removeBlockFromTree(blocks, source.id as string);
				if (!removed) return;
				blocks = newTree;
				insertIntoContainerAtIndex(targetContainerId, removed, targetChildId);
				selectedBlockId = removed.id;
				expandedContainers = new Set(expandedContainers).add(targetContainerId);
				unsavedChanges = true;
				return;
			}

			// Container child from a different container → move between containers.
			if (sourceData?.source === 'container-child' && sourceData.containerId !== targetContainerId) {
				const { tree: newTree, removed } = removeBlockFromTree(blocks, source.id as string);
				if (!removed) return;
				blocks = newTree;
				insertIntoContainerAtIndex(targetContainerId, removed, targetChildId);
				selectedBlockId = removed.id;
				expandedContainers = new Set(expandedContainers).add(targetContainerId);
				unsavedChanges = true;
				return;
			}
		}

		// Drop from palette → add new block at target position (or append).
		if (sourceData?.source === 'palette') {
			const nb = makeBlock(sourceData.blockType as string);
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
		const body: Record<string, unknown> = {
			title: pageTitle,
			slug: pageData.slug,
			status: pageData.status,
			// Send blocks as a parsed array — api() already JSON.stringifies
			// the body. Double-encoding (JSON.stringify here) produced a JSON
			// string that the backend's json.RawMessage + sanitizeBlocksJSON
			// mishandled, silently emptying the page on save.
			blocks,
		};
		if (selectedThemeId) body.theme_id = selectedThemeId;
		const result = await api(`/admin/pages/${pageData.id}`, { method: 'PATCH', body });
		saving = false;
		if (result.ok) { unsavedChanges = false; return true; }
		saveError = result.error ?? t('pageBuilder.save_failed', 'Failed to save draft');
		return false;
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

	// Warn before navigating away with unsaved edits.
	$effect(() => {
		if (!unsavedChanges) return;
		const handler = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = ''; };
		window.addEventListener('beforeunload', handler);
		return () => window.removeEventListener('beforeunload', handler);
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
				<input type="text" bind:value={pageTitle} class="w-48 rounded border px-2 py-1 text-sm font-medium" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" aria-label={t('page_builder.title', 'Title')} />
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
				{#if pageData?.status === 'published'}
					<Button variant="outline" size="sm" class="shrink-0" onclick={archive} disabled={publishing}>
						<Archive class="mr-1 h-3 w-3" />{t('page_builder.archive', 'Archive')}
					</Button>
				{:else if canPublishPages}
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
					{#each Object.entries(groupedBlockTypes) as [category, types] (category)}
						<p class="mb-1.5 mt-3 text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{category}</p>
						<div class="space-y-1.5">
							{#each types as [type, meta] (type)}
								<DraggableBlockType blockType={type} {meta} {getIcon} />
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
							{#if block.type === 'container'}
								<div class="ml-2">
									<button
										type="button"
										onclick={() => toggleContainerExpand(block.id)}
										class="text-[10px] text-[var(--muted)] hover:text-[var(--foreground)] px-2 py-0.5"
									>
										{expandedContainers.has(block.id) ? '▼' : '▶'} {(block.props as ContainerProps).children?.length ?? 0} children
									</button>
								</div>
								{#if expandedContainers.has(block.id)}
									<ContainerChildren
										containerId={block.id}
										children={(block.props as ContainerProps).children ?? []}
										layout={(block.props as ContainerProps).layout ?? 'grid'}
										columns={(block.props as ContainerProps).columns ?? 2}
										{selectedBlockId}
										{getIcon}
										onSelect={(id) => (selectedBlockId = id)}
										onReorder={reorderContainerChild}
										onRemove={removeContainerChild}
										onMoveOut={moveChildToTopLevel}
										onSpanChange={updateChildSpan}
									/>
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
						<LandingRenderer blocks={resolvedBlocks} geoCountry={previewCountry} />
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
										<select id="motion-preset" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.preset ?? 'none'} onchange={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? ({ ...b, motion: { ...(b.motion ?? { delay: 0 }), preset: e.currentTarget.value as any } } as Block) : b); unsavedChanges = true; }}>
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
												<input id="motion-delay" type="number" min="0" step="50" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.delay ?? 0} oninput={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? ({ ...b, motion: { ...(b.motion ?? { preset: 'none' }), delay: Number(e.currentTarget.value) } } as Block) : b); unsavedChanges = true; }} />
											</div>
											<div>
												<label for="motion-duration" class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.motion_duration', 'Duration (ms)')}</label>
												<input id="motion-duration" type="number" min="100" step="50" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.duration ?? 700} oninput={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? ({ ...b, motion: { ...(b.motion ?? { preset: 'none', delay: 0 }), duration: Number(e.currentTarget.value) } } as Block) : b); unsavedChanges = true; }} />
											</div>
										</div>
										<div>
											<label for="motion-easing" class="mb-1 block text-xs" style="color: var(--color-muted)">{t('landing.motion_easing', 'Easing')}</label>
											<select id="motion-easing" class="w-full rounded border px-2 py-1 text-sm" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" value={selectedBlock.motion?.easing ?? 'cubic-bezier(0.16, 1, 0.3, 1)'} onchange={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? ({ ...b, motion: { ...(b.motion ?? { preset: 'none', delay: 0 }), easing: e.currentTarget.value } } as Block) : b); unsavedChanges = true; }}>
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
											<input id="motion-threshold" type="range" min="0" max="1" step="0.05" class="w-full" value={selectedBlock.motion?.threshold ?? 0.1} oninput={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? ({ ...b, motion: { ...(b.motion ?? { preset: 'none', delay: 0 }), threshold: Number(e.currentTarget.value) } } as Block) : b); unsavedChanges = true; }} />
										</div>
										<label class="flex items-center justify-between text-xs">
											<span style="color: var(--color-foreground)">{t('landing.motion_once', 'Animate only once')}</span>
											<input type="checkbox" checked={selectedBlock.motion?.once ?? true} onchange={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? ({ ...b, motion: { ...(b.motion ?? { preset: 'none', delay: 0 }), once: e.currentTarget.checked } } as Block) : b); unsavedChanges = true; }} />
										</label>
										<button type="button" onclick={() => { blocks = blocks.map((b) => b.id === selectedBlock.id ? ({ ...b, motion: undefined } as Block) : b); unsavedChanges = true; }} class="w-full rounded border px-2 py-1 text-xs hover:bg-[var(--surface)]" style="border-color: var(--color-border); color: var(--color-muted)">
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

<!-- Save-as-template dialog -->
{#if showSaveAsTemplate}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" role="dialog" aria-modal="true">
		<div class="w-full max-w-md rounded-lg border p-6 shadow-lg" style="border-color: var(--color-border); background: var(--color-background)">
			<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('page_builder.save_as_template', 'Save as Template')}</h3>
			<div class="mt-4 space-y-3">
				<div>
					<label for="tpl-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('page_builder.template_name', 'Template name')}</label>
					<input id="tpl-name" type="text" bind:value={templateName} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
				</div>
				<div class="flex justify-end gap-2">
					<Button variant="ghost" onclick={() => { showSaveAsTemplate = false; }}>{t('common.cancel', 'Cancel')}</Button>
					<Button onclick={saveAsTemplate} disabled={savingTemplate || !templateName.trim()}>{savingTemplate ? t('common.saving', 'Saving...') : t('common.save', 'Save')}</Button>
				</div>
			</div>
		</div>
	</div>
{/if}

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
