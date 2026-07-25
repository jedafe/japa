<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Editor } from '@tiptap/core';
	import { buildExtensions } from '$lib/tiptap/extensions';
	import { SLASH_BLOCKS } from '$lib/tiptap/slashMenu';
	import { t } from '$lib/i18n';

	interface Props {
		html: string;
		onedit: () => void;
	}

	let { html, onedit }: Props = $props();

	let element: HTMLDivElement;
	let editor: Editor | null = null;
	let slashOpen = $state(false);
	let slashIndex = $state(0);
	let slashQuery = $state('');

	onMount(() => {
		editor = new Editor({
			element: element,
			extensions: buildExtensions(),
			content: html,
			onUpdate: () => onedit(),
			onSelectionUpdate: ({ editor }) => {
				// Slash menu: detect `/query` at the cursor position
				const { from } = editor.state.selection;
				const text = editor.state.doc.textBetween(1, from, '\n');
				const match = /\/(\w*)$/.exec(text);
				if (match) {
					slashOpen = true;
					slashQuery = match[1];
					slashIndex = 0;
				} else {
					slashOpen = false;
				}
			}
		});
	});

	onDestroy(() => editor?.destroy());

	function runBlock(block: (typeof SLASH_BLOCKS)[0]) {
		if (!editor) return;
		// Delete the `/query` text before inserting the block
		const { from } = editor.state.selection;
		const text = editor.state.doc.textBetween(1, from, '\n');
		const match = /\/(\w*)$/.exec(text);
		if (match) {
			editor.chain().focus().deleteRange({ from: from - match[0].length, to: from }).run();
		}
		block.run(editor);
		slashOpen = false;
	}

	function slashKeyDown(e: KeyboardEvent) {
		if (!slashOpen) return false;
		const blocks = filteredBlocks;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			slashIndex = (slashIndex + 1) % blocks.length;
			return true;
		}
		if (e.key === 'ArrowUp') {
			e.preventDefault();
			slashIndex = (slashIndex - 1 + blocks.length) % blocks.length;
			return true;
		}
		if (e.key === 'Enter') {
			e.preventDefault();
			if (blocks[slashIndex]) runBlock(blocks[slashIndex]);
			return true;
		}
		if (e.key === 'Escape') {
			slashOpen = false;
			return true;
		}
		return false;
	}

	const filteredBlocks = $derived(
		slashQuery
			? SLASH_BLOCKS.filter((b) => b.key.toLowerCase().includes(slashQuery.toLowerCase()))
			: SLASH_BLOCKS
	);

	export function getHTML(): string {
		return editor?.getHTML() ?? '';
	}
</script>

<div class="post-editor">
	<div bind:this={element} class="prose-content" onkeydown={slashKeyDown} role="textbox" tabindex="-1"></div>

	{#if slashOpen && filteredBlocks.length > 0}
		<div class="slash-menu">
			{#each filteredBlocks as block, i (block.key)}
				<button
					type="button"
					class="slash-item"
					class:active={i === slashIndex}
					onclick={() => runBlock(block)}
					onmouseenter={() => (slashIndex = i)}
				>
					{t(block.labelKey)}
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.post-editor {
		position: relative;
	}

	.prose-content {
		min-height: 300px;
		padding: 0.75rem;
		outline: none;
		font-size: 1rem;
		line-height: 1.6;
	}

	.prose-content :global(h1) { font-size: 1.5rem; font-weight: 700; margin: 1rem 0 0.5rem; }
	.prose-content :global(h2) { font-size: 1.25rem; font-weight: 600; margin: 0.75rem 0 0.5rem; }
	.prose-content :global(ul) { list-style: disc; padding-left: 1.5rem; }
	.prose-content :global(ol) { list-style: decimal; padding-left: 1.5rem; }
	.prose-content :global(blockquote) { border-left: 3px solid var(--color-border); padding-left: 1rem; color: var(--color-muted); }
	.prose-content :global(pre) { background: var(--color-surface-subtle); border-radius: var(--radius); padding: 0.75rem; overflow-x: auto; }
	.prose-content :global(p) { margin: 0.5rem 0; }

	.slash-menu {
		position: absolute;
		z-index: 30;
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 180px;
		padding: 4px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		box-shadow: 0 4px 12px rgb(0 0 0 / 0.1);
	}

	.slash-item {
		padding: 6px 10px;
		border: none;
		border-radius: calc(var(--radius) - 2px);
		background: transparent;
		text-align: left;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.slash-item.active {
		background: var(--color-surface-subtle);
	}
</style>
