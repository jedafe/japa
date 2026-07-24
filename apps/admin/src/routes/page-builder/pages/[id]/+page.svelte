<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, pageBuilderApi, pageTemplatesAdminApiExtended, pageBuilderAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { DragDropProvider, DragOverlay } from '@dnd-kit/svelte';
	import { isSortable } from '@dnd-kit/svelte/sortable';
	import { DraggableBlockType, SortableBlock, BlockConfigPanel } from '$lib/components/page-builder';
	import { Button } from '$lib/components/ui/button';
	import { ArrowLeft, Save, Eye, Plus, Search, LayoutTemplate } from 'lucide-svelte';

	type Block = { id: string; block_type: string; order: number; props: Record<string, unknown> };
	type BlockType = { type: string; name: string; description: string; icon: string; category: string; default_props: Record<string, unknown> };

	let pageData = $state<{ id: string; title: string; slug: string; status: string; blocks: Block[] } | null>(null);
	let blocks = $state<Block[]>([]);
	let blockTypes = $state<BlockType[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let selectedBlockId = $state<string | null>(null);
	let paletteSearch = $state('');
	let activeDragData = $state<{ blockType?: string; defaultProps?: Record<string, unknown>; id?: string } | null>(null);

	// Theme + save-as-template wiring
	let themes = $state<{ id: string; name: string }[]>([]);
	let selectedThemeId = $state<string>('');
	let showSaveAsTemplate = $state(false);
	let templateName = $state('');
	let savingTemplate = $state(false);

	const selectedBlock = $derived(blocks.find((b) => b.id === selectedBlockId) ?? null);

	const filteredBlockTypes = $derived.by(() => {
		const q = paletteSearch.toLowerCase().trim();
		if (!q) return blockTypes;
		return blockTypes.filter((bt) => bt.name.toLowerCase().includes(q) || bt.type.toLowerCase().includes(q));
	});

	const groupedBlockTypes = $derived.by(() => {
		const groups: Record<string, BlockType[]> = {};
		for (const bt of filteredBlockTypes) {
			(groups[bt.category] ??= []).push(bt);
		}
		return groups;
	});

	let counter = $state(0);

	$effect(() => { load(); });

	async function load() {
		loading = true;
		try {
			const pageId = page.params.id;
			const [pageRes, typesRes, themesRes] = await Promise.all([
				api<{ id: string; title: string; slug: string; status: string; theme_id?: string; blocks: Block[] }>(`/admin/pages/${pageId}`),
				pageBuilderApi.getBlockTypes(),
				pageBuilderAdminApiExtended.listThemes()
			]);
			if (!pageRes.ok) {
				errorMessage = pageRes.error || 'Failed to load page';
			} else if (pageRes.data) {
				pageData = pageRes.data;
				blocks = Array.isArray(pageRes.data.blocks) ? pageRes.data.blocks : [];
				selectedThemeId = pageRes.data.theme_id ?? '';
			}
			if (typesRes.ok) {
				blockTypes = typesRes.data?.block_types ?? [];
			}
			if (themesRes.ok) {
				themes = (themesRes.data as any[])?.map((th) => ({ id: th.id, name: th.name })) ?? [];
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function handleDragStart(event: any) {
		activeDragData = event?.operation?.source?.data ?? null;
	}

	function handleDragEnd(event: any) {
		activeDragData = null;
		if (event?.canceled) return;
		const { source, target } = event.operation ?? {};
		if (!source) return;

		const sourceData = source.data;
		// Drop from palette → add new block
		if (sourceData?.source === 'palette') {
			const targetType = target?.data?.blockType;
			const newBlock: Block = {
				id: `new-${Date.now()}-${counter++}`,
				block_type: sourceData.blockType,
				order: 0,
				props: { ...(sourceData.defaultProps ?? {}) }
			};
			if (target && isSortable(target)) {
				const targetIdx = blocks.findIndex((b) => b.id === target.id);
				if (targetIdx >= 0) {
					blocks = [...blocks.slice(0, targetIdx), newBlock, ...blocks.slice(targetIdx)];
				} else {
					blocks = [...blocks, newBlock];
				}
			} else {
				blocks = [...blocks, newBlock];
			}
			reindex();
			selectedBlockId = newBlock.id;
			return;
		}
		// Reorder within canvas
		if (sourceData?.source === 'canvas' && target && isSortable(target)) {
			const fromIdx = blocks.findIndex((b) => b.id === source.id);
			const toIdx = blocks.findIndex((b) => b.id === target.id);
			if (fromIdx < 0 || toIdx < 0 || fromIdx === toIdx) return;
			const moved = blocks[fromIdx];
			const next = blocks.filter((_, i) => i !== fromIdx);
			next.splice(toIdx, 0, moved);
			blocks = next;
			reindex();
		}
	}

	function reindex() { blocks = blocks.map((b, i) => ({ ...b, order: i })); }

	function addBlock(bt: BlockType) {
		const newBlock: Block = { id: `new-${Date.now()}-${counter++}`, block_type: bt.type, order: blocks.length, props: { ...bt.default_props } };
		blocks = [...blocks, newBlock];
		selectedBlockId = newBlock.id;
	}

	function deleteBlock(id: string) {
		blocks = blocks.filter((b) => b.id !== id);
		if (selectedBlockId === id) selectedBlockId = null;
		reindex();
	}

	function duplicateBlock(id: string) {
		const idx = blocks.findIndex((b) => b.id === id);
		if (idx < 0) return;
		const orig = blocks[idx];
		const dup: Block = { id: `new-${Date.now()}-${counter++}`, block_type: orig.block_type, order: idx + 1, props: JSON.parse(JSON.stringify(orig.props)) };
		blocks = [...blocks.slice(0, idx + 1), dup, ...blocks.slice(idx + 1)];
		reindex();
	}

	function moveBlock(id: string, dir: -1 | 1) {
		const idx = blocks.findIndex((b) => b.id === id);
		const swap = idx + dir;
		if (idx < 0 || swap < 0 || swap >= blocks.length) return;
		const next = [...blocks];
		[next[idx], next[swap]] = [next[swap], next[idx]];
		blocks = next;
		reindex();
	}

	function updateBlockProps(id: string, props: Record<string, unknown>) {
		blocks = blocks.map((b) => (b.id === id ? { ...b, props } : b));
	}

	async function save() {
		if (!pageData) return;
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const body: Record<string, unknown> = {
				title: pageData.title,
				slug: pageData.slug,
				status: pageData.status,
				blocks: blocks.map((b, i) => ({ block_id: b.id, block_type: b.block_type, order: i, props: b.props }))
			};
			if (selectedThemeId) body.theme_id = selectedThemeId;
			const result = await api(`/admin/pages/${pageData.id}`, { method: 'PATCH', body });
			if (result.ok) {
				successMessage = t('pageBuilder.saved', 'Saved!');
				setTimeout(() => (successMessage = ''), 2000);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function changeTheme(themeId: string) {
		if (!pageData) return;
		selectedThemeId = themeId;
		try {
			const body: Record<string, unknown> = { title: pageData.title, slug: pageData.slug, status: pageData.status, blocks: blocks.map((b, i) => ({ block_id: b.id, block_type: b.block_type, order: i, props: b.props })) };
			if (themeId) body.theme_id = themeId;
			await api(`/admin/pages/${pageData.id}`, { method: 'PATCH', body });
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function saveAsTemplate() {
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
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			savingTemplate = false;
		}
	}
</script>

<svelte:head>
	<title>{pageData?.title ?? 'Page Editor'} — JapaConnect Admin</title>
</svelte:head>

{#if loading}
	<div class="flex h-[calc(100vh-4rem)] items-center justify-center">
		<p style="color: var(--color-muted)">{t('common.loading')}</p>
	</div>
{:else if errorMessage && !pageData}
	<div class="flex h-[calc(100vh-4rem)] flex-col items-center justify-center gap-4">
		<p class="text-sm" style="color: var(--color-danger)">{errorMessage}</p>
		<Button variant="ghost" onclick={() => goto('/page-builder/pages')}><ArrowLeft class="mr-2 h-4 w-4" />Back</Button>
	</div>
{:else}
	<div class="flex h-screen flex-col overflow-hidden">
		<!-- Top bar -->
		<div class="flex items-center justify-between border-b px-4 py-2" style="border-color: var(--color-border); background: var(--color-background)">
			<div class="flex items-center gap-3">
				<Button variant="ghost" size="sm" onclick={() => goto('/page-builder/pages')}><ArrowLeft class="mr-1 h-4 w-4" />{t('common.back', 'Back')}</Button>
				<div>
					<h1 class="text-sm font-semibold" style="color: var(--color-foreground)">{pageData?.title}</h1>
					<p class="text-xs" style="color: var(--color-muted)">/{pageData?.slug} · {blocks.length} blocks</p>
				</div>
			</div>
			<div class="flex items-center gap-2">
				{#if successMessage}<span class="text-sm text-success">{successMessage}</span>{/if}
				{#if errorMessage}<span class="text-sm" style="color: var(--color-danger)">{errorMessage}</span>{/if}
				<select value={selectedThemeId} onchange={(e) => changeTheme(e.currentTarget.value)} class="rounded-md border px-2 py-1.5 text-xs" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)">
					<option value="">{t('page_builder.default_theme', 'Default Theme')}</option>
					{#each themes as th}<option value={th.id} selected={selectedThemeId === th.id}>{th.name}</option>{/each}
				</select>
				<Button variant="outline" size="sm" onclick={() => { templateName = `${pageData?.title ?? 'Page'} Template`; showSaveAsTemplate = true; }}>
					<LayoutTemplate class="mr-1 h-3 w-3" />{t('page_builder.save_as_template')}
				</Button>
				<Button onclick={save} disabled={saving}>{saving ? t('common.saving') : t('common.save')}</Button>
			</div>
		</div>

		<!-- 3-panel layout -->
		<DragDropProvider onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
			<div class="flex min-h-0 flex-1">
				<!-- Left: Block Palette -->
				<div class="w-64 shrink-0 overflow-y-auto border-r p-3" style="border-color: var(--color-border); background: var(--color-surface)">
					<h2 class="mb-3 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('pageBuilder.blockTypes', 'Block Types')}</h2>
					<div class="relative mb-3">
						<Search class="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2" style="color: var(--color-muted)" />
						<input type="text" bind:value={paletteSearch} placeholder={t('common.search')} class="w-full rounded-md border py-1.5 pl-8 pr-2 text-xs" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
					</div>
					{#each Object.entries(groupedBlockTypes) as [category, types] (category)}
						<p class="mb-1.5 mt-3 text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{category}</p>
						<div class="space-y-1.5">
							{#each types as bt (bt.type)}
								<DraggableBlockType blockType={bt} />
							{/each}
						</div>
					{/each}
				</div>

				<!-- Center: Block Canvas -->
				<div class="flex-1 overflow-y-auto p-6" style="background: var(--color-surface)">
					{#if blocks.length === 0}
						<div class="flex h-full flex-col items-center justify-center rounded-xl border-2 border-dashed" style="border-color: var(--color-border)">
							<Plus class="mb-3 h-8 w-8" style="color: var(--color-muted)" />
							<p class="text-sm font-medium" style="color: var(--color-foreground)">{t('pageBuilder.dropBlocks', 'Drag blocks here or click to add')}</p>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('pageBuilder.emptyCanvas', 'Build your page by adding blocks from the left panel')}</p>
						</div>
					{:else}
						<div class="mx-auto max-w-3xl space-y-3">
							{#each blocks as block, i (block.id)}
								<SortableBlock {block} index={i} totalBlocks={blocks.length} selected={selectedBlockId === block.id} onSelect={() => (selectedBlockId = block.id)} onDelete={() => deleteBlock(block.id)} onDuplicate={() => duplicateBlock(block.id)} onMoveUp={() => moveBlock(block.id, -1)} onMoveDown={() => moveBlock(block.id, 1)} />
							{/each}
						</div>
					{/if}
				</div>

				<!-- Right: Config Panel -->
				{#if selectedBlock}
					<div class="w-80 shrink-0 overflow-hidden border-l" style="border-color: var(--color-border); background: var(--color-background)">
						<BlockConfigPanel block={selectedBlock} onUpdate={updateBlockProps} onClose={() => (selectedBlockId = null)} />
					</div>
				{/if}
			</div>

			<DragOverlay>
				{#if activeDragData}
					<div class="rounded-lg border px-4 py-3 text-sm font-medium shadow-lg" style="border-color: var(--color-primary); background: var(--color-background); color: var(--color-foreground)">
						{activeDragData.blockType?.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase()) ?? 'Block'}
					</div>
				{/if}
			</DragOverlay>
		</DragDropProvider>
	</div>
{/if}

{#if showSaveAsTemplate}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" role="dialog" aria-modal="true">
		<div class="w-full max-w-md rounded-lg border p-6 shadow-lg" style="border-color: var(--color-border); background: var(--color-background)">
			<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('page_builder.save_as_template')}</h3>
			<div class="mt-4 space-y-3">
				<div>
					<label for="tpl-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('page_builder.template_name')}</label>
					<input id="tpl-name" type="text" bind:value={templateName} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
				</div>
				<div class="flex justify-end gap-2">
					<Button variant="ghost" onclick={() => { showSaveAsTemplate = false; }}>{t('common.cancel')}</Button>
					<Button onclick={saveAsTemplate} disabled={savingTemplate || !templateName.trim()}>{savingTemplate ? t('common.saving') : t('common.save')}</Button>
				</div>
			</div>
		</div>
	</div>
{/if}