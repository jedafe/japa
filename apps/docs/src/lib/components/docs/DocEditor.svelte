<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Editor } from '@tiptap/core';
	import { docExtensions } from '$lib/tiptap/extensions';
	import { t } from '$lib/i18n';
	import {
		Bold,
		Italic,
		Strikethrough,
		Code,
		Heading2,
		Heading3,
		List,
		ListOrdered,
		TextQuote,
		SquareCode,
		Minus,
		Link as LinkIcon,
		Unlink,
		Table as TableIcon,
		Undo2,
		Redo2,
		Trash2
	} from 'lucide-svelte';

	interface Props {
		/**
		 * Initial content: a TipTap doc JSON object or an HTML string (for
		 * pages whose stored content is markdown/plain — seeded from the
		 * server-rendered HTML so existing formatting survives).
		 */
		initialContent?: unknown;
		/** Bindable output — the current TipTap JSON doc, updated on edit. */
		value?: unknown;
		placeholder?: string;
	}

	let {
		initialContent = null,
		value = $bindable<unknown>(null),
		placeholder = ''
	}: Props = $props();

	let element: HTMLDivElement;
	// $state.raw — the Editor instance must not be deep-proxied.
	let editor = $state.raw<Editor | null>(null);
	// Bumped on every transaction so toolbar isActive states re-render.
	let tick = $state(0);

	// Link popover state.
	let linkOpen = $state(false);
	let linkUrl = $state('');

	function seedContent(): unknown {
		if (!initialContent) return '';
		// TipTap doc object → pass through; anything else (HTML string from the
		// server-rendered pipeline) → parsed by the editor schema.
		return initialContent;
	}

	// Reactive wrappers around editor state — `tick` is bumped on every
	// transaction so these recompute as the selection/document changes.
	const isActive = $derived.by(() => {
		tick;
		return (name: string, attrs?: Record<string, unknown>) =>
			editor?.isActive(name, attrs ?? {}) ?? false;
	});
	const canUndo = $derived.by(() => {
		tick;
		return editor?.can().undo() ?? false;
	});
	const canRedo = $derived.by(() => {
		tick;
		return editor?.can().redo() ?? false;
	});
	const inTable = $derived.by(() => {
		tick;
		return editor?.isActive('table') ?? false;
	});

	onMount(() => {
		editor = new Editor({
			element,
			extensions: docExtensions,
			content: seedContent() as never,
			editorProps: {
				attributes: {
					class: 'doc-editor-surface',
					spellcheck: 'true',
					'aria-label': placeholder || t('editor.content_label', 'Page content')
				}
			},
			onUpdate: () => {
				if (editor) value = editor.getJSON();
			},
			onTransaction: () => {
				tick += 1;
			}
		});
		// Emit the seed so `value` is never null on first save.
		if (editor) value = editor.getJSON();
	});

	onDestroy(() => {
		editor?.destroy();
		editor = null;
	});

	function run(fn: (e: Editor) => unknown) {
		if (editor) fn(editor);
		tick += 1;
	}

	function openLinkPopover() {
		linkUrl = (editor?.getAttributes('link').href as string | undefined) ?? '';
		linkOpen = true;
	}

	function applyLink() {
		const url = linkUrl.trim();
		if (!editor) return;
		if (url === '') {
			editor.chain().focus().unsetLink().run();
		} else {
			editor
				.chain()
				.focus()
				.extendMarkRange('link')
				.setLink({ href: url, rel: 'noopener noreferrer' })
				.run();
		}
		linkOpen = false;
		tick += 1;
	}
</script>

