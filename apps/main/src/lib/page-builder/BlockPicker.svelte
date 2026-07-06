<script lang="ts">
	import { X, Search } from 'lucide-svelte';
	import {
		Layout,
		Type,
		Image,
		MousePointerClick,
		BarChart3,
		HelpCircle,
		Quote,
		GitCompare,
		FileText,
		Minus,
		List,
	} from 'lucide-svelte';
	import { getBlockTypes } from './registry';
	import type { BlockDefinition, BlockCategory } from './types';

	interface Props {
		open: boolean;
		onSelect: (blockType: string) => void;
		onClose: () => void;
	}

	let { open, onSelect, onClose }: Props = $props();

	let searchQuery = $state('');

	const ICON_MAP: Record<string, typeof Layout> = {
		layout: Layout,
		type: Type,
		image: Image,
		'mouse-pointer-click': MousePointerClick,
		'bar-chart-3': BarChart3,
		'help-circle': HelpCircle,
		quote: Quote,
		'git-compare': GitCompare,
		'file-text': FileText,
		minus: Minus,
		list: List,
	};

	const CATEGORY_LABELS: Record<BlockCategory, string> = {
		layout: 'Layout',
		content: 'Content',
		media: 'Media',
		interactive: 'Interactive',
		data: 'Data',
	};

	const CATEGORY_ORDER: BlockCategory[] = [
		'layout',
		'content',
		'media',
		'interactive',
		'data',
	];

	let allBlockTypes = $derived(getBlockTypes());

	let filteredBySearch = $derived.by(() => {
		const query = searchQuery.trim().toLowerCase();
		if (!query) return allBlockTypes;
		return allBlockTypes.filter(
			(bt) =>
				bt.name.toLowerCase().includes(query) ||
				bt.description.toLowerCase().includes(query) ||
				bt.type.toLowerCase().includes(query)
		);
	});

	let groupedBlocks = $derived.by(() => {
		const groups = new Map<string, BlockDefinition[]>();
		for (const category of CATEGORY_ORDER) {
			const items = filteredBySearch.filter((bt) => bt.category === category);
			if (items.length > 0) {
				groups.set(category, items);
			}
		}
		return groups;
	});

	let hasResults = $derived(filteredBySearch.length > 0);

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			onClose();
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			onClose();
		}
	}

	function handleSelect(blockType: string) {
		onSelect(blockType);
		searchQuery = '';
	}

	function getIconComponent(iconName: string): typeof Layout {
		return ICON_MAP[iconName] ?? Layout;
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-start justify-center bg-black/40 pt-20 backdrop-blur-sm"
		role="dialog"
		aria-modal="true"
		aria-label="Add a block"
		tabindex="-1"
		onclick={handleBackdropClick}
		onkeydown={handleKeydown}
	>
		<div
			class="flex max-h-[70vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-gray-700 dark:bg-gray-900"
		>
			<!-- Header -->
			<div class="flex items-center gap-3 border-b border-gray-100 px-5 py-4 dark:border-gray-800">
				<h2 class="flex-1 text-base font-semibold text-gray-900 dark:text-gray-100">
					Add a Block
				</h2>
				<button
					type="button"
					class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
					aria-label="Close block picker"
					onclick={onClose}
				>
					<X class="h-5 w-5" />
				</button>
			</div>

			<!-- Search -->
			<div class="border-b border-gray-100 px-5 py-3 dark:border-gray-800">
				<div class="relative">
					<Search
						class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
					/>
					<input
						type="text"
						placeholder="Search blocks..."
						bind:value={searchQuery}
						class="w-full rounded-lg border border-gray-200 bg-gray-50 py-2 pl-9 pr-3 text-sm text-gray-900 placeholder-gray-400 outline-none transition-colors focus:border-blue-400 focus:bg-white focus:ring-2 focus:ring-blue-400/20 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-500 dark:focus:border-blue-500 dark:focus:bg-gray-800"
					/>
				</div>
			</div>

			<!-- Block list -->
			<div class="flex-1 overflow-y-auto px-5 py-4">
				{#if hasResults}
					{#each [...groupedBlocks.entries()] as [category, items]}
						<div class="mb-4 last:mb-0">
							<h3 class="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">
								{CATEGORY_LABELS[category as BlockCategory]}
							</h3>
							<div class="grid grid-cols-1 gap-1.5">
								{#each items as block}
									{@const Icon = getIconComponent(block.icon)}
									<button
										type="button"
										class="flex items-start gap-3 rounded-lg px-3 py-2.5 text-left transition-colors hover:bg-gray-50 dark:hover:bg-gray-800"
										onclick={() => handleSelect(block.type)}
									>
										<span
											class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400"
										>
											<Icon class="h-4 w-4" />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block text-sm font-medium text-gray-900 dark:text-gray-100">
												{block.name}
											</span>
											<span class="block text-xs text-gray-500 dark:text-gray-400">
												{block.description}
											</span>
										</span>
									</button>
								{/each}
							</div>
						</div>
					{/each}
				{:else}
					<div class="flex flex-col items-center justify-center py-12 text-center">
						<p class="text-sm text-gray-500 dark:text-gray-400">
							No blocks match "{searchQuery}"
						</p>
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
