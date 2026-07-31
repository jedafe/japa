<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Button } from '$lib/components/ui/button';
	import { pageBuilderApi, pageBuilderAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { ArrowLeft, ArrowRight, Plus, Save, Trash2, GripVertical, Eye, EyeOff } from 'lucide-svelte';

	type Block = { id: string; block_type: string; order: number; props: Record<string, unknown> };
	type BlockType = { type: string; name: string; description: string; icon: string; category: string; default_props: Record<string, unknown> };

	let activePosition = $state<'left' | 'right'>('left');
	let leftBlocks = $state<Block[]>([]);
	let rightBlocks = $state<Block[]>([]);
	let leftMeta = $state<Record<string, unknown>>({});
	let rightMeta = $state<Record<string, unknown>>({});
	let blockTypes = $state<BlockType[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let paletteSearch = $state('');
	let selectedBlockId = $state<string | null>(null);
	let showLeftEnabled = $state(true);
	let showRightEnabled = $state(true);
	let counter = $state(0);

	const canManage = $derived(hasPermission('pages.manage'));

	const currentBlocks = $derived(activePosition === 'left' ? leftBlocks : rightBlocks);
	const currentShowEnabled = $derived(activePosition === 'left' ? showLeftEnabled : showRightEnabled);

	const filteredBlockTypes = $derived.by(() => {
		const q = paletteSearch.toLowerCase().trim();
		if (!q) return blockTypes;
		return blockTypes.filter((bt) => bt.name.toLowerCase().includes(q) || bt.type.toLowerCase().includes(q) || bt.category.toLowerCase().includes(q));
	});

	const groupedBlockTypes = $derived.by(() => {
		const groups: Record<string, BlockType[]> = {};
		for (const bt of filteredBlockTypes) {
			(groups[bt.category] ??= []).push(bt);
		}
		return groups;
	});

	$effect(() => { load(); });

	async function load() {
		loading = true;
		try {
			const [configRes, typesRes] = await Promise.all([
				pageBuilderAdminApiExtended.getSidebarConfig(),
				pageBuilderApi.getBlockTypes()
			]);
			if (configRes.ok && configRes.data) {
				const data = configRes.data as { left?: any; right?: any };
				if (data.left) {
					leftBlocks = Array.isArray(data.left.blocks) ? data.left.blocks : [];
					leftMeta = data.left.meta ?? {};
					showLeftEnabled = data.left.meta?.show_left_sidebar !== false;
				}
				if (data.right) {
					rightBlocks = Array.isArray(data.right.blocks) ? data.right.blocks : [];
					rightMeta = data.right.meta ?? {};
					showRightEnabled = data.right.meta?.show_right_sidebar !== false;
				}
			}
			if (typesRes.ok) {
				blockTypes = typesRes.data?.block_types ?? [];
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function addBlock(bt: BlockType) {
		const newBlock: Block = {
			id: `new-${Date.now()}-${counter++}`,
			block_type: bt.type,
			order: currentBlocks.length,
			props: { ...bt.default_props }
		};
		if (activePosition === 'left') {
			leftBlocks = [...leftBlocks, newBlock];
		} else {
			rightBlocks = [...rightBlocks, newBlock];
		}
		selectedBlockId = newBlock.id;
	}

	function deleteBlock(id: string) {
		if (activePosition === 'left') {
			leftBlocks = leftBlocks.filter((b) => b.id !== id);
		} else {
			rightBlocks = rightBlocks.filter((b) => b.id !== id);
		}
		if (selectedBlockId === id) selectedBlockId = null;
		reindex();
	}

	function duplicateBlock(id: string) {
		const blocks = activePosition === 'left' ? leftBlocks : rightBlocks;
		const idx = blocks.findIndex((b) => b.id === id);
		if (idx < 0) return;
		const orig = blocks[idx];
		const dup: Block = {
			id: `new-${Date.now()}-${counter++}`,
			block_type: orig.block_type,
			order: idx + 1,
			props: JSON.parse(JSON.stringify(orig.props))
		};
		const next = [...blocks.slice(0, idx + 1), dup, ...blocks.slice(idx + 1)];
		if (activePosition === 'left') leftBlocks = next;
		else rightBlocks = next;
		reindex();
	}

	function moveBlock(id: string, dir: -1 | 1) {
		const blocks = activePosition === 'left' ? leftBlocks : rightBlocks;
		const idx = blocks.findIndex((b) => b.id === id);
		const swap = idx + dir;
		if (idx < 0 || swap < 0 || swap >= blocks.length) return;
		const next = [...blocks];
		[next[idx], next[swap]] = [next[swap], next[idx]];
		if (activePosition === 'left') leftBlocks = next;
		else rightBlocks = next;
		reindex();
	}

	function reindex() {
		leftBlocks = leftBlocks.map((b, i) => ({ ...b, order: i }));
		rightBlocks = rightBlocks.map((b, i) => ({ ...b, order: i }));
	}

	function updateBlockProps(id: string, props: Record<string, unknown>) {
		if (activePosition === 'left') {
			leftBlocks = leftBlocks.map((b) => (b.id === id ? { ...b, props } : b));
		} else {
			rightBlocks = rightBlocks.map((b) => (b.id === id ? { ...b, props } : b));
		}
	}

	function toggleSidebarVisibility() {
		if (activePosition === 'left') {
			showLeftEnabled = !showLeftEnabled;
			leftMeta = { ...leftMeta, show_left_sidebar: showLeftEnabled };
		} else {
			showRightEnabled = !showRightEnabled;
			rightMeta = { ...rightMeta, show_right_sidebar: showRightEnabled };
		}
	}

	async function save() {
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const blocks = activePosition === 'left' ? leftBlocks : rightBlocks;
			const meta = activePosition === 'left' ? leftMeta : rightMeta;
			const result = await pageBuilderAdminApiExtended.updateSidebar(activePosition, {
				blocks: blocks.map((b, i) => ({ block_id: b.id, block_type: b.block_type, order: i, props: b.props })),
				meta
			});
			if (result.ok) {
				successMessage = t('pageBuilder.saved', 'Saved!');
				setTimeout(() => (successMessage = ''), 2000);
			} else {
				errorMessage = (result as any).error ?? t('common.error');
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function saveBoth() {
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const [leftRes, rightRes] = await Promise.all([
				pageBuilderAdminApiExtended.updateSidebar('left', {
					blocks: leftBlocks.map((b, i) => ({ block_id: b.id, block_type: b.block_type, order: i, props: b.props })),
					meta: { ...leftMeta, show_left_sidebar: showLeftEnabled }
				}),
				pageBuilderAdminApiExtended.updateSidebar('right', {
					blocks: rightBlocks.map((b, i) => ({ block_id: b.id, block_type: b.block_type, order: i, props: b.props })),
					meta: { ...rightMeta, show_right_sidebar: showRightEnabled }
				})
			]);
			if (leftRes.ok && rightRes.ok) {
				successMessage = t('pageBuilder.saved', 'Saved!');
				setTimeout(() => (successMessage = ''), 2000);
			} else {
				errorMessage = (!(leftRes as any).ok ? (leftRes as any).error : '') || (!(rightRes as any).ok ? (rightRes as any).error : '') || t('common.error');
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	function propLabel(key: string): string {
		return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}
</script>

<svelte:head>
	<title>{t('sidebars.title', 'Sidebar Manager')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('sidebars.title', 'Sidebar Manager')}
	description={t('sidebars.description', 'Manage sitewide sidebar blocks for left and right sidebars')}
	actions={canManage ? [{ label: t('sidebars.save_both', 'Save Both'), icon: Save, onclick: saveBoth }] : []}
/>

{#if !canManage}
	<div class="rounded-lg p-4 text-sm" style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)">
		{t('sidebars.no_permission', 'You do not have permission to manage sidebars.')}
	</div>
{:else if loading}
	<div class="flex h-64 items-center justify-center">
		<p style="color: var(--color-muted)">{t('common.loading')}</p>
	</div>
{:else}
	{#if errorMessage}
		<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
			{successMessage}
		</div>
	{/if}

	<!-- Position tabs -->
	<div class="mb-4 flex gap-2">
		<button
			class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors"
			style={activePosition === 'left' ? 'border-color: var(--color-primary); background: var(--color-primary); color: white' : 'border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)'}
			onclick={() => { activePosition = 'left'; selectedBlockId = null; }}
		>
			<ArrowLeft class="mr-1 inline h-4 w-4" />
			{t('sidebars.left_sidebar', 'Left Sidebar')}
			<span class="ml-2 rounded-full px-2 py-0.5 text-xs" style={showLeftEnabled ? 'background: color-mix(in srgb, var(--color-success) 20%, transparent); color: var(--color-success)' : 'background: color-mix(in srgb, var(--color-muted) 20%, transparent); color: var(--color-muted)'}>
				{showLeftEnabled ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}
			</span>
		</button>
		<button
			class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors"
			style={activePosition === 'right' ? 'border-color: var(--color-primary); background: var(--color-primary); color: white' : 'border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)'}
			onclick={() => { activePosition = 'right'; selectedBlockId = null; }}
		>
			<ArrowRight class="mr-1 inline h-4 w-4" />
			{t('sidebars.right_sidebar', 'Right Sidebar')}
			<span class="ml-2 rounded-full px-2 py-0.5 text-xs" style={showRightEnabled ? 'background: color-mix(in srgb, var(--color-success) 20%, transparent); color: var(--color-success)' : 'background: color-mix(in srgb, var(--color-muted) 20%, transparent); color: var(--color-muted)'}>
				{showRightEnabled ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}
			</span>
		</button>
	</div>

	<div class="flex gap-4">
		<!-- Left: Block Palette -->
		<div class="w-64 shrink-0 overflow-y-auto rounded-lg border p-3" style="border-color: var(--color-border); background: var(--color-surface); max-height: calc(100vh - 280px)">
			<h2 class="mb-3 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('pageBuilder.blockTypes', 'Block Types')}</h2>
			<input type="text" bind:value={paletteSearch} placeholder={t('common.search')} class="mb-3 w-full rounded-md border px-2 py-1.5 text-xs" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
			{#each Object.entries(groupedBlockTypes) as [category, types] (category)}
				<p class="mb-1.5 mt-3 text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{category}</p>
				<div class="space-y-1.5">
					{#each types as bt (bt.type)}
						<button
							class="flex w-full items-center gap-2 rounded-md border px-3 py-2 text-left text-xs transition-colors hover:opacity-80"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
							onclick={() => addBlock(bt)}
						>
							<Plus class="h-3 w-3 shrink-0" style="color: var(--color-primary)" />
							<span class="flex-1 font-medium">{bt.name}</span>
						</button>
					{/each}
				</div>
			{/each}
		</div>

		<!-- Center: Canvas -->
		<div class="flex-1 overflow-y-auto rounded-lg border p-4" style="border-color: var(--color-border); background: var(--color-surface); max-height: calc(100vh - 280px)">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">
					{activePosition === 'left' ? t('sidebars.left_sidebar', 'Left Sidebar') : t('sidebars.right_sidebar', 'Right Sidebar')}
					<span class="ml-2 text-xs font-normal" style="color: var(--color-muted)">({currentBlocks.length} {t('sidebars.blocks', 'blocks')})</span>
				</h2>
				<Button variant="ghost" size="sm" onclick={toggleSidebarVisibility}>
					{#if currentShowEnabled}<Eye class="mr-1 h-3.5 w-3.5" />{:else}<EyeOff class="mr-1 h-3.5 w-3.5" />{/if}
					{currentShowEnabled ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}
				</Button>
			</div>

			{#if currentBlocks.length === 0}
				<div class="flex h-48 flex-col items-center justify-center rounded-xl border-2 border-dashed" style="border-color: var(--color-border)">
					<Plus class="mb-3 h-8 w-8" style="color: var(--color-muted)" />
					<p class="text-sm font-medium" style="color: var(--color-foreground)">{t('pageBuilder.dropBlocks', 'Click blocks from the left to add')}</p>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('sidebars.empty_hint', 'Sidebar blocks appear sitewide on all pages')}</p>
				</div>
			{:else}
				<div class="space-y-2">
					{#each currentBlocks as block, i (block.id)}
						<div
							class="rounded-lg border p-3 transition-colors"
							style={selectedBlockId === block.id ? 'border-color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 5%, transparent)' : 'border-color: var(--color-border); background: var(--color-background)'}
						>
							<div class="flex items-center gap-2">
								<GripVertical class="h-4 w-4 shrink-0 cursor-grab" style="color: var(--color-muted)" />
								<button class="flex-1 text-left text-sm font-medium" style="color: var(--color-foreground)" onclick={() => selectedBlockId = selectedBlockId === block.id ? null : block.id}>
									{block.block_type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
								</button>
								<button class="rounded p-1 text-xs hover:opacity-70" style="color: var(--color-muted)" onclick={() => moveBlock(block.id, -1)} disabled={i === 0} aria-label="Move up">↑</button>
								<button class="rounded p-1 text-xs hover:opacity-70" style="color: var(--color-muted)" onclick={() => moveBlock(block.id, 1)} disabled={i === currentBlocks.length - 1} aria-label="Move down">↓</button>
								<button class="rounded p-1 hover:opacity-70" style="color: var(--color-muted)" onclick={() => duplicateBlock(block.id)} aria-label={t('common.duplicate', 'Duplicate')}>⧉</button>
								<button class="rounded p-1 hover:opacity-70" style="color: var(--color-danger)" onclick={() => deleteBlock(block.id)} aria-label={t('common.delete', 'Delete')}><Trash2 class="h-3.5 w-3.5" /></button>
							</div>
							{#if selectedBlockId === block.id}
								<div class="mt-3 space-y-3 border-t pt-3" style="border-color: var(--color-border)">
									{#each Object.entries(block.props) as [key, value] (key)}
										<div>
											<label for="prop-{block.id}-{key}" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{propLabel(key)}</label>
											{#if typeof value === 'boolean'}
												<input id="prop-{block.id}-{key}" type="checkbox" checked={value} onchange={(e) => updateBlockProps(block.id, { ...block.props, [key]: e.currentTarget.checked })} class="rounded" />
											{:else if typeof value === 'number'}
												<input id="prop-{block.id}-{key}" type="number" value={value} oninput={(e) => updateBlockProps(block.id, { ...block.props, [key]: parseFloat(e.currentTarget.value) })} class="w-full rounded-md border px-2 py-1 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
											{:else if Array.isArray(value)}
												<textarea
													id="prop-{block.id}-{key}"
													value={JSON.stringify(value, null, 2)}
													oninput={(e) => { try { updateBlockProps(block.id, { ...block.props, [key]: JSON.parse(e.currentTarget.value) }) } catch {} }}
													rows="3"
													class="w-full rounded-md border px-2 py-1 text-xs font-mono"
													style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												></textarea>
											{:else}
												<input
													id="prop-{block.id}-{key}"
													type="text"
													value={value ?? ''}
													oninput={(e) => updateBlockProps(block.id, { ...block.props, [key]: e.currentTarget.value })}
													class="w-full rounded-md border px-2 py-1 text-xs"
													style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												/>
											{/if}
										</div>
									{/each}
									{#if Object.keys(block.props).length === 0}
										<p class="text-xs" style="color: var(--color-muted)">{t('sidebars.no_props', 'No configurable properties for this block.')}</p>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

			<div class="mt-4 flex justify-end">
				<Button onclick={save} disabled={saving || !canManage}>
					<Save class="mr-1 h-3.5 w-3.5" />
					{saving ? t('common.saving') : t('sidebars.save', 'Save Sidebar')}
				</Button>
			</div>
		</div>
	</div>
{/if}
