<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { Editor } from '@tiptap/core';
	import { buildExtensions } from '$lib/tiptap/extensions';
	import { SLASH_BLOCKS } from '$lib/tiptap/slashMenu';
	import { t } from '$lib/i18n';
	import {
		Heading1,
		Heading2,
		Heading3,
		List,
		ListOrdered,
		ListChecks,
		Quote,
		Code2,
		ImagePlus,
		Minus,
		Table,
		Type,
		Bold,
		Italic,
		Strikethrough,
		Code,
		Underline,
		Link as LinkIcon,
		Undo2,
		Redo2,
		LayoutTemplate,
		FileCode
	} from 'lucide-svelte';

	const ICONS: Record<string, typeof Heading1> = {
		Heading1,
		Heading2,
		Heading3,
		List,
		ListOrdered,
		ListChecks,
		Quote,
		Code2,
		ImagePlus,
		Minus,
		Table,
		Type
	};

	export type EditorMode = 'block' | 'wysiwyg' | 'markdown';

	interface Props {
		html: string;
		onedit: () => void;
		onhtml?: (html: string) => void;
		onfocus?: () => void;
		defaultMode?: EditorMode;
	}

	let {
		html,
		onedit,
		onhtml,
		onfocus,
		defaultMode = 'block'
	}: Props = $props();

	let element = $state<HTMLDivElement>(null!);
	let editor = $state<Editor | null>(null);
	let slashOpen = $state(false);
	let slashIndex = $state(0);
	let slashQuery = $state('');
	// Cursor coordinates (relative to .post-editor) for positioning the slash menu
	let slashTop = $state(0);
	let slashLeft = $state(0);

	// Editor mode — initialised from the admin-configured default (passed via
	// appearance.default_editor system setting). Ref: conventions/system-settings-catalog.md
	let mode = $state<EditorMode>(untrack(() => defaultMode));
	// Raw markdown source for markdown mode. Kept in sync with the Tiptap
	// document when switching modes so round-trips are lossless.
	let markdownSource = $state('');
	// Whether the markdown textarea has unsynced local edits (used to avoid
	// clobbering the textarea while the user is typing in markdown mode).
	let markdownDirty = $state(false);

	const MODES: { value: EditorMode; icon: typeof Heading1; labelKey: string; descKey: string }[] = [
		{ value: 'block', icon: LayoutTemplate, labelKey: 'create.editor.mode.block', descKey: 'create.editor.mode.block_desc' },
		{ value: 'wysiwyg', icon: Type, labelKey: 'create.editor.mode.wysiwyg', descKey: 'create.editor.mode.wysiwyg_desc' },
		{ value: 'markdown', icon: FileCode, labelKey: 'create.editor.mode.markdown', descKey: 'create.editor.mode.markdown_desc' }
	];

	onMount(() => {
		// Determine whether the initial content is HTML or plain text/markdown.
		// When body_html is empty in the database (some content was saved with
		// only the markdown body), CreateShell falls back to draft.body — which
		// is markdown, not HTML. The Tiptap Markdown extension can parse it via
		// setContent(..., { contentType: 'markdown' }).
		const isHTML = html.trim().startsWith('<');
		// Initialize with empty content and set it in onCreate — the editor's
		// view isn't always ready to render when setContent is called
		// immediately after construction in Tiptap 3.x. Using onCreate ensures
		// the ProseMirror view is fully mounted before we inject content.
		editor = new Editor({
			element: element,
			extensions: buildExtensions({
				placeholder: t('create.editor.placeholder')
			}),
			content: '',
			onCreate: () => {
				if (!html) return;
				// Defer to the next animation frame so the view has finished
				// its initial layout pass before we replace the document.
				// Without this, ProseMirror can leave the DOM empty even
				// though the editor state contains the content.
				requestAnimationFrame(() => {
					if (!editor || editor.isDestroyed) return;
					if (isHTML) {
						editor.commands.setContent(html, { emitUpdate: false });
					} else {
						editor.commands.setContent(html, { contentType: 'markdown', emitUpdate: false });
					}
					// When the editor starts in markdown mode, the Tiptap
					// surface is hidden (display:none) and the markdown
					// textarea is shown instead. The textarea reads from
					// markdownSource, which is only populated on mode switch.
					// Without this, the textarea is empty even though the
					// ProseMirror document has the content. Serialize the
					// editor's document to markdown for the textarea.
					if (mode === 'markdown') {
						markdownSource = editor.getMarkdown();
						markdownDirty = false;
					}
				});
			},
			onUpdate: () => {
				// Sync the editor's HTML back to the parent so the autosave
				// snapshot includes the current body — without this the body
				// is never persisted. (Fix: body HTML sync.)
				onhtml?.(editor?.getHTML() ?? '');
				onedit();
				// If the user is editing in a Tiptap mode, the markdown mirror
				// is stale until they switch back to markdown mode.
				if (mode !== 'markdown') markdownDirty = false;
			},
			onFocus: () => onfocus?.(),
			onSelectionUpdate: ({ editor }) => {
				// Slash menu only applies in block mode.
				if (mode !== 'block') {
					slashOpen = false;
					return;
				}
				// Slash menu: detect `/query` at the cursor position
				const { from } = editor.state.selection;
				const text = editor.state.doc.textBetween(1, from, '\n');
				const match = /\/(\w*)$/.exec(text);
				if (match) {
					slashOpen = true;
					slashQuery = match[1];
					slashIndex = 0;
					// Position the menu at the cursor, relative to .post-editor
					const coords = editor.view.coordsAtPos(from);
					const containerRect = element.getBoundingClientRect();
					slashLeft = coords.left - containerRect.left;
					slashTop = coords.bottom - containerRect.top + 6;
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

	// ---- Mode switching ----
	// When leaving markdown mode, parse the textarea content back into the
	// Tiptap document so edits are preserved. When entering markdown mode,
	// serialize the current Tiptap document to markdown.
	function switchMode(next: EditorMode) {
		if (next === mode || !editor) return;

		// Leaving markdown → push textarea edits into the Tiptap doc immediately.
		if (mode === 'markdown' && markdownDirty) {
			syncMarkdownToEditor();
		}

		// Entering markdown → serialize the current doc to markdown.
		if (next === 'markdown') {
			markdownSource = editor.getMarkdown();
			markdownDirty = false;
		}

		mode = next;

		// Refocus the appropriate surface.
		if (next === 'markdown') {
			queueMicrotask(() => markdownTextarea?.focus());
		} else {
			queueMicrotask(() => editor?.commands.focus());
		}
	}

	let markdownTextarea = $state<HTMLTextAreaElement | null>(null);

	// Debounce timer for the markdown → editor sync so we don't do a full
	// ProseMirror document replacement on every keystroke.
	let markdownSyncTimer: ReturnType<typeof setTimeout> | null = null;
	const MARKDOWN_SYNC_DELAY = 400;

	// syncMarkdownToEditor parses the current markdownSource into the Tiptap
	// document and propagates the resulting HTML to the parent. Called on
	// mode switch (immediate) and on a debounce after typing in markdown mode.
	function syncMarkdownToEditor() {
		if (!editor) return;
		editor.commands.setContent(markdownSource, { contentType: 'markdown', emitUpdate: false });
		onhtml?.(editor.getHTML());
		onedit();
		markdownDirty = false;
	}

	function onMarkdownInput(e: Event) {
		markdownSource = (e.currentTarget as HTMLTextAreaElement).value;
		markdownDirty = true;
		onedit();
		// Debounce the markdown→HTML conversion so the parent's autosave
		// snapshot stays current without parsing markdown on every keystroke.
		if (markdownSyncTimer) clearTimeout(markdownSyncTimer);
		markdownSyncTimer = setTimeout(syncMarkdownToEditor, MARKDOWN_SYNC_DELAY);
	}

	onDestroy(() => {
		if (markdownSyncTimer) clearTimeout(markdownSyncTimer);
	});

	// ---- WYSIWYG toolbar helpers ----
	function tb(cmd: string, ...args: unknown[]) {
		if (!editor) return;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const chain = (editor.chain().focus() as any);
		chain[cmd](...args).run();
	}

	function isActive(name: string, attrs?: Record<string, unknown>): boolean {
		return editor?.isActive(name, attrs) ?? false;
	}

	function setLinkFromPrompt() {
		if (!editor) return;
		const prev = editor.getAttributes('link').href ?? '';
		const url = window.prompt(t('create.editor.prompt.url'), prev);
		if (url === null) return;
		if (url === '') {
			editor.chain().focus().extendMarkRange('link').unsetLink().run();
		} else {
			editor.chain().focus().extendMarkRange('link').setLink({ href: url }).run();
		}
	}

	function setImageFromPrompt() {
		if (!editor) return;
		const url = window.prompt(t('create.editor.prompt.image_url'));
		if (url) editor.chain().focus().setImage({ src: url }).run();
	}

	export function getHTML(): string {
		return editor?.getHTML() ?? '';
	}
</script>

<div class="post-editor">
	{#if mode === 'markdown'}
		<textarea
			bind:this={markdownTextarea}
			class="markdown-source"
			value={markdownSource}
			oninput={onMarkdownInput}
			onfocus={() => onfocus?.()}
			placeholder={t('create.editor.placeholder_markdown')}
			spellcheck="true"
		></textarea>
	{/if}

	<!-- The editor card wraps the Tiptap mount and (in WYSIWYG mode) the
	     formatting toolbar docked at the bottom inside the same border.
	     The .editor-surface is always mounted so the Editor instance keeps
	     its DOM node across mode switches; hidden via CSS in markdown mode. -->
	<div class="editor-surface" class:hidden={mode === 'markdown'}>
		<div
			bind:this={element}
			class="prose-content"
			onkeydown={slashKeyDown}
			role="textbox"
			tabindex="-1"
		></div>

		{#if mode === 'wysiwyg' && editor}
			<div class="toolbar" role="toolbar">
				<button type="button" class="tb-btn" class:active={isActive('bold')} title={t('create.editor.toolbar.bold')} onclick={() => tb('toggleBold')}><Bold size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('italic')} title={t('create.editor.toolbar.italic')} onclick={() => tb('toggleItalic')}><Italic size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('underline')} title={t('create.editor.toolbar.underline')} onclick={() => tb('toggleUnderline')}><Underline size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('strike')} title={t('create.editor.toolbar.strike')} onclick={() => tb('toggleStrike')}><Strikethrough size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('code')} title={t('create.editor.toolbar.code')} onclick={() => tb('toggleCode')}><Code size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('heading', { level: 1 })} title={t('create.editor.toolbar.h1')} onclick={() => tb('toggleHeading', { level: 1 })}><Heading1 size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('heading', { level: 2 })} title={t('create.editor.toolbar.h2')} onclick={() => tb('toggleHeading', { level: 2 })}><Heading2 size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('heading', { level: 3 })} title={t('create.editor.toolbar.h3')} onclick={() => tb('toggleHeading', { level: 3 })}><Heading3 size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('bulletList')} title={t('create.editor.toolbar.bullet_list')} onclick={() => tb('toggleBulletList')}><List size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('orderedList')} title={t('create.editor.toolbar.ordered_list')} onclick={() => tb('toggleOrderedList')}><ListOrdered size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('taskList')} title={t('create.editor.toolbar.task_list')} onclick={() => tb('toggleTaskList')}><ListChecks size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('blockquote')} title={t('create.editor.toolbar.blockquote')} onclick={() => tb('toggleBlockquote')}><Quote size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('codeBlock')} title={t('create.editor.toolbar.code_block')} onclick={() => tb('toggleCodeBlock')}><Code2 size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('link')} title={t('create.editor.toolbar.link')} onclick={setLinkFromPrompt}><LinkIcon size={16} /></button>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.image')} onclick={setImageFromPrompt}><ImagePlus size={16} /></button>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.divider')} onclick={() => tb('setHorizontalRule')}><Minus size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.undo')} onclick={() => tb('undo')} disabled={!editor.can().undo()}><Undo2 size={16} /></button>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.redo')} onclick={() => tb('redo')} disabled={!editor.can().redo()}><Redo2 size={16} /></button>
			</div>
		{/if}
	</div>

	<!-- Mode switcher — floated at top-right of the editing surface -->
	<div class="mode-switcher" role="tablist" aria-label={t('create.title')}>
		{#each MODES as m (m.value)}
			<button
				type="button"
				role="tab"
				aria-selected={mode === m.value}
				class="mode-tab"
				class:active={mode === m.value}
				title={t(m.descKey)}
				aria-label={t(m.labelKey)}
				onclick={() => switchMode(m.value)}
			>
				<m.icon size={16} />
			</button>
		{/each}
	</div>

	{#if mode === 'block' && slashOpen && filteredBlocks.length > 0}
		<div class="slash-menu" style="top: {slashTop}px; left: {slashLeft}px">
			{#each filteredBlocks as block, i (block.key)}
				{@const Icon = ICONS[block.icon]}
				<button
					type="button"
					class="slash-item"
					class:active={i === slashIndex}
					onclick={() => runBlock(block)}
					onmouseenter={() => (slashIndex = i)}
				>
					<span class="slash-icon">
						{#if Icon}<Icon size={18} />{/if}
					</span>
					<span class="slash-text">
						<span class="slash-label">{t(block.labelKey)}</span>
						<span class="slash-desc">{t(block.descKey)}</span>
					</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.post-editor {
		position: relative;
	}

	/* ---- Mode switcher — floated at top-right of the editing surface ---- */
	.mode-switcher {
		position: absolute;
		top: 8px;
		right: 8px;
		z-index: 20;
		display: inline-flex;
		gap: 2px;
		padding: 2px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
	}
	.mode-tab {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		padding: 0;
		border: none;
		border-radius: calc(var(--radius) - 2px);
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: background 0.12s, color 0.12s;
	}
	.mode-tab:hover {
		color: var(--color-text);
	}
	.mode-tab.active {
		background: var(--color-surface);
		color: var(--color-primary);
		box-shadow: 0 1px 2px rgb(0 0 0 / 0.08);
	}

	/* ---- WYSIWYG toolbar — docked at the bottom inside the editor card ---- */
	.toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 2px;
		padding: 6px 8px;
		border-top: 1px solid var(--color-border);
		border-radius: 0 0 var(--radius) var(--radius);
		background: var(--color-surface-subtle);
	}
	.tb-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--color-text);
		cursor: pointer;
		transition: background 0.1s, color 0.1s;
	}
	.tb-btn:hover:not(:disabled) {
		background: var(--color-surface-subtle);
	}
	.tb-btn.active {
		background: var(--color-primary);
		color: var(--color-surface);
	}
	.tb-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.tb-sep {
		width: 1px;
		height: 18px;
		margin: 0 4px;
		background: var(--color-border);
	}

	/* ---- Markdown source textarea ---- */
	.markdown-source {
		width: 100%;
		min-height: 300px;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-family: ui-monospace, 'SF Mono', Menlo, monospace;
		font-size: 0.9375rem;
		line-height: 1.7;
		resize: vertical;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.markdown-source:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 20%, transparent);
	}

	/* The editor card — a rounded surface wrapping the Tiptap mount and
	 * (in WYSIWYG mode) the docked formatting toolbar at the bottom. */
	.editor-surface {
		display: flex;
		flex-direction: column;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		overflow: hidden;
		transition: border-color 0.15s, box-shadow 0.15s;
	}

	/* The editable region fills the card; the toolbar docks at the bottom
	 * inside the same border. */
	.prose-content {
		flex: 1 1 auto;
		min-height: 300px;
		padding: 10px 14px;
		outline: none;
		font-size: 1rem;
		line-height: 1.7;
		color: var(--color-text);
	}

	/* Hide the editor card when markdown mode is active — the div stays
	 * mounted so the Editor instance retains its DOM node. */
	.editor-surface.hidden {
		display: none;
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

	/* Task list */
	.prose-content :global(ul[data-type='taskList']) {
		list-style: none;
		padding: 0;
		margin: 0.5rem 0;
	}
	.prose-content :global(ul[data-type='taskList'] li) {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
	}
	.prose-content :global(ul[data-type='taskList'] li > label) {
		flex-shrink: 0;
		margin-top: 0.25rem;
	}
	.prose-content :global(ul[data-type='taskList'] li > div) {
		flex: 1;
	}
	.prose-content :global(ul[data-type='taskList'] input[type='checkbox']) {
		width: 16px;
		height: 16px;
		accent-color: var(--color-primary);
		cursor: pointer;
	}

	/* Table */
	.prose-content :global(table) {
		border-collapse: collapse;
		width: 100%;
		margin: 0.75rem 0;
		overflow: hidden;
	}
	.prose-content :global(th),
	.prose-content :global(td) {
		border: 1px solid var(--color-border);
		padding: 8px 12px;
		text-align: left;
		font-size: 0.9rem;
	}
	.prose-content :global(th) {
		background: var(--color-surface-subtle);
		font-weight: 600;
	}
	.prose-content :global(.selectedCell) {
		background: var(--color-primary);
		opacity: 0.1;
	}
	.prose-content :global(.column-resize-handle) {
		position: absolute;
		right: -2px;
		top: 0;
		bottom: 0;
		width: 4px;
		background: var(--color-primary);
		pointer-events: none;
	}
	.prose-content :global(.resize-cursor) {
		cursor: col-resize;
	}

	/* Slash command menu — rendered inside the component DOM so scoped
	 * styles work fine here. */
	.slash-menu {
		position: absolute;
		z-index: 30;
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 240px;
		max-height: 320px;
		overflow-y: auto;
		padding: 6px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		box-shadow: 0 10px 30px rgb(0 0 0 / 0.15);
	}

	.slash-item {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 10px;
		border: none;
		border-radius: calc(var(--radius) - 2px);
		background: transparent;
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
		transition: background 0.1s;
	}

	.slash-item:hover,
	.slash-item.active {
		background: var(--color-surface-subtle);
	}

	.slash-item.active .slash-label {
		color: var(--color-primary);
	}

	.slash-icon {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		width: 32px;
		height: 32px;
		border-radius: 6px;
		background: var(--color-surface-subtle);
		color: var(--color-text);
	}

.slash-item.active .slash-icon {
    background: var(--color-primary);
    color: var(--color-surface);
}

	.slash-text {
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 0;
	}

	.slash-label {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
	}

	.slash-desc {
		font-size: 0.75rem;
		color: var(--color-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
</style>
