<script lang="ts">
	// RichTextEditor — shared TipTap editor extracted from the main app's
	// PostEditor (sections/15-tiptap-rich-text.md). App-specific pieces are
	// injected as props: i18n `t`, suggestion search, coupon search,
	// link-preview fetch, image picking. Feature nodes are opt-in via
	// `features` so surfaces without those APIs get a clean reduced schema.
	import { untrack, mount, unmount } from 'svelte';
	import { tick as svelteTick } from 'svelte';
	import { Editor } from '@tiptap/core';
	import { buildEditorExtensions } from './extensions';
	import { SLASH_BLOCKS } from './slashMenu';
	import type { SlashBlockContext } from './slashMenu';
	import type {
		EditorFeatures,
		EditorMode,
		EditorSearchHit,
		EditorTranslator,
		LinkPreviewData
	} from './types';
	import EmojiPicker from './EmojiPicker.svelte';
	import EditorInsertModal from './EditorInsertModal.svelte';
	import LinkPreviewCard from './LinkPreviewCard.svelte';
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
		FileCode,
		Rows3,
		Columns3,
		Trash2
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

	type InsertKind = 'youtube' | 'link' | 'coupon';

	interface Props {
		/**
		 * Initial content: a TipTap doc JSON object, an HTML string, or a
		 * markdown string (anything not starting with `<` is parsed as
		 * markdown when the markdown feature is enabled, else as plain text
		 * HTML-escaped by TipTap's parser).
		 */
		content?: unknown;
		/** Called on every document edit (not on initial seed). */
		onedit?: () => void;
		/** Serialized HTML after each update / seed. */
		onhtml?: (html: string) => void;
		/** TipTap JSON doc after each update / seed. */
		onjson?: (json: Record<string, unknown>) => void;
		/** Markdown serialization after each update — for surfaces that
		 * persist markdown (e.g. academy article lessons). Requires the
		 * markdown feature (enabled by default). */
		onmarkdown?: (markdown: string) => void;
		onfocus?: () => void;
		placeholder?: string;
		/** Mode tab initially active. */
		defaultMode?: EditorMode;
		/** Which mode tabs to offer; one entry hides the switcher.
		 *  'markdown' is dropped if the markdown feature is off. */
		modes?: EditorMode[];
		/** i18n translator injected by the host app. */
		t?: EditorTranslator;
		/** Suggestion-search provider for @/#/[[/{{ pickers. */
		onSearch?: (
			kind: 'users' | 'content' | 'tags' | 'literals',
			query: string
		) => Promise<EditorSearchHit[]>;
		/** Coupon search provider for the coupon insert modal. */
		onSearchCoupons?: (query: string) => Promise<EditorSearchHit[]>;
		/** Link-preview metadata fetcher — enables preview-card hydration. */
		fetchLinkPreview?: (url: string) => Promise<LinkPreviewData | null>;
		/** Host image picker. Called with an `insert(src, alt)` function;
		 * the host opens its media UI and calls insert on selection. When
		 * absent, image insertion falls back to a URL prompt. */
		onPickImage?: (insert: (src: string, alt?: string) => void) => void;
		/** Feature flags for optional nodes/UI. */
		features?: EditorFeatures;
		/** Optional floating trigger rendered bottom-left (e.g. AI assist). */
		aiTrigger?: import('svelte').Snippet;
		/** Minimum height of the editable region. */
		minHeight?: number;
		/** aria-label for the editable region. */
		ariaLabel?: string;
	}

	let {
		content = null,
		onedit,
		onhtml,
		onjson,
		onmarkdown,
		onfocus,
		placeholder,
		defaultMode,
		modes,
		t = (_k, fb) => fb ?? _k,
		onSearch,
		onSearchCoupons,
		fetchLinkPreview,
		onPickImage,
		features,
		aiTrigger,
		minHeight = 355,
		ariaLabel
	}: Props = $props();

	const feat = $derived<Required<EditorFeatures>>({
		suggestions: features?.suggestions ?? !!onSearch,
		coupon: features?.coupon ?? !!onSearchCoupons,
		linkPreview: features?.linkPreview ?? !!fetchLinkPreview,
		video: features?.video ?? false,
		image: features?.image ?? true,
		youtube: features?.youtube ?? true,
		markdown: features?.markdown ?? true
	});

	// Visible mode tabs — markdown requires the markdown extension.
	const modeTabs = $derived.by<EditorMode[]>(() => {
		const base = modes ?? ['block', 'wysiwyg', 'markdown'];
		return base.filter((m) => m !== 'markdown' || feat.markdown);
	});

	let element = $state<HTMLDivElement>(null!);
	let postEditorEl = $state<HTMLDivElement>(null!);
	// $state.raw — the Editor instance must not be deep-proxied.
	let editor = $state.raw<Editor | null>(null);
	// Bumped on every transaction so toolbar isActive states re-render.
	let txTick = $state(0);
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

	let mode = $state<EditorMode>(untrack(() => defaultMode ?? 'block'));
	// Raw markdown source for markdown mode. Kept in sync with the Tiptap
	// document when switching modes so round-trips are lossless.
	let markdownSource = $state('');
	// Whether the markdown textarea has unsynced local edits (used to avoid
	// clobbering the textarea while the user is typing in markdown mode).
	let markdownDirty = $state(false);

	// Content-seeding markers. Plain (non-reactive) on purpose: the sync
	// effect compares `content` against these to decide whether it holds a
	// genuinely new external value — the value last consumed via seedContent,
	// or the serialized output we last emitted to the parent (parents often
	// feed the emitted value straight back in, which must not reseed).
	let lastSeeded: unknown = null;
	let lastEmittedHtml = '';
	let lastEmittedMarkdown = '';
	let lastEmittedJson: unknown = null;

	const MODE_DEFS: Record<EditorMode, { icon: typeof Heading2; labelKey: string; descKey: string; fallback: string; descFallback: string }> = {
		block: { icon: LayoutTemplate, labelKey: 'create.editor.mode.block', descKey: 'create.editor.mode.block_desc', fallback: 'Blocks', descFallback: 'Block editor — type / for blocks' },
		wysiwyg: { icon: Type, labelKey: 'create.editor.mode.wysiwyg', descKey: 'create.editor.mode.wysiwyg_desc', fallback: 'Rich text', descFallback: 'Rich text with toolbar' },
		markdown: { icon: FileCode, labelKey: 'create.editor.mode.markdown', descKey: 'create.editor.mode.markdown_desc', fallback: 'Markdown', descFallback: 'Write raw markdown' }
	};

	// Hydrate link-preview placeholder divs inside the editor DOM into
	// LinkPreviewCard Svelte components. Called after each editor update.
	function hydrateEditorLinkPreviews() {
		if (!element || !feat.linkPreview || !fetchLinkPreview) return;
		const placeholders = element.querySelectorAll<HTMLElement>(
			'div[data-type="link-preview"]:not([data-hydrated])'
		);
		for (const el of placeholders) {
			const url = el.dataset.url;
			if (!url) continue;
			el.setAttribute('data-hydrated', 'true');
			const comp = mount(LinkPreviewCard, {
				target: el,
				props: { url, fetchPreview: fetchLinkPreview, t }
			});
			lpMounts.set(el, comp as Record<string, unknown>);
		}
	}

	function emitAll() {
		if (!editor || editor.isDestroyed) return;
		const html = editor.getHTML() ?? '';
		const json = editor.getJSON() ?? { type: 'doc', content: [] };
		const md = feat.markdown ? editor.getMarkdown() : '';
		lastEmittedHtml = html;
		lastEmittedJson = json;
		lastEmittedMarkdown = md;
		onhtml?.(html);
		onjson?.(json);
		if (feat.markdown) onmarkdown?.(md);
	}

	// Seed the editor from the `content` prop — JSON doc, HTML string, or
	// markdown/plain string.
	function seedContent(initial: unknown): boolean {
		if (!editor || editor.isDestroyed || initial == null || initial === '') return false;
		lastSeeded = initial;
		if (typeof initial === 'object') {
			editor.commands.setContent(initial as never, { emitUpdate: false });
		} else if (typeof initial === 'string') {
			if (initial.trim().startsWith('<')) {
				editor.commands.setContent(initial, { emitUpdate: false });
			} else if (feat.markdown) {
				editor.commands.setContent(initial, { contentType: 'markdown', emitUpdate: false });
			} else {
				editor.commands.setContent(initial, { emitUpdate: false });
			}
		} else {
			return false;
		}
		return true;
	}

	// Initialise the TipTap editor once the mount element is available.
	// All reactive reads are untracked so the effect only re-runs when
	// `element` changes — not on every editor update.
	$effect(() => {
		if (!element) return;

		const initialContent = untrack(() => content);
		const initialMode = untrack(() => mode);
		const placeholderText = untrack(() => placeholder ?? t('create.editor.placeholder', 'Write something…'));
		const initialFeat = untrack(() => feat);
		const initialSearch = untrack(() => onSearch);

		const ed = new Editor({
			element: element,
			extensions: buildEditorExtensions({
				placeholder: placeholderText,
				onSearch: initialFeat.suggestions ? initialSearch : undefined,
				startTypingText: untrack(() => t('create.editor.suggestion.start_typing', 'Start typing…')),
				noResultsText: untrack(() => t('create.editor.suggestion.no_results', 'No results found')),
				features: {
					image: initialFeat.image,
					youtube: initialFeat.youtube,
					video: initialFeat.video,
					coupon: initialFeat.coupon,
					linkPreview: initialFeat.linkPreview,
					markdown: initialFeat.markdown
				}
			}),
			content: '',
			editable: initialMode !== 'markdown',
			onCreate: () => {
				requestAnimationFrame(() => {
					if (!ed || ed.isDestroyed) return;
					if (seedContent(initialContent)) {
						if (untrack(() => mode) === 'markdown' && initialFeat.markdown) {
							markdownSource = ed.getMarkdown();
							markdownDirty = false;
						}
					}
					// Sync the initial parsed document back to the parent so
					// edit forms can submit without requiring a keystroke.
					emitAll();
					hydrateEditorLinkPreviews();
				});
			},
			onUpdate: () => {
				emitAll();
				onedit?.();
				if (untrack(() => mode) !== 'markdown') markdownDirty = false;
				requestAnimationFrame(hydrateEditorLinkPreviews);
			},
			onTransaction: () => {
				// Deferred to a microtask: setContent inside the content-sync
				// $effect would otherwise write $state mid-effect (Svelte
				// infinite-loop guard), and rapid transactions coalesce.
				queueMicrotask(() => {
					txTick += 1;
				});
			},
			onFocus: () => onfocus?.(),
			onSelectionUpdate: ({ editor }) => {
				// Selection state must be captured synchronously, but the writes
				// are deferred — setContent inside the content-sync $effect fires
				// this handler mid-effect, and writing $state there trips
				// Svelte's infinite-loop guard.
				const { from } = editor.state.selection;
				const text = editor.state.doc.textBetween(1, from, '\n');
				const match = /\/(\w*)$/.exec(text);
				queueMicrotask(() => {
					if (untrack(() => mode) !== 'block') {
						slashOpen = false;
						return;
					}
					if (match) {
						slashOpen = true;
						slashQuery = match[1];
						slashIndex = 0;
						requestAnimationFrame(() => {
							if (!ed || ed.isDestroyed) return;
							const pel = untrack(() => postEditorEl);
							if (!pel) return;
							const coords = ed.view.coordsAtPos(from, -1);
							const containerRect = pel.getBoundingClientRect();
							slashLeft = coords.left - containerRect.left;
							slashTop = coords.bottom - containerRect.top + 6;
						});
					} else {
						slashOpen = false;
					}
				});
			}
		});
		editor = ed;

		// DOM-level paste listener for link preview (ROADMAP-MAIN 07.09).
		// Uses capture phase to fire before ProseMirror's internal handling.
		// Only active when the linkPreview feature is on — otherwise pasting
		// a bare URL stays a normal text paste.
		if (initialFeat.linkPreview && fetchLinkPreview) {
			function handlePasteEvent(e: ClipboardEvent) {
				const text = e.clipboardData?.getData('text/plain') ?? '';
				const trimmed = text.trim();
				if (!/^https?:\/\/\S+$/i.test(trimmed) || !ed || ed.isDestroyed) return;

				const { from, to } = ed.state.selection;
				if (from !== to) {
					// Text is selected — apply the URL as a link mark to the
					// selected text (inline link), don't insert a preview card.
					e.preventDefault();
					e.stopPropagation();
					ed.chain().focus().setLink({ href: trimmed }).run();
				} else {
					// No selection (cursor only) — insert a linkPreview block
					// node. The onUpdate handler hydrates it into a card.
					e.preventDefault();
					e.stopPropagation();
					ed.chain().focus().insertLinkPreview(trimmed).run();
				}
			}
			element.addEventListener('paste', handlePasteEvent, true);
			pasteHandler = handlePasteEvent;
		}

		// Cleanup — Svelte 5 $effect cleanup function.
		return () => {
			if (pasteHandler && element) {
				element.removeEventListener('paste', pasteHandler, true);
			}
			for (const [, comp] of lpMounts) {
				unmount(comp);
			}
			lpMounts.clear();
			ed.destroy();
			editor = null;
		};
	});

	// Slash blocks filtered by feature availability.
	const availableBlocks = $derived(
		SLASH_BLOCKS.filter((b) => {
			if (b.key === 'image') return feat.image;
			if (b.key === 'youtube') return feat.youtube;
			if (b.key === 'coupon') return feat.coupon;
			return true;
		})
	);

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
			onInsertImage: pickImage,
			onInsertYoutube: () => openInsertModal('youtube'),
			onInsertLink: () => openInsertModal('link'),
			onInsertCoupon: () => openInsertModal('coupon')
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
			? availableBlocks.filter((b) => b.key.toLowerCase().includes(slashQuery.toLowerCase()))
			: availableBlocks
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
	// document and propagates the resulting content to the parent. Called on
	// mode switch (immediate) and on a debounce after typing in markdown mode.
	function syncMarkdownToEditor() {
		if (!editor) return;
		editor.commands.setContent(markdownSource, { contentType: 'markdown', emitUpdate: false });
		emitAll();
		onedit?.();
		markdownDirty = false;
		// Hydrate link-preview nodes that were parsed from the markdown
		// source (the placeholder divs become linkPreview nodes via
		// parseHTML, but need hydration into LinkPreviewCard components).
		requestAnimationFrame(hydrateEditorLinkPreviews);
	}

	function onMarkdownInput(e: Event) {
		markdownSource = (e.currentTarget as HTMLTextAreaElement).value;
		markdownDirty = true;
		onmarkdown?.(markdownSource);
		onedit?.();
		// Debounce the markdown→doc conversion so the parent's snapshot
		// stays current without parsing markdown on every keystroke.
		if (markdownSyncTimer) clearTimeout(markdownSyncTimer);
		markdownSyncTimer = setTimeout(syncMarkdownToEditor, MARKDOWN_SYNC_DELAY);
	}

	// Clear the markdown sync timer on cleanup (Svelte 5 $effect cleanup).
	$effect(() => {
		return () => {
			if (markdownSyncTimer) clearTimeout(markdownSyncTimer);
		};
	});

	// Keep the editor document in sync with external `content` updates (e.g.
	// a host applies generated text). The guard is value-based, not
	// serialization-based: comparing `content` against getHTML()/getMarkdown()
	// is unreliable (serialization normalizes the doc, so the two are rarely
	// byte-identical), which turned every parent re-render into a reseed and
	// let bound-back emissions loop the effect. Instead we skip values we
	// already consumed or emitted ourselves; only a genuinely new external
	// value reseeds the document.
	$effect(() => {
		if (!editor || editor.isDestroyed) return;
		const c = content;
		if (
			c == null ||
			c === '' ||
			c === lastSeeded ||
			c === lastEmittedJson ||
			c === lastEmittedHtml ||
			c === lastEmittedMarkdown
		)
			return;
		if (seedContent(c)) {
			if (untrack(() => mode) === 'markdown' && feat.markdown) {
				markdownSource = editor.getMarkdown();
				markdownDirty = false;
			}
			requestAnimationFrame(hydrateEditorLinkPreviews);
		}
	});

	// ---- Toolbar helpers ----
	function tb(cmd: string, ...args: unknown[]) {
		if (!editor) return;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const chain = editor.chain().focus() as any;
		chain[cmd](...args).run();
	}

	function isActive(name: string, attrs?: Record<string, unknown>): boolean {
		txTick;
		return editor?.isActive(name, attrs) ?? false;
	}

	function canRun(cmd: 'undo' | 'redo'): boolean {
		txTick;
		return editor?.can()[cmd]() ?? false;
	}

	const inTable = $derived.by(() => {
		txTick;
		return editor?.isActive('table') ?? false;
	});

	function setLinkFromPrompt() {
		if (!editor) return;
		openInsertModal('link');
	}

	function pickImage() {
		if (!editor) return;
		if (onPickImage) {
			onPickImage((src, alt) => insertImage(src, alt));
			return;
		}
		const raw = window.prompt('Image URL');
		if (raw) insertImage(raw);
	}

	/** Insert an image at the cursor — callable by hosts via bind:this too. */
	export function insertImage(src: string, alt = '') {
		if (!editor || editor.isDestroyed) return;
		const trimmed = src.trim();
		if (!trimmed || /^(javascript|data|vbscript):/i.test(trimmed)) return;
		editor.chain().focus().setImage({ src: trimmed, alt }).run();
	}

	function insertEmoji(emoji: string) {
		if (!editor) return;
		editor.chain().focus().insertContent(emoji).run();
	}

	// ---- Insert modal (YouTube / Link / Coupon) ----
	let insertModalKind = $state<InsertKind | null>(null);
	let insertCurrentUrl = $state('');

	function openInsertModal(kind: InsertKind) {
		if (kind === 'coupon' && !feat.coupon) return;
		if (kind === 'youtube' && !feat.youtube) return;
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
		const { from, to } = editor.state.selection;
		if (/^https?:\/\//i.test(trimmed)) {
			if (from === to && feat.linkPreview) {
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				(editor.commands as any).insertLinkPreview?.(trimmed) ??
					editor.chain().focus().insertContent({
						type: 'text',
						text: trimmed,
						marks: [{ type: 'link', attrs: { href: trimmed } }]
					}).run();
			} else {
				editor.chain().focus().extendMarkRange('link').setLink({ href: trimmed }).run();
			}
		} else {
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

	export function getJSON(): Record<string, unknown> {
		return editor?.getJSON() ?? { type: 'doc', content: [] };
	}

	export function getMarkdown(): string {
		if (mode === 'markdown') return markdownSource;
		return editor?.getMarkdown() ?? '';
	}

	/** Flush any pending markdown → doc sync before the host reads state
	 *  (e.g. on a save click while the debounce is still pending). */
	export function flush() {
		if (markdownSyncTimer) {
			clearTimeout(markdownSyncTimer);
			markdownSyncTimer = null;
		}
		if (mode === 'markdown' && markdownDirty) {
			syncMarkdownToEditor();
		}
	}

	/**
	 * Append a TipTap node (or a whole `{type:'doc',content:[…]}` document's
	 * content) at the end of the editor. Used by the video composer's
	 * VideoField to insert the backend-built `video` node (§106).
	 */
	export function appendVideoNode(json: Record<string, unknown>) {
		if (!editor || editor.isDestroyed) return;
		const content =
			json && json.type === 'doc' && Array.isArray(json.content) ? json.content : json;
		editor.chain().focus('end').insertContent(content).run();
	}

	// Markdown mode paste handler — inserts a link preview placeholder as
	// raw HTML into the markdown source (only when linkPreview is on).
	function handleMarkdownPaste(e: ClipboardEvent) {
		if (!feat.linkPreview) return;
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
		onmarkdown?.(markdownSource);
		onedit?.();
		// Debounce the markdown→doc conversion so the editor document updates.
		if (markdownSyncTimer) clearTimeout(markdownSyncTimer);
		markdownSyncTimer = setTimeout(syncMarkdownToEditor, MARKDOWN_SYNC_DELAY);
		// Move cursor after the inserted placeholder.
		svelteTick().then(() => {
			const pos = start + placeholder.length;
			ta.focus();
			ta.setSelectionRange(pos, pos);
		});
	}
</script>

<div class="post-editor" bind:this={postEditorEl}>
	{#if mode === 'markdown' && feat.markdown}
		<textarea
			bind:this={markdownTextarea}
			class="markdown-source"
			value={markdownSource}
			oninput={onMarkdownInput}
			onfocus={() => onfocus?.()}
			onpaste={handleMarkdownPaste}
			placeholder={t('create.editor.placeholder_markdown', 'Write markdown…')}
			spellcheck="true"
		></textarea>
	{/if}

	<!-- The editor card wraps the Tiptap mount and (in WYSIWYG mode) the
	     formatting toolbar docked at the top inside the same border.
	     The .editor-surface is always mounted so the Editor instance keeps
	     its DOM node across mode switches; hidden via CSS in markdown mode. -->
	<div class="editor-surface" class:hidden={mode === 'markdown' && feat.markdown}>
		{#if mode === 'wysiwyg' && editor}
			<div class="toolbar" role="toolbar" aria-label={t('create.editor.toolbar.label', 'Formatting')}>
				<button type="button" class="tb-btn" class:active={isActive('bold')} title={t('create.editor.toolbar.bold', 'Bold')} onclick={() => tb('toggleBold')}><Bold size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('italic')} title={t('create.editor.toolbar.italic', 'Italic')} onclick={() => tb('toggleItalic')}><Italic size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('underline')} title={t('create.editor.toolbar.underline', 'Underline')} onclick={() => tb('toggleUnderline')}><Underline size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('strike')} title={t('create.editor.toolbar.strike', 'Strikethrough')} onclick={() => tb('toggleStrike')}><Strikethrough size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('code')} title={t('create.editor.toolbar.code', 'Inline code')} onclick={() => tb('toggleCode')}><Code size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('heading', { level: 2 })} title={t('create.editor.toolbar.h2', 'Heading 2')} onclick={() => tb('toggleHeading', { level: 2 })}><Heading2 size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('heading', { level: 3 })} title={t('create.editor.toolbar.h3', 'Heading 3')} onclick={() => tb('toggleHeading', { level: 3 })}><Heading3 size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('bulletList')} title={t('create.editor.toolbar.bullet_list', 'Bullet list')} onclick={() => tb('toggleBulletList')}><List size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('orderedList')} title={t('create.editor.toolbar.ordered_list', 'Numbered list')} onclick={() => tb('toggleOrderedList')}><ListOrdered size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('blockquote')} title={t('create.editor.toolbar.blockquote', 'Quote')} onclick={() => tb('toggleBlockquote')}><Quote size={16} /></button>
				<button type="button" class="tb-btn" class:active={isActive('codeBlock')} title={t('create.editor.toolbar.code_block', 'Code block')} onclick={() => tb('toggleCodeBlock')}><Code2 size={16} /></button>
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" class:active={isActive('link')} title={t('create.editor.toolbar.link', 'Link')} onclick={setLinkFromPrompt}><LinkIcon size={16} /></button>
				{#if feat.image}
					<button type="button" class="tb-btn" title={t('create.editor.toolbar.image', 'Image')} onclick={pickImage}><ImagePlus size={16} /></button>
				{/if}
				{#if feat.youtube}
					<button type="button" class="tb-btn" title={t('create.editor.toolbar.youtube', 'YouTube')} onclick={() => openInsertModal('youtube')}><Video size={16} /></button>
				{/if}
				{#if feat.coupon}
					<button type="button" class="tb-btn" title={t('create.editor.toolbar.coupon', 'Coupon')} onclick={() => openInsertModal('coupon')}><Ticket size={16} /></button>
				{/if}
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.divider', 'Divider')} onclick={() => tb('setHorizontalRule')}><Minus size={16} /></button>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.table', 'Table')} onclick={() => tb('insertTable', { rows: 3, cols: 3, withHeaderRow: true })}><Table size={16} /></button>
				{#if inTable}
					<button type="button" class="tb-btn" title={t('create.editor.toolbar.add_row', 'Add row')} onclick={() => tb('addRowAfter')}><Rows3 size={16} /></button>
					<button type="button" class="tb-btn" title={t('create.editor.toolbar.add_col', 'Add column')} onclick={() => tb('addColumnAfter')}><Columns3 size={16} /></button>
					<button type="button" class="tb-btn tb-danger" title={t('create.editor.toolbar.delete_row', 'Delete row')} onclick={() => tb('deleteRow')}><Trash2 size={16} /></button>
					<button type="button" class="tb-btn tb-danger" title={t('create.editor.toolbar.delete_col', 'Delete column')} onclick={() => tb('deleteColumn')}><Trash2 size={16} /></button>
					<button type="button" class="tb-btn tb-danger" title={t('create.editor.toolbar.delete_table', 'Delete table')} onclick={() => tb('deleteTable')}><Table size={16} /></button>
				{/if}
				<span class="tb-sep"></span>
				<EmojiPicker onselect={insertEmoji} {t} compact />
				<span class="tb-sep"></span>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.undo', 'Undo')} onclick={() => tb('undo')} disabled={!canRun('undo')}><Undo2 class="rtl:-scale-x-100" size={16} /></button>
				<button type="button" class="tb-btn" title={t('create.editor.toolbar.redo', 'Redo')} onclick={() => tb('redo')} disabled={!canRun('redo')}><Redo2 class="rtl:-scale-x-100" size={16} /></button>
			</div>
		{/if}
		<div
			bind:this={element}
			class="prose-content"
			style="--rte-min-height: {minHeight}px"
			onkeydown={slashKeyDown}
			role="textbox"
			aria-label={ariaLabel ?? placeholder ?? t('create.editor.content_label', 'Content')}
			tabindex="-1"
		></div>
	</div>

	<!-- AI trigger — floated at the bottom-right, left of the mode switcher -->
	{#if aiTrigger}
		<div class="ai-trigger">
			{@render aiTrigger()}
		</div>
	{/if}

	<!-- Mode switcher — floated at bottom-right of the editing surface -->
	{#if modeTabs.length > 1}
		<div class="mode-switcher" role="tablist" aria-label={t('create.editor.mode.label', 'Editor mode')}>
			{#each modeTabs as m (m)}
				{@const def = MODE_DEFS[m]}
				<button
					type="button"
					role="tab"
					aria-selected={mode === m}
					class="mode-tab"
					class:active={mode === m}
					title={t(def.descKey, def.descFallback)}
					aria-label={t(def.labelKey, def.fallback)}
					onclick={() => switchMode(m)}
				>
					<def.icon size={16} />
				</button>
			{/each}
		</div>
	{/if}

	{#if mode === 'block' && slashOpen && filteredBlocks.length > 0}
		<div class="slash-menu" style="top: {slashTop}px; inset-inline-start: {slashLeft}px">
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
						<span class="slash-label">{t(block.labelKey, block.label)}</span>
						<span class="slash-desc">{t(block.descKey, block.desc)}</span>
					</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

{#if insertModalKind}
	<EditorInsertModal
		kind={insertModalKind}
		currentUrl={insertCurrentUrl}
		{onSearchCoupons}
		{t}
		onConfirm={handleInsertConfirm}
		onClose={closeInsertModal}
	/>
{/if}

<style>
	.post-editor {
		position: relative;
	}

	/* ---- AI trigger — floated at the bottom-left of the editing surface ---- */
	.ai-trigger {
		position: absolute;
		bottom: 20px;
		inset-inline-start: 20px;
		z-index: 20;
	}

	/* ---- Mode switcher — floated at bottom-right of the editing surface ---- */
	.mode-switcher {
		position: absolute;
		bottom: 20px;
		inset-inline-end: 8px;
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
		width: 40px;
		height: 40px;
		padding: 0;
		border: none;
		border-radius: 0;
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: color 0.12s;
	}
	.mode-tab :global(svg) {
		width: 20px;
		height: 20px;
	}
	.mode-tab:hover {
		color: var(--color-text);
	}
	.mode-tab.active {
		background: var(--color-surface);
		color: var(--color-primary);
	}

	/* ---- WYSIWYG toolbar — docked at the top inside the editor card ---- */
	/* The toolbar is always a single horizontally-scrollable row — items
	 * never wrap to a second line. flex-shrink: 0 keeps buttons at their
	 * set width instead of being compressed when the row overflows. */
	.toolbar {
		display: flex;
		flex-wrap: nowrap;
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
		align-items: center;
		gap: 2px;
		padding: 6px 8px;
		border-bottom: 1px solid var(--color-border);
		border-radius: var(--radius) var(--radius) 0 0;
		background: var(--color-surface-subtle);
	}
	.toolbar > * {
		flex-shrink: 0;
	}
	.tb-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--color-text);
		cursor: pointer;
		transition: background 0.1s, color 0.1s;
	}
	.tb-btn :global(svg) {
		width: 20px;
		height: 20px;
	}
	.tb-btn:hover:not(:disabled) {
		background: var(--color-surface-subtle);
	}
	.tb-btn.active {
		background: var(--color-primary);
		color: var(--color-surface);
	}
	.tb-btn.tb-danger:hover:not(:disabled) {
		color: var(--color-danger, #dc2626);
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

	/* On desktop the toolbar can be kept to one compact row by shrinking the
	 * buttons slightly and tightening the gaps and separators. */
	@media (min-width: 768px) {
		.toolbar {
			gap: 1px;
			padding: 6px 8px;
		}
		.tb-btn {
			width: 36px;
			height: 36px;
		}
		.tb-btn :global(svg) {
			width: 18px;
			height: 18px;
		}
		.tb-sep {
			margin: 0 2px;
		}
	}

	/* ---- Markdown source textarea ---- */
	.markdown-source {
		width: 100%;
		min-height: var(--rte-min-height, 355px);
		padding: 10px 14px;
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
		outline: none;
	}

	/* The editor card — a rounded surface wrapping the Tiptap mount and
	 * (in WYSIWYG mode) the docked formatting toolbar at the top. */
	.editor-surface {
		display: flex;
		flex-direction: column;
		background: var(--color-surface);
		border-radius: var(--radius);
		overflow: hidden;
		transition: border-color 0.15s, box-shadow 0.15s;
	}

	/* The editable region fills the card; the toolbar docks at the top
	 * inside the same border. */
	.prose-content {
		flex: 1 1 auto;
		min-height: var(--rte-min-height, 355px);
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
		min-height: calc(var(--rte-min-height, 355px) - 50px);
	}
	/* The host apps' global *:focus-visible outline paints a border around
	 * the contenteditable on focus — suppress it; the surface carries the
	 * visual state instead. */
	.prose-content :global(.ProseMirror:focus-visible) {
		outline: none;
	}
	.prose-content :global(.ProseMirror p.is-editor-empty:first-child::before) {
		content: attr(data-placeholder);
		color: var(--color-muted);
		float: inline-start;
		height: 0;
		pointer-events: none;
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
		padding-inline-start: 1.5rem;
		margin: 0.5rem 0;
	}
	.prose-content :global(ol) {
		list-style: decimal;
		padding-inline-start: 1.5rem;
		margin: 0.5rem 0;
	}
	.prose-content :global(li) {
		margin: 0.25rem 0;
	}
	.prose-content :global(blockquote) {
		border-inline-start: 3px solid var(--color-primary);
		padding-inline-start: 1rem;
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
		inset-inline-start: 0;
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
		text-align: start;
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
		inset-inline-end: -2px;
		top: 0;
		bottom: 0;
		width: 4px;
		background: var(--color-primary);
		pointer-events: none;
	}
	.prose-content :global(.resize-cursor) {
		cursor: col-resize;
	}

	/* Suggestion popup — rendered at document.body by suggestionRenderer,
	 * so it needs :global selectors. */
	:global(.suggestion-popup) {
		position: fixed;
		z-index: 100;
		display: flex;
		flex-direction: column;
		min-width: 180px;
		max-height: 240px;
		overflow-y: auto;
		padding: 4px;
		border: 1px solid var(--color-border);
		border-radius: 8px;
		background: var(--color-surface);
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.15);
		font-size: 0.875rem;
		color: var(--color-text-subtle);
	}
	:global(.suggestion-popup .suggestion-item) {
		display: block;
		width: 100%;
		padding: 6px 10px;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: var(--color-text);
		text-align: start;
		cursor: pointer;
	}
	:global(.suggestion-popup .suggestion-item.active) {
		background: var(--color-surface-subtle);
		color: var(--color-primary);
	}

	/* Mention / reference chips rendered inside the editor. */
	.prose-content :global(.mention),
	.prose-content :global(.hashtag),
	.prose-content :global(.interlink) {
		color: var(--color-primary);
		border-radius: 4px;
		padding: 0 2px;
	}
	.prose-content :global(.mention) {
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
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
		text-align: start;
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