<div class="doc-editor">
	{#if editor}
		<div class="toolbar" role="toolbar" aria-label={t('editor.toolbar_label', 'Formatting')}>
			<button type="button" class="tb" title={t('editor.undo', 'Undo')} onclick={() => run((e) => e.chain().focus().undo().run())} disabled={!canUndo}><Undo2 size={15} /></button>
			<button type="button" class="tb" title={t('editor.redo', 'Redo')} onclick={() => run((e) => e.chain().focus().redo().run())} disabled={!canRedo}><Redo2 size={15} /></button>
			<span class="tb-sep"></span>
			<button type="button" class="tb" class:on={isActive('bold')} title={t('editor.bold', 'Bold')} onclick={() => run((e) => e.chain().focus().toggleBold().run())}><Bold size={15} /></button>
			<button type="button" class="tb" class:on={isActive('italic')} title={t('editor.italic', 'Italic')} onclick={() => run((e) => e.chain().focus().toggleItalic().run())}><Italic size={15} /></button>
			<button type="button" class="tb" class:on={isActive('strike')} title={t('editor.strike', 'Strikethrough')} onclick={() => run((e) => e.chain().focus().toggleStrike().run())}><Strikethrough size={15} /></button>
			<button type="button" class="tb" class:on={isActive('code')} title={t('editor.inline_code', 'Inline code')} onclick={() => run((e) => e.chain().focus().toggleCode().run())}><Code size={15} /></button>
			<span class="tb-sep"></span>
			<button type="button" class="tb" class:on={isActive('heading', { level: 2 })} title={t('editor.h2', 'Heading 2')} onclick={() => run((e) => e.chain().focus().toggleHeading({ level: 2 }).run())}><Heading2 size={15} /></button>
			<button type="button" class="tb" class:on={isActive('heading', { level: 3 })} title={t('editor.h3', 'Heading 3')} onclick={() => run((e) => e.chain().focus().toggleHeading({ level: 3 }).run())}><Heading3 size={15} /></button>
			<span class="tb-sep"></span>
			<button type="button" class="tb" class:on={isActive('bulletList')} title={t('editor.bullet_list', 'Bullet list')} onclick={() => run((e) => e.chain().focus().toggleBulletList().run())}><List size={15} /></button>
			<button type="button" class="tb" class:on={isActive('orderedList')} title={t('editor.ordered_list', 'Numbered list')} onclick={() => run((e) => e.chain().focus().toggleOrderedList().run())}><ListOrdered size={15} /></button>
			<button type="button" class="tb" class:on={isActive('blockquote')} title={t('editor.quote', 'Quote')} onclick={() => run((e) => e.chain().focus().toggleBlockquote().run())}><TextQuote size={15} /></button>
			<button type="button" class="tb" class:on={isActive('codeBlock')} title={t('editor.code_block', 'Code block')} onclick={() => run((e) => e.chain().focus().toggleCodeBlock().run())}><SquareCode size={15} /></button>
			<button type="button" class="tb" title={t('editor.hr', 'Horizontal rule')} onclick={() => run((e) => e.chain().focus().setHorizontalRule().run())}><Minus size={15} /></button>
			<span class="tb-sep"></span>
			<button type="button" class="tb" class:on={isActive('link')} title={t('editor.link', 'Link')} onclick={openLinkPopover}><LinkIcon size={15} /></button>
			<button type="button" class="tb" title={t('editor.unlink', 'Remove link')} onclick={() => run((e) => e.chain().focus().unsetLink().run())} disabled={!isActive('link')}><Unlink size={15} /></button>
			<button type="button" class="tb" title={t('editor.table', 'Insert table')} onclick={() => run((e) => e.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run())}><TableIcon size={15} /></button>
			{#if inTable}
				<button type="button" class="tb tb-danger" title={t('editor.delete_table', 'Delete table')} onclick={() => run((e) => e.chain().focus().deleteTable().run())}><Trash2 size={15} /></button>
			{/if}
		</div>
		{#if linkOpen}
			<div class="link-pop">
				<input
					type="url"
					class="link-input"
					bind:value={linkUrl}
					placeholder={t('editor.link_placeholder', 'https://…')}
					onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); applyLink(); } }}
				/>
				<button type="button" class="link-apply" onclick={applyLink}>{t('editor.link_apply', 'Apply')}</button>
				<button type="button" class="link-cancel" onclick={() => (linkOpen = false)}>{t('editor.cancel', 'Cancel')}</button>
			</div>
		{/if}
	{/if}
	<div class="prose doc-editor-body" bind:this={element} data-placeholder={placeholder}></div>
</div>

<style>
	.doc-editor {
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		overflow: hidden;
	}
	.toolbar {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.125rem;
		padding: 0.375rem 0.5rem;
		border-bottom: 1px solid var(--color-border);
		background: var(--color-surface-subtle);
		position: sticky;
		top: 0;
		z-index: 5;
	}
	.tb {
		display: grid;
		place-items: center;
		height: 1.75rem;
		width: 1.75rem;
		border: none;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
	}
	.tb:hover { background: var(--color-surface); color: var(--color-text); }
	.tb.on {
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-primary);
	}
	.tb:disabled { opacity: 0.4; cursor: default; }
	.tb-danger:hover { color: var(--color-danger, #dc2626); }
	.tb-sep {
		width: 1px;
		height: 1.125rem;
		background: var(--color-border);
		margin: 0 0.25rem;
	}

	.link-pop {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		border-bottom: 1px solid var(--color-border);
		background: var(--color-surface);
	}
	.link-input {
		flex: 1;
		min-width: 0;
		padding: 0.375rem 0.625rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		color: var(--color-text);
		font-size: 0.8125rem;
	}
	.link-input:focus { outline: none; border-color: var(--color-primary); }
	.link-apply {
		padding: 0.375rem 0.75rem;
		border: none;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.8125rem;
		font-weight: 600;
		cursor: pointer;
	}
	.link-cancel {
		padding: 0.375rem 0.625rem;
		border: none;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-muted);
		font-size: 0.8125rem;
		cursor: pointer;
	}
	.link-cancel:hover { color: var(--color-text); }

	.doc-editor-body {
		padding: 1rem 1.25rem 2rem;
		max-width: none;
		min-height: 16rem;
	}
	/* ProseMirror mounts inside this wrapper. */
	.doc-editor-body :global(.doc-editor-surface) {
		outline: none;
		min-height: 14rem;
	}
	.doc-editor-body :global(.doc-editor-surface p.is-editor-empty:first-child::before) {
		content: attr(data-placeholder);
		color: var(--color-muted);
		float: left;
		height: 0;
		pointer-events: none;
	}
	/* Table editing affordances */
	.doc-editor-body :global(.doc-editor-surface table) {
		border-collapse: collapse;
		width: 100%;
	}
	.doc-editor-body :global(.doc-editor-surface td),
	.doc-editor-body :global(.doc-editor-surface th) {
		border: 1px solid var(--color-border);
		padding: 0.375rem 0.625rem;
		vertical-align: top;
	}
	.doc-editor-body :global(.doc-editor-surface .selectedCell) {
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}
</style>
