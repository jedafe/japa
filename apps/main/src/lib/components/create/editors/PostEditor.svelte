<script lang="ts">
	import { untrack, mount, unmount } from 'svelte';
	import { tick } from 'svelte';
	import { Editor } from '@tiptap/core';
	import { buildExtensions } from '$lib/tiptap/extensions';
	import { SLASH_BLOCKS } from '$lib/tiptap/slashMenu';
	import type { SlashBlockContext } from '$lib/tiptap/slashMenu';
	import { t } from '$lib/i18n';
	import type { MediaAsset } from '$lib/types';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import InsertModal from './InsertModal.svelte';
	import LinkPreviewCard from './LinkPreviewCard.svelte';
	import EmojiPicker from '$lib/components/tiptap/EmojiPicker.svelte';
	import {
		Heading2,
		Heading3,
		List,
		ListOrdered,
		Quote,
		Code2,
		ImagePlus,
		Video,
		Minus,
		Table,
		Ticket,
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

	const ICONS: Record<string, typeof Heading2> = {
		Heading2,
		Heading3,
		List,
		ListOrdered,
		Quote,
		Code2,
		ImagePlus,
		Youtube: Video,
		Minus,
		Table,
		Ticket,
		Type,
		Link: LinkIcon
	};

	export type EditorMode = 'block' | 'wysiwyg' | 'markdown';

	interface Props {
		html: string;
		onedit: () => void;
		onhtml?: (html: string) => void;
		/** Fires with the editor's TipTap JSON document on every update.
		 *  Used by the forum composer which stores body as base64 TipTap JSON. */
		onjson?: (json: Record<string, unknown>) => void;
		onfocus?: () => void;
		defaultMode?: EditorMode;
		aiTrigger?: import('svelte').Snippet;
	}

	let {
		html,
		onedit,
		onhtml,
		onjson,
		onfocus,
		defaultMode = 'block',
		aiTrigger
	}: Props = $props();

	let element = $state<HTMLDivElement>(null!);
	let postEditorEl = $state<HTMLDivElement>(null!);
	let editor = $state<Editor | null>(null);
	let slashOpen = $state(false);
	let slashIndex = $state(0);
	let slashQuery = $state('');
	// Cursor coordinates (relative to .post-editor) for positioning the slash menu
	let slashTop = $state(0);
	let slashLeft = $state(0);

	// Paste handler reference for cleanup
	let pasteHandler: ((e: ClipboardEvent) => void) | null = null;
	// Mounted LinkPreviewCard components inside the editor DOM
	const lpMounts = new Map<HTMLElement, Record<string, unknown>>();

	// Editor mode — initialised from the admin-configured default (passed via
	// appearance.default_editor system setting). Ref: conventions/system-settings-catalog.md
	let mode = $state<EditorMode>(untrack(() => defaultMode));
	// Raw markdown source for markdown mode. Kept in sync with the Tiptap
	// document when switching modes so round-trips are lossless.
	let markdownSource = $state('');
	// Whether the markdown textarea has unsynced local edits (used to avoid
	// clobbering the textarea while the user is typing in markdown mode).
	let markdownDirty = $state(false);

	const MODES: { value: EditorMode; icon: typeof Heading2; labelKey: string; descKey: string }[] = [
		{ value: 'block', icon: LayoutTemplate, labelKey: 'create.editor.mode.block', descKey: 'create.editor.mode.block_desc' },
		{ value: 'wysiwyg', icon: Type, labelKey: 'create.editor.mode.wysiwyg', descKey: 'create.editor.mode.wysiwyg_desc' },
		{ value: 'markdown', icon: FileCode, labelKey: 'create.editor.mode.markdown', descKey: 'create.editor.mode.markdown_desc' }
	];

	// Hydrate link-preview placeholder divs inside the editor DOM into
	// LinkPreviewCard Svelte components. Called after each editor update.
	function hydrateEditorLinkPreviews() {
		if (!element) return;
		const placeholders = element.querySelectorAll<HTMLElement>(
			'div[data-type="link-preview"]:not([data-hydrated])'
		);
		for (const el of placeholders) {
			const url = el.dataset.url;
			if (!url) continue;
			el.setAttribute('data-hydrated', 'true');
			const comp = mount(LinkPreviewCard, { target: el, props: { url } });
			lpMounts.set(el, comp);
		}
	}

	// Initialise the TipTap editor once the mount element is available.
	// Uses $effect (Svelte 5 runes) instead of onMount. All reactive reads
	// (html, mode, t, postEditorEl) are untracked so the effect only re-runs
	// when `element` changes — not on every editor update that changes html.
	$effect(() => {
		if (!element) return;

		// Capture initial values without tracking them as dependencies.
		// The editor is created once; reading html/mode here would cause
		// the effect to re-run on every keystroke (onhtml → html prop
		// changes → effect re-runs → editor destroyed & recreated).
		const initialHtml = untrack(() => html);
		const initialMode = untrack(() => mode);
		const placeholderText = untrack(() => t('create.editor.placeholder'));
		const isHTML = initialHtml.trim().startsWith('<');

		editor = new Editor({
			element: element,
			extensions: buildExtensions({
				placeholder: placeholderText
			}),
			content: '',
			editable: initialMode !== 'markdown',
			onCreate: () => {
				if (!initialHtml) return;
				requestAnimationFrame(() => {
					if (!editor || editor.isDestroyed) return;
					if (isHTML) {
						editor.commands.setContent(initialHtml, { emitUpdate: false });
					} else {
						editor.commands.setContent(initialHtml, { contentType: 'markdown', emitUpdate: false });
					}
					if (untrack(() => mode) === 'markdown') {
						markdownSource = editor.getMarkdown();
						markdownDirty = false;
					}
					hydrateEditorLinkPreviews();
				});
			},
			onUpdate: () => {
				onhtml?.(editor?.getHTML() ?? '');
				onjson?.(editor?.getJSON() ?? { type: 'doc', content: [] });
				onedit();
				if (untrack(() => mode) !== 'markdown') markdownDirty = false;
				requestAnimationFrame(hydrateEditorLinkPreviews);
			},
			onFocus: () => onfocus?.(),
			onSelectionUpdate: ({ editor }) => {
				if (untrack(() => mode) !== 'block') {
					slashOpen = false;
					return;
				}
				const { from } = editor.state.selection;
				const text = editor.state.doc.textBetween(1, from, '\n');
				const match = /\/(\w*)$/.exec(text);
				if (match) {
					slashOpen = true;
					slashQuery = match[1];
					slashIndex = 0;
					requestAnimationFrame(() => {
						if (!editor || editor.isDestroyed) return;
						const pel = untrack(() => postEditorEl);
						if (!pel) return;
						const coords = editor.view.coordsAtPos(from, -1);
						const containerRect = pel.getBoundingClientRect();
						slashLeft = coords.left - containerRect.left;
						slashTop = coords.bottom - containerRect.top + 6;
					});
				} else {
					slashOpen = false;
				}
			}
		});

		// DOM-level paste listener for link preview (ROADMAP-MAIN 07.09).
		// Uses capture phase to fire before ProseMirror's internal handling.
		function handlePasteEvent(e: ClipboardEvent) {
			const text = e.clipboardData?.getData('text/plain') ?? '';
			const trimmed = text.trim();
			if (!/^https?:\/\/\S+$/i.test(trimmed) || !editor || editor.isDestroyed) return;

			const { from, to } = editor.state.selection;
			if (from !== to) {
				// Text is selected — apply the URL as a link mark to the
				// selected text (inline link), don't insert a preview card.
				e.preventDefault();
				e.stopPropagation();
				editor.chain().focus().setLink({ href: trimmed }).run();
			} else {
				// No selection (cursor only) — insert a linkPreview block
				// node. The onUpdate handler hydrates it into a card.
				e.preventDefault();
				e.stopPropagation();
				editor.chain().focus().insertLinkPreview(trimmed).run();
			}
		}
		element.addEventListener('paste', handlePasteEvent, true);
		pasteHandler = handlePasteEvent;

		// Cleanup — Svelte 5 $effect cleanup function.
		return () => {
			if (pasteHandler && element) {
				element.removeEventListener('paste', pasteHandler, true);
			}
			for (const [, comp] of lpMounts) {
				unmount(comp);
			}
			lpMounts.clear();
			editor?.destroy();
			editor = null;
		};
	});

	function runBlock(block: (typeof SLASH_BLOCKS)[0]) {
		if (!editor) return;
		// Delete the `/query` text before inserting the block
		const { from } = editor.state.selection;
		const text = editor.state.doc.textBetween(1, from, '\n');
		const match = /\/(\w*)$/.exec(text);
		if (match) {
			editor.chain().focus().deleteRange({ from: from - match[0].length, to: from }).run();
		}
		const ctx: SlashBlockContext = {
			onInsertImage: openMediaPicker,
			onInsertYoutube: () => openInsertModal('youtube'),
			onInsertLink: () => openInsertModal('link'),
			onInsertCoupon: () => openInsertModal('coupon'),
		};
		block.run(editor, ctx);
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

		// Disable the TipTap editor in markdown mode so its suggestion
		// plugins (mention, hashtag, interlink) don't activate when
		// syncMarkdownToEditor calls setContent. The editor DOM is
		// already hidden via CSS, but an editable editor still processes
		// transactions that can trigger the suggestion popup — which
		// would appear at (0,0) since the editor has no layout box.
		editor.setEditable(next !== 'markdown');

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
		// Hydrate link-preview nodes that were parsed from the markdown
		// source (the placeholder divs become linkPreview nodes via
		// parseHTML, but need hydration into LinkPreviewCard components).
		requestAnimationFrame(hydrateEditorLinkPreviews);
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

	// Clear the markdown sync timer on cleanup (Svelte 5 $effect cleanup).
	$effect(() => {
		return () => {
			if (markdownSyncTimer) clearTimeout(markdownSyncTimer);
		};
	});

	// Keep the editor document in sync with external `html` updates (e.g.
	// AI-generated body applied from CreateShell). `emitUpdate: false` avoids
	// re-firing onhtml/onjson and creating a feedback loop.
	$effect(() => {
		if (!editor || editor.isDestroyed) return;
		if (html !== editor.getHTML()) {
			editor.commands.setContent(html, { emitUpdate: false });
			if (untrack(() => mode) === 'markdown') {
				markdownSource = editor.getMarkdown();
				markdownDirty = false;
			}
			requestAnimationFrame(hydrateEditorLinkPreviews);
		}
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
		openInsertModal('link');
	}

	function setImageFromPrompt() {
		if (!editor) return;
		openMediaPicker();
	}

	function insertEmoji(emoji: string) {
		if (!editor) return;
		editor.chain().focus().insertContent(emoji).run();
	}

	// ---- Media picker (image upload via MediaLibraryModal) ----
	let mediaPickerOpen = $state(false);

	function openMediaPicker() {
		mediaPickerOpen = true;
	}

	function handleMediaSelect(asset: MediaAsset) {
		mediaPickerOpen = false;
		if (editor && !editor.isDestroyed) {
			editor.chain().focus().setImage({ src: asset.url, alt: asset.alt_text ?? asset.filename ?? '' }).run();
		}
	}

	// ---- Insert modal (YouTube / Link / Coupon) ----
	type InsertKind = 'youtube' | 'link' | 'coupon';
	let insertModalKind = $state<InsertKind | null>(null);
	let insertCurrentUrl = $state('');

	function openInsertModal(kind: InsertKind) {
		if (kind === 'link' && editor) {
			insertCurrentUrl = editor.getAttributes('link').href ?? '';
		} else {
			insertCurrentUrl = '';
		}
		insertModalKind = kind;
	}

	function closeInsertModal() {
		insertModalKind = null;
		insertCurrentUrl = '';
	}

	function handleInsertConfirm(value: string) {
		if (!editor || editor.isDestroyed) {
			closeInsertModal();
			return;
		}
		if (insertModalKind === 'youtube') {
			editor.chain().focus().setYoutubeVideo({ src: value }).run();
		} else if (insertModalKind === 'link') {
			applyLinkUrl(value);
		} else if (insertModalKind === 'coupon') {
			const sep = value.indexOf('|');
			const couponId = sep >= 0 ? value.slice(0, sep) : value;
			const couponLabel = sep >= 0 ? value.slice(sep + 1) : '';
			editor.chain().focus().insertContent({
				type: 'couponEmbed',
				attrs: { coupon_id: couponId, label: couponLabel },
				content: [{ type: 'text', text: couponLabel }]
			}).run();
		}
		closeInsertModal();
	}

	function applyLinkUrl(url: string) {
		if (!editor) return;
		const trimmed = url.trim();
		if (trimmed === '') {
			editor.chain().focus().extendMarkRange('link').unsetLink().run();
			return;
		}
		if (/^https?:\/\//i.test(trimmed)) {
			const { from, to } = editor.state.selection;
			if (from === to) {
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				(editor.commands as any).insertLinkPreview?.(trimmed);
			} else {
				editor.chain().focus().extendMarkRange('link').setLink({ href: trimmed }).run();
			}
		} else {
			const { from, to } = editor.state.selection;
			if (from === to) {
				editor.chain().focus().insertContent({
					type: 'text',
					text: trimmed,
					marks: [{ type: 'link', attrs: { href: trimmed } }]
				}).run();
			} else {
				editor.chain().focus().extendMarkRange('link').setLink({ href: trimmed }).run();
			}
		}
	}

	export function getHTML(): string {
		return editor?.getHTML() ?? '';
	}

	// Markdown mode paste handler — inserts a link preview placeholder as
	// raw HTML into the markdown source. When the user switches back to
	// block/wysiwyg mode, TipTap's parseHTML rule picks up the div and
	// creates a linkPreview node (ROADMAP-MAIN 07.09).
	function handleMarkdownPaste(e: ClipboardEvent) {
		const text = e.clipboardData?.getData('text/plain') ?? '';
		const trimmed = text.trim();
		if (!/^https?:\/\/\S+$/i.test(trimmed)) return;
		// BUG-003: HTML-escape the URL before interpolating into the
		// data-url attribute, matching the escaping in linkPreviewNode.ts
		// renderMarkdown. Without this, a URL containing " or < could break
		// out of the attribute and inject arbitrary HTML into the editor.
		const escaped = trimmed
			.replace(/&/g, '&amp;')
			.replace(/"/g, '&quot;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;');
		// Insert the placeholder HTML at the cursor position in the textarea.
		const ta = e.currentTarget as HTMLTextAreaElement;
		const start = ta.selectionStart;
		const end = ta.selectionEnd;
		const placeholder = `\n\n<div data-type="link-preview" data-url="${escaped}"></div>\n\n`;
		e.preventDefault();
		markdownSource = markdownSource.slice(0, start) + placeholder + markdownSource.slice(end);
		markdownDirty = true;
		onedit();
		// Debounce the markdown→HTML conversion so the editor document updates.
		if (markdownSyncTimer) clearTimeout(markdownSyncTimer);
		markdownSyncTimer = setTimeout(syncMarkdownToEditor, MARKDOWN_SYNC_DELAY);
		// Move cursor after the inserted placeholder.
		tick().then(() => {
			const pos = start + placeholder.length;
			ta.focus();
			ta.setSelectionRange(pos, pos);
		});
	}
</script>

<div class="post-editor" bind:this={postEditorEl}>
	{#if mode === 'markdown'}
		<textarea
			bind:this={markdownTextarea}
			class="markdown-source"
			value={markdownSource}
			oninput={onMarkdownInput}
			onfocus={() => onfocus?.()}
			onpaste={handleMarkdownPaste}
			placeholder={t('create.editor.placeholder_markdown')}
			spellcheck="true"
		></textarea>
	{/if}

	<!-- The editor card wraps the Tiptap mount and (in WYSIWYG mode) the
	     formatting toolbar docked at the top inside the same border.
	     The .editor-surface is always mounted so the Editor instance keeps
	     its DOM node across mode switches; hidden via CSS in markdown mode. -->
	<div class="editor-surface" class:hidden={mode === 'markdown'}>
		{#if mode === 'wysiwyg' && editor}
			<div class="toolbar" role="toolbar">
				<button type="button" class="tb-btn" class:active={isActive('bold')} title={t('create.editor.toolbar.bold')} onclick={() => tb('toggleBold')}><Bold size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('italic')} title={t('create.editor.toolbar.italic')} onclick={() => tb('toggleItalic')}><Italic size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('underline')} title={t('create.editor.toolbar.underline')} onclick={() => tb('toggleUnderline')}><Underline size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('strike')} title={t('create.editor.toolbar.strike')} onclick={() => tb('toggleStrike')}><Strikethrough size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('code')} title={t('create.editor.toolbar.code')} onclick={() => tb('toggleCode')}><Code size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('heading', { level: 2 })} title={t('create.editor.toolbar.h2')} onclick={() => tb('toggleHeading', { level: 2 })}><Heading2 size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('heading', { level: 3 })} title={t('create.editor.toolbar.h3')} onclick={() => tb('toggleHeading', { level: 3 })}><Heading3 size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('bulletList')} title={t('create.editor.toolbar.bullet_list')} onclick={() => tb('toggleBulletList')}><List size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('orderedList')} title={t('create.editor.toolbar.ordered_list')} onclick={() => tb('toggleOrderedList')}><ListOrdered size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('blockquote')} title={t('create.editor.toolbar.blockquote')} onclick={() => tb('toggleBlockquote')}><Quote size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('codeBlock')} title={t('create.editor.toolbar.code_block')} onclick={() => tb('toggleCodeBlock')}><Code2 size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('link')} title={t('create.editor.toolbar.link')} onclick={setLinkFromPrompt}><LinkIcon size={16} /></button>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.image')} onclick={setImageFromPrompt}><ImagePlus size={16} /></button>
				<button type="button" class="tb-btn" title={t("create.editor.toolbar.youtube")} onclick={() => openInsertModal("youtube")}><Video size={16} /></button>
				<button type="button" class="tb-btn" title={t("create.editor.toolbar.coupon")} onclick={() => openInsertModal("coupon")}><Ticket size={16} /></button>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.divider')} onclick={() => tb('setHorizontalRule')}><Minus size={16} /></button>
				<span class="tb-sep"></span>
				<EmojiPicker onselect={insertEmoji} />
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.undo')} onclick={() => tb('undo')} disabled={!editor.can().undo()}><Undo2 size={16} /></button>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.redo')} onclick={() => tb('redo')} disabled={!editor.can().redo()}><Redo2 size={16} /></button>
				{#if aiTrigger}
					<span class="tb-sep"></span>
					{@render aiTrigger()}
				{/if}
			</div>
		{/if}
		<div
			bind:this={element}
			class="prose-content"
			onkeydown={slashKeyDown}
			role="textbox"
			tabindex="-1"
		></div>
	</div>

	<!-- Mode switcher — floated at bottom-right of the editing surface -->
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
					onmousedown={(e) => { e.preventDefault(); runBlock(block); }}
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

{#if mediaPickerOpen}
	<MediaLibraryModal onSelect={handleMediaSelect} onClose={() => (mediaPickerOpen = false)} />
{/if}

{#if insertModalKind}
	<InsertModal
		kind={insertModalKind}
		currentUrl={insertCurrentUrl}
		onConfirm={handleInsertConfirm}
		onClose={closeInsertModal}
	/>
{/if}

<style>
	.post-editor {
		position: relative;
	}

	/* ---- Mode switcher — floated at bottom-right of the editing surface ---- */
	.mode-switcher {
		position: absolute;
		bottom: 20px;
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
		border-radius: 0;
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: color 0.12s;
	}
	.mode-tab:hover {
		color: var(--color-text);
	}
	.mode-tab.active {
		background: var(--color-surface);
		color: var(--color-primary);
	}

	/* ---- WYSIWYG toolbar — docked at the top inside the editor card ---- */
	.toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 2px;
		padding: 6px 8px;
		border-bottom: 1px solid var(--color-border);
		border-radius: var(--radius) var(--radius) 0 0;
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
		min-height: 355px;
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

	/* The editable region fills the card; the toolbar docks at the top
	 * inside the same border. */
	.prose-content {
		flex: 1 1 auto;
		min-height: 355px;
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
		min-height: 305px;
	}

	/* Link preview block node — styled container inside the editor. */
	.prose-content :global(div[data-type="link-preview"]) {
		margin: 1rem 0;
		max-width: 100%;
	}

	.prose-content :global(div[data-type="link-preview"]:hover) {
		outline: 2px solid var(--color-primary);
		outline-offset: 2px;
		border-radius: 0.625rem;
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
	.prose-content :global(.youtube-embed) {
		position: relative;
		overflow: hidden;
		width: 100%;
		height: 0;
		padding-bottom: 56.25%;
		margin: 0.75rem 0;
		border-radius: var(--radius);
	}
	.prose-content :global(.youtube-embed iframe) {
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
		height: 100%;
		border: 0;
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
		z-index: 50;
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
