<script lang="ts">
	import { DragDropProvider } from '@dnd-kit/svelte';
	import { isSortableOperation } from '@dnd-kit/svelte/sortable';
	import type { DragEndEvent } from '@dnd-kit/abstract';
	import type { BlockInstance } from './types';
	import { getBlockTypeByType, createBlockInstance } from './registry';
	import SortableBlock from './SortableBlock.svelte';
	import {
		HeroBlock,
		RichTextBlock,
		ImageBlock,
		CTABlock,
		StatsBlock,
		FAQBlock,
		TestimonialBlock,
		ComparisonBlock,
		FormEmbedBlock,
		SpacerBlock,
		DynamicListingBlock,
	} from './blocks/index.js';

	interface Props {
		blocks: BlockInstance[];
		readOnly?: boolean;
		onChange?: (blocks: BlockInstance[]) => void;
	}

	let { blocks, readOnly = false, onChange }: Props = $props();

	/** Map block_type to its Svelte component. */
	const BLOCK_COMPONENTS: Record<string, typeof HeroBlock> = {
		hero: HeroBlock,
		rich_text: RichTextBlock,
		image: ImageBlock,
		cta: CTABlock,
		stats: StatsBlock,
		faq: FAQBlock,
		testimonial: TestimonialBlock,
		comparison: ComparisonBlock,
		form: FormEmbedBlock,
		spacer: SpacerBlock,
		dynamic_listing: DynamicListingBlock,
	};

	// ── DnD event handlers ──────────────────────────────────────────

	function handleDragEnd(event: DragEndEvent) {
		if (readOnly || event.canceled) return;

		const { operation } = event;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		if (!isSortableOperation(operation as any)) return;

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const op = operation as any;
		const source = op.source;
		if (!source) return;

		const sourceIndex = source.initialIndex as number;
		const targetIndex = op.target?.index as number | undefined;

		if (targetIndex == null || sourceIndex === targetIndex) return;

		const updated = [...blocks];
		const [moved] = updated.splice(sourceIndex, 1);
		updated.splice(targetIndex, 0, moved);
		emitChange(updated);
	}

	// ── Block CRUD ───────────────────────────────────────────────────

	function deleteBlock(id: string) {
		emitChange(blocks.filter((b) => b.block_id !== id));
	}

	function duplicateBlock(id: string) {
		const original = blocks.find((b) => b.block_id === id);
		if (!original) return;

		const clone: BlockInstance = {
			block_id: crypto.randomUUID(),
			block_type: original.block_type,
			order: original.order,
			props: structuredClone(original.props),
		};

		const idx = blocks.findIndex((b) => b.block_id === id);
		const updated = [...blocks];
		updated.splice(idx + 1, 0, clone);
		emitChange(updated);
	}

	function addBlock(type: string) {
		const instance = createBlockInstance(type);
		instance.order = blocks.length;
		emitChange([...blocks, instance]);
	}

	// ── Propagate changes ────────────────────────────────────────────

	function emitChange(next: BlockInstance[]) {
		const reindexed = next.map((b, i) => ({ ...b, order: i }));
		onChange?.(reindexed);
	}

	// ── Helpers ──────────────────────────────────────────────────────

	function getBlockLabel(blockType: string): string {
		return getBlockTypeByType(blockType)?.name ?? blockType;
	}

	function getBlockComponent(blockType: string): typeof HeroBlock | undefined {
		return BLOCK_COMPONENTS[blockType];
	}

	let isEmpty = $derived(blocks.length === 0);
</script>

<DragDropProvider onDragEnd={handleDragEnd}>
	<div
		class="page-builder flex flex-col gap-3"
		role="list"
		aria-label="Page builder blocks"
	>
		{#each blocks as block, index (block.block_id)}
			{@const Component = getBlockComponent(block.block_type)}

			<SortableBlock
				id={block.block_id}
				blockType={block.block_type}
				blockLabel={getBlockLabel(block.block_type)}
				{index}
				{readOnly}
				onDelete={!readOnly ? deleteBlock : undefined}
				onDuplicate={!readOnly ? duplicateBlock : undefined}
			>
				{#if Component}
					<Component {...block.props} {readOnly} />
				{:else}
					<div class="rounded border border-dashed border-red-300 bg-red-50 p-4 text-center text-sm text-red-600 dark:border-red-700 dark:bg-red-950/30 dark:text-red-400">
						Unknown block type: <code>{block.block_type}</code>
					</div>
				{/if}
			</SortableBlock>
		{/each}

		{#if isEmpty}
			<div class="flex flex-col items-center justify-center rounded-lg border-2 border-dashed border-gray-300 px-6 py-12 text-center dark:border-gray-700">
				<p class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">
					No blocks yet
				</p>
				<p class="text-xs text-gray-400 dark:text-gray-500">
					Add a block to start building your page
				</p>
			</div>
		{/if}
	</div>
</DragDropProvider>
