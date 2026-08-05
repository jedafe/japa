<script lang="ts">
	// Ref: japaconnect-engineering/sections/16-page-builder.md
	// Unified Page Builder editor — handles all page types (landing, about,
	// service, event, custom) on the shared @japaconnect/landing-blocks library.
	// dnd-kit drag-and-drop + live LandingRenderer preview + per-type content
	// editor + BlockStylePanel + motion/visibility settings + geo preview.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { untrack, type Component } from 'svelte';
	import { api, pageBuilderAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DragDropProvider, DragOverlay } from '@dnd-kit/svelte';
	import { isSortable } from '@dnd-kit/svelte/sortable';
	import {
		LandingRenderer,
		blockRegistry,
		contentEditorRegistry,
		BlockStylePanel,
		getGeoTokens,
		resolveGeoCopy,
		type Block,
		type BlockStyle,
		type BlockMeta,
		type GeoContext,
	} from '@japaconnect/landing-blocks';
	import { DraggableBlockType, SortableSection } from '$lib/components/landing-editor';
	import { Button } from '$lib/components/ui/button';
	import { Tabs, Dialog } from 'bits-ui';
	import * as LucideIcons from 'lucide-svelte';
	import { ArrowLeft, Save, Plus, Search, LayoutTemplate, Rocket, Archive } from 'lucide-svelte';
	import MediaPickerModal from '$lib/components/ui/MediaPickerModal.svelte';

	// Cast for DI — the editors expect Component<any>, MediaPickerModal has bindable props
	const MediaPicker = MediaPickerModal as unknown as Component;

	const canManagePages = $derived(hasPermission('pages.manage'));
	const canPublishPages = $derived(hasPermission('pages.publish'));

	interface Theme { id: string; name: string; is_default?: boolean }
	interface GeoCountry { id: string; name: string; geo_code: string }
	interface PageData {
		id: string; title: string; slug: string; status: string;
		page_type?: string; theme_id?: string; blocks: Block[] | string;
	}

	let pageData = $state<{ id: string; title: string; slug: string; status: string; page_type: string; theme_id?: string } | null>(null);
	let blocks = $state<Block[]>([]);
	let themes = $state<Theme[]>([]);
	let countries = $state<GeoCountry[]>([]);
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

	const selectedBlock = $derived(blocks.find((b) => b.id === selectedBlockId) ?? null);
	const visibleBlocks = $derived(blocks.filter((b) => b.visible));

	const previewCtx: GeoContext = $derived({
		contentCountry: previewCountry,
		tokens: getGeoTokens(previewCountry, 'en'),
		locale: 'en',
	});
	const resolvedBlocks = $derived(resolveGeoCopy(visibleBlocks, previewCtx));
	const deviceWidth = $derived(deviceMode === 'desktop' ? 1280 : deviceMode === 'tablet' ? 768 : 375);

	// Geo countries for preview dropdown — sourced from /geo/countries (Rule 16).
	// 'global' is prepended as the no-geo-override default.
	const geoCountries = $derived(['global', ...countries.map((c) => c.geo_code)]);

	// Block palette — grouped by category from the canonical blockRegistry.
	const filteredBlockTypes = $derived.by(() => {
		const q = paletteSearch.toLowerCase().trim();
		const entries = Object.entries(blockRegistry) as [Block['type'], BlockMeta][];
		if (!q) return entries;
		return entries.filter(([type, meta]) => meta.label.toLowerCase().includes(q) || type.toLowerCase().includes(q));
	});
	const groupedBlockTypes = $derived.by(() => {
		const groups: Record<string, [Block['type'], BlockMeta][]> = {};
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
			const [pageRes, themesRes, countriesRes] = await Promise.all([
				api<PageData>(`/admin/pages/${pageId}`),
				pageBuilderAdminApiExtended.listThemes(),
				api<GeoCountry[]>('/geo/countries'),
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
			if (countriesRes.ok) countries = countriesRes.data ?? [];
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
	function makeBlock(type: Block['type']): Block {
		return { id: crypto.randomUUID(), type, visible: true, props: {} } as Block;
	}
	function addBlock(type: Block['type']): void {
		const nb = makeBlock(type);
		blocks = [...blocks, nb];
		selectedBlockId = nb.id;
		unsavedChanges = true;
	}
	function removeBlock(id: string): void {
		blocks = blocks.filter((b) => b.id !== id);
		if (selectedBlockId === id) selectedBlockId = null;
		unsavedChanges = true;
	}
	function duplicateBlock(id: string): void {
		const idx = blocks.findIndex((b) => b.id === id);
		if (idx < 0) return;
		const copy = { ...blocks[idx], id: crypto.randomUUID(), props: JSON.parse(JSON.stringify(blocks[idx].props)) } as Block;
		blocks = [...blocks.slice(0, idx + 1), copy, ...blocks.slice(idx + 1)];
		selectedBlockId = copy.id;
		unsavedChanges = true;
	}
	function toggleBlockVisibility(id: string): void {
		blocks = blocks.map((b) => (b.id === id ? { ...b, visible: !b.visible } : b));
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
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function updateBlockProps(id: string, props: any): void {
		blocks = blocks.map((b) => (b.id === id ? ({ ...b, props } as Block) : b));
		unsavedChanges = true;
	}
	function updateBlockStyle(id: string, style: BlockStyle): void {
		blocks = blocks.map((b) => (b.id === id ? ({ ...b, style } as Block) : b));
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

		// Drop from palette → add new block at target position (or append).
		if (sourceData?.source === 'palette') {
			const nb = makeBlock(sourceData.blockType as Block['type']);
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
		// Reorder within the sections list.
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
			blocks: JSON.stringify(blocks),
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
<div class="flex h-screen flex-col overflow-hidden">
	<!-- Top bar -->
	<header class="flex items-center justify-between border-b px-4 py-2" style="border-color: var(--color-border); background: var(--color-background)">
		<div class="flex items-center gap-3">
			<Button variant="ghost" size="sm" onclick={() => goto('/page-builder/pages')}><ArrowLeft class="mr-1 h-4 w-4" />{t('common.back', 'Back')}</Button>
			<input type="text" bind:value={pageTitle} class="rounded border px-2 py-1 text-sm font-medium" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" aria-label={t('page_builder.title', 'Title')} />
			{#if pageData?.page_type}<span class="rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider" style="background: var(--color-surface); color: var(--color-muted)">{pageData.page_type}</span>{/if}
			<span class="text-xs" style="color: var(--color-muted)">/{pageData?.slug} · {blocks.length} {t('page_builder.blocks', 'blocks')}</span>
			{#if unsavedChanges}<span class="h-2 w-2 rounded-full" style="background: var(--color-warning)" title={t('pageBuilder.unsaved_changes', 'Unsaved changes')}></span>{/if}
		</div>
		<div class="flex items-center gap-2">
			{#if successMessage}<span class="text-sm" style="color: var(--color-success)">{successMessage}</span>{/if}
			{#if errorMessage}<span class="text-sm" style="color: var(--color-danger)">{errorMessage}</span>{/if}
			<!-- Preview country (geo-copy preview) -->
			<select bind:value={previewCountry} class="rounded border px-2 py-1 text-xs" style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)" aria-label={t('landing.preview_country', 'Preview country')}>
				{#each geoCountries as c}<option value={c}>{c === 'global' ? t('landing.global', 'Global') : c}</option>{/each}
			</select>
			<!-- Device toggle -->
			<div class="flex gap-1">
				<button onclick={() => (deviceMode = 'desktop')} class="rounded px-2 py-1 text-xs {deviceMode === 'desktop' ? 'text-white' : 'border'}" style={deviceMode === 'desktop' ? 'background: var(--color-primary)' : 'border-color: var(--color-border)'}>{t('landing.device_desktop', 'Desktop')}</button>
				<button onclick={() => (deviceMode = 'tablet')} class="rounded px-2 py-1 text-xs {deviceMode === 'tablet' ? 'text-white' : 'border'}" style={deviceMode === 'tablet' ? 'background: var(--color-primary)' : 'border-color: var(--color-border)'}>{t('landing.device_tablet', 'Tablet')}</button>
				<button onclick={() => (deviceMode = 'mobile')} class="rounded px-2 py-1 text-xs {deviceMode === 'mobile' ? 'text-white' : 'border'}" style={deviceMode === 'mobile' ? 'background: var(--color-primary)' : 'border-color: var(--color-border)'}>{t('landing.device_mobile', 'Mobile')}</button>
			</div>
			<!-- Theme -->
			<select value={selectedThemeId} onchange={(e) => changeTheme(e.currentTarget.value)} class="rounded-md border px-2 py-1.5 text-xs" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" aria-label={t('page_builder.theme', 'Theme')}>
				<option value="">{t('page_builder.default_theme', 'Default Theme')}</option>
				{#each themes as th}<option value={th.id} selected={selectedThemeId === th.id}>{th.name}{th.is_default ? ' (default)' : ''}</option>{/each}
			</select>
			{#if canManagePages}
				<Button variant="outline" size="sm" onclick={() => { templateName = `${pageData?.title ?? 'Page'} Template`; showSaveAsTemplate = true; }}>
					<LayoutTemplate class="mr-1 h-3 w-3" />{t('page_builder.save_as_template', 'Save as Template')}
				</Button>
			{/if}
			<Button variant="outline" size="sm" onclick={saveDraft} disabled={saving || !unsavedChanges}>
				<Save class="mr-1 h-3 w-3" />{saving ? t('common.saving', 'Saving...') : t('page_builder.save_draft', 'Save Draft')}
			</Button>
			{#if pageData?.status === 'published'}
				<Button variant="outline" size="sm" onclick={archive} disabled={publishing}>
					<Archive class="mr-1 h-3 w-3" />{t('page_builder.archive', 'Archive')}
				</Button>
			{:else if canPublishPages}
				<Button size="sm" onclick={() => { publishError = ''; showPublishDialog = true; }} disabled={publishing}>
					<Rocket class="mr-1 h-3 w-3" />{t('page_builder.publish', 'Publish')}
				</Button>
			{/if}
		</div>
	</header>
	{#if saveError}
		<div class="px-4 py-2 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">{saveError}</div>
	{/if}

	<DragDropProvider onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
		<div class="flex min-h-0 flex-1">
			<!-- Left pane: palette + sections list -->
			<aside class="flex w-64 shrink-0 flex-col border-r" style="border-color: var(--color-border); background: var(--color-background)">
				<!-- Palette -->
				<div class="overflow-y-auto border-b p-2" style="border-color: var(--color-border); max-height: 50%">
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

				<!-- Sections list (sortable) -->
				<div class="flex min-h-0 flex-1 flex-col overflow-y-auto p-2">
					<h2 class="mb-2 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('landing.sections', 'Sections')}</h2>
					{#if blocks.length === 0}
						<p class="px-1 py-2 text-xs" style="color: var(--color-muted)">{t('pageBuilder.dropBlocks', 'Drag blocks here or click to add')}</p>
					{:else}
						{#each blocks as block, i (block.id)}
							{@const meta = blockRegistry[block.type]}
							<SortableSection {block} index={i} selected={selectedBlockId === block.id} {meta} {getIcon} onSelect={() => (selectedBlockId = block.id)} />
						{/each}
					{/if}
					<button onclick={() => addBlock('hero')} class="mt-2 w-full rounded border border-dashed py-1.5 text-xs hover:bg-[var(--surface)]" style="border-color: var(--color-border); color: var(--color-muted)">
						+ {t('landing.add_section', 'Add Section')}
					</button>
				</div>
			</aside>

			<!-- Center pane: live preview -->
			<main class="flex-1 overflow-y-auto p-4" style="background: var(--color-surface)">
				<div class="mx-auto shadow-lg" style="width: {deviceWidth}px; max-width: 100%; background: var(--color-background)">
					{#if resolvedBlocks.length > 0}
						<LandingRenderer blocks={resolvedBlocks} />
					{:else}
						<div class="flex h-full flex-col items-center justify-center rounded-xl border-2 border-dashed py-20" style="border-color: var(--color-border)">
							<Plus class="mb-3 h-8 w-8" style="color: var(--color-muted)" />
							<p class="text-sm font-medium" style="color: var(--color-foreground)">{t('pageBuilder.emptyCanvas', 'Build your page by adding blocks from the left panel')}</p>
						</div>
					{/if}
				</div>
			</main>

			<!-- Right pane: inspector -->
			<aside class="w-96 shrink-0 overflow-y-auto border-l" style="border-color: var(--color-border); background: var(--color-background)">
				{#if selectedBlock}
					{@const meta = blockRegistry[selectedBlock.type]}
					<div class="border-b p-3" style="border-color: var(--color-border)">
						<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">{meta?.label ?? selectedBlock.type}</h3>
					</div>
					<Tabs.Root bind:value={activeTab} class="flex h-full flex-col">
						<Tabs.List class="flex border-b" style="border-color: var(--color-border)">
							<Tabs.Trigger value="content" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'content' ? 'border-b-2 text-[var(--primary)]' : 'text-[var(--muted)]'}" style="border-color: var(--color-primary)">{t('landing.tab_content', 'Content')}</Tabs.Trigger>
							<Tabs.Trigger value="style" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'style' ? 'border-b-2 text-[var(--primary)]' : 'text-[var(--muted)]'}" style="border-color: var(--color-primary)">{t('landing.tab_style', 'Style')}</Tabs.Trigger>
							<Tabs.Trigger value="settings" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'settings' ? 'border-b-2 text-[var(--primary)]' : 'text-[var(--muted)]'}" style="border-color: var(--color-primary)">{t('landing.tab_settings', 'Settings')}</Tabs.Trigger>
						</Tabs.List>
						<Tabs.Content value="content" class="flex-1 overflow-y-auto p-3">
							{#if ContentEditor}
								{@const Editor = ContentEditor}
								<Editor props={selectedBlock.props} onchange={(v: unknown) => updateBlockProps(selectedBlock.id, v)} mediaPickerComponent={MediaPicker} geoCtx={previewCtx} />
							{:else}
								<p class="text-xs" style="color: var(--color-muted)">{t('pageBuilder.no_config', 'No content editor for this block type')}</p>
							{/if}
						</Tabs.Content>
						<Tabs.Content value="style" class="flex-1 overflow-y-auto p-3">
							<BlockStylePanel value={selectedBlock.style} onchange={(v: BlockStyle) => updateBlockStyle(selectedBlock.id, v)} mediaPickerComponent={MediaPicker} />
						</Tabs.Content>
						<Tabs.Content value="settings" class="flex-1 overflow-y-auto p-3">
							<div class="space-y-4">
								<label class="flex items-center justify-between text-sm">
									<span style="color: var(--color-foreground)">{t('landing.visible', 'Visible')}</span>
									<input type="checkbox" checked={selectedBlock.visible} onchange={() => toggleBlockVisibility(selectedBlock.id)} />
								</label>

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
		</div>

		<DragOverlay>
			{#if activeDragData}
				<div class="rounded-lg border px-4 py-3 text-sm font-medium shadow-lg" style="border-color: var(--color-primary); background: var(--color-background); color: var(--color-foreground)">
					{activeDragData.meta?.label ?? activeDragData.blockType ?? 'Block'}
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
