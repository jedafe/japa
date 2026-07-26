<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Editor } from '@tiptap/core';
	import { buildExtensions } from '$lib/tiptap/extensions';
	import { SLASH_BLOCKS } from '$lib/tiptap/slashMenu';
	import { t } from '$lib/i18n';

	interface Props {
		html: string;
		onedit: () => void;
		onhtml?: (html: string) => void;
		onfocus?: () => void;
	}

	let { html, onedit, onhtml, onfocus }: Props = $props();

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
			onUpdate: () => {
				// Sync the editor's HTML back to the parent so the autosave
				// snapshot includes the current body — without this the body
				// is never persisted. (Fix: body HTML sync.)
				onhtml?.(editor?.getHTML() ?? '');
				onedit();
			},
			onFocus: () => onfocus?.(),
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

	/* The outer wrapper provides the visible editor surface — a rounded
	 * card with a subtle border and surface background. The inner
	 * .ProseMirror (styled globally in app.css) handles the editable
	 * padding and min-height. */
	.prose-content {
		min-height: 300px;
		padding: 10px 14px;
		outline: none;
		font-size: 1rem;
		line-height: 1.7;
		color: var(--color-text);
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		transition: border-color 0.15s, box-shadow 0.15s;
	}

	/* Typography — these target the Tiptap-generated children inside the
	 * scoped .prose-content wrapper. The :global() is needed because
	 * Tiptap creates the .ProseMirror div at runtime without Svelte's
	 * scoped style hash. */
	.prose-content :global(.ProseMirror) {
		min-height: 260px;
	}

	.prose-content :global(h1) {
		font-size: 1.75rem;
		font-weight: 700;
		margin: 1.5rem 0 0.75rem;
		line-height: 1.25;
	}
	.prose-content :global(h2) {
		font-size: 1.375rem;
		font-weight: 600;
		margin: 1.25rem 0 0.5rem;
		line-height: 1.3;
	}
	.prose-content :global(h3) {
		font-size: 1.125rem;
		font-weight: 600;
		margin: 1rem 0 0.5rem;
		line-height: 1.35;
	}
	.prose-content :global(p) {
		margin: 0.5rem 0;
	}
	.prose-content :global(ul) {
		list-style: disc;
		padding-left: 1.5rem;
		margin: 0.5rem 0;
	}
	.prose-content :global(ol) {
		list-style: decimal;
		padding-left: 1.5rem;
		margin: 0.5rem 0;
	}
	.prose-content :global(li) {
		margin: 0.25rem 0;
	}
	.prose-content :global(blockquote) {
		border-left: 3px solid var(--color-primary);
		padding-left: 1rem;
		margin: 0.75rem 0;
		color: var(--color-text-subtle);
		font-style: italic;
	}
	.prose-content :global(pre) {
		background: var(--color-surface-subtle);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		padding: 0.75rem 1rem;
		overflow-x: auto;
		margin: 0.75rem 0;
		font-size: 0.875rem;
		line-height: 1.5;
	}
	.prose-content :global(pre code) {
		background: none;
		padding: 0;
		font-size: inherit;
	}
	.prose-content :global(code) {
		font-family: ui-monospace, 'SF Mono', Menlo, monospace;
		font-size: 0.875em;
		background: var(--color-surface-subtle);
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
	}
	.prose-content :global(a) {
		color: var(--color-primary);
		text-decoration: underline;
		text-underline-offset: 2px;
	}
	.prose-content :global(a:hover) {
		color: var(--color-primary-hover);
	}
	.prose-content :global(img) {
		max-width: 100%;
		height: auto;
		border-radius: var(--radius);
		margin: 0.75rem 0;
	}
	.prose-content :global(hr) {
		border: none;
		border-top: 1px solid var(--color-border);
		margin: 1.25rem 0;
	}

	/* Slash command menu — rendered inside the component DOM so scoped
	 * styles work fine here. */
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
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.12);
	}

	.slash-item {
		padding: 6px 10px;
		border: none;
		border-radius: calc(var(--radius) - 2px);
		background: transparent;
		color: var(--color-text);
		text-align: left;
		font-size: 0.875rem;
		cursor: pointer;
		transition: background 0.1s;
	}

	.slash-item:hover,
	.slash-item.active {
		background: var(--color-surface-subtle);
		color: var(--color-primary);
	}
</style>
