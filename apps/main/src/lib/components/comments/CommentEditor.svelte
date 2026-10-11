<script lang="ts">
	/**
	 * Minimal TipTap WYSIWYG editor for comments (Section 49).
	 * Supports blocks suitable for comments: paragraph, bold, italic,
	 * strike, code, link, image, and @mention. Uses the shared
	 * EditorInsertModal link dialog and MediaLibraryModal image picker.
	 *
	 * Ref: japaconnect-engineering/sections/49-comments-replies.md
	 */

	import { onMount, onDestroy } from 'svelte';
	import { Editor } from '@tiptap/core';
	import { StarterKit } from '@tiptap/starter-kit';
	import Link from '@tiptap/extension-link';
	import Image from '@tiptap/extension-image';
	import Placeholder from '@tiptap/extension-placeholder';
	import Mention from '@tiptap/extension-mention';
	import { PluginKey } from '@tiptap/pm/state';
	import { Bold, Italic, Link as LinkIcon, Code, Strikethrough, ImagePlus } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { searchHits } from '$lib/api/create';
	import { createSuggestionRenderer } from '@japaconnect/ui';
	import { encodeTiptapDoc, decodeTipTapDoc, type TipTapDoc } from '$lib/tiptap/serialize';
	import { EmojiPicker, EditorInsertModal } from '@japaconnect/ui';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import type { MediaAsset } from '$lib/types';

	interface Props {
		/** Placeholder text for the editor. */
		placeholder?: string;
		/** Initial content as base64 TipTap JSON (for edit mode). */
		initialBody?: string | null;
		/** Called whenever the editor content changes (with base64 TipTap JSON). */
		oninput?: (body: string) => void;
		/** CSS class for the editor container. */
		class?: string;
	}

	let { placeholder = '', initialBody = null, oninput, class: cls = '' }: Props = $props();

	let editor: Editor | null = null;
	let element: HTMLDivElement;
	const mentionPluginKey = new PluginKey('commentMention');

	// Reactive mark states — updated on editor selection/transaction.
	let isBold = $state(false);
	let isItalic = $state(false);
	let isStrike = $state(false);
	let isCode = $state(false);
	let isLink = $state(false);

	function syncMarks() {
		if (!editor) return;
		isBold = editor.isActive('bold');
		isItalic = editor.isActive('italic');
		isStrike = editor.isActive('strike');
		isCode = editor.isActive('code');
		isLink = editor.isActive('link');
	}

	onMount(() => {
		// Decode initial body if provided (edit mode).
		let initialDoc: TipTapDoc | null = null;
		if (initialBody) {
			initialDoc = decodeTipTapDoc(initialBody) as TipTapDoc | null;
		}

		// Minimal extensions for comments — no headings, tables, lists, etc.
		const mentionExt = Mention.configure({
			HTMLAttributes: { class: 'mention' },
			suggestion: {
				char: '@',
				allowSpaces: true,
				pluginKey: mentionPluginKey,
				minQueryLength: 2,
				items: ({ query }) => searchHits('users', query),
				render: () => createSuggestionRenderer({ entityType: 'user' })
			}
		}).extend({
			name: 'mention',
			addAttributes() {
				return {
					id: { default: null },
					label: { default: null },
					handle: { default: null },
					entity_type: { default: 'user' }
				};
			}
		});

		editor = new Editor({
			element: element,
			extensions: [
				StarterKit.configure({
					heading: false,
					bulletList: false,
					orderedList: false,
					blockquote: false,
					codeBlock: false,
					horizontalRule: false,
					link: false
				}),
				Link.configure({
					openOnClick: false,
					autolink: true,
					HTMLAttributes: { rel: 'noopener noreferrer', target: '_blank' }
				}),
				Image.configure({
					inline: false,
					allowBase64: false,
					HTMLAttributes: { loading: 'lazy' }
				}),
				Placeholder.configure({ placeholder }),
				mentionExt
			],
			content: initialDoc ?? '',
			onUpdate: () => {
				if (oninput && editor) {
					oninput(encodeTiptapDoc(editor.getJSON() as TipTapDoc));
				}
			},
			onSelectionUpdate: () => syncMarks(),
			onTransaction: () => syncMarks()
		});
		syncMarks();
	});

	onDestroy(() => {
		editor?.destroy();
		editor = null;
	});

	// Toolbar actions.
	function toggleBold() { editor?.chain().focus().toggleBold().run(); syncMarks(); }
	function toggleItalic() { editor?.chain().focus().toggleItalic().run(); syncMarks(); }
	function toggleStrike() { editor?.chain().focus().toggleStrike().run(); syncMarks(); }
	function toggleCode() { editor?.chain().focus().toggleCode().run(); syncMarks(); }

	function insertEmoji(emoji: string) {
		editor?.chain().focus().insertContent(emoji).run();
		syncMarks();
	}

	// ---- Link modal (same InsertModal as PostEditor) ----
	let linkModalOpen = $state(false);
	let linkCurrentUrl = $state('');

	function openLinkModal() {
		linkCurrentUrl = editor?.getAttributes('link').href ?? '';
		linkModalOpen = true;
	}

	function closeLinkModal() {
		linkModalOpen = false;
		linkCurrentUrl = '';
	}

	function handleLinkConfirm(url: string) {
		if (!editor) return;
		const trimmed = url.trim();
		if (trimmed === '') {
			editor.chain().focus().extendMarkRange('link').unsetLink().run();
		} else {
			editor.chain().focus().extendMarkRange('link').setLink({ href: trimmed }).run();
		}
		closeLinkModal();
		syncMarks();
	}

	// ---- Image picker (same MediaLibraryModal as PostEditor) ----
	let mediaPickerOpen = $state(false);

	function openImagePicker() {
		mediaPickerOpen = true;
	}

	function handleMediaSelect(asset: MediaAsset) {
		mediaPickerOpen = false;
		if (editor && !editor.isDestroyed) {
			editor.chain().focus().setImage({ src: asset.url, alt: asset.alt_text ?? asset.filename ?? '' }).run();
		}
		syncMarks();
	}

	const btnBase = 'grid place-items-center rounded p-1 transition-colors';
	const btnOn = 'text-[var(--color-primary)] bg-[var(--color-primary)]/10';
	const btnOff = 'text-[var(--color-muted)] hover:text-[var(--color-text)]';
</script>

<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] {cls}">
	<!-- Toolbar -->
	<div class="flex items-center gap-0.5 border-b border-[var(--color-border)] px-1.5 py-1">
		<button type="button" onclick={toggleBold} class="{btnBase} {isBold ? btnOn : btnOff}" aria-label="Bold" title="Bold">
			<Bold size={14} />
		</button>
		<button type="button" onclick={toggleItalic} class="{btnBase} {isItalic ? btnOn : btnOff}" aria-label="Italic" title="Italic">
			<Italic size={14} />
		</button>
		<button type="button" onclick={toggleStrike} class="{btnBase} {isStrike ? btnOn : btnOff}" aria-label="Strikethrough" title="Strikethrough">
			<Strikethrough size={14} />
		</button>
		<button type="button" onclick={toggleCode} class="{btnBase} {isCode ? btnOn : btnOff}" aria-label="Inline code" title="Inline code">
			<Code size={14} />
		</button>
		<button type="button" onclick={openLinkModal} class="{btnBase} {isLink ? btnOn : btnOff}" aria-label="Link" title="Link">
			<LinkIcon size={14} />
		</button>
		<button type="button" onclick={openImagePicker} class="{btnBase} {btnOff}" aria-label="Image" title="Image">
			<ImagePlus size={14} />
		</button>
		<EmojiPicker onselect={insertEmoji} {t} />
	</div>
	<!-- Editor surface -->
	<div bind:this={element} class="tiptap-comment-editor px-3 py-2 text-sm text-[var(--color-text)]"></div>
</div>

{#if linkModalOpen}
	<EditorInsertModal
		kind="link"
		currentUrl={linkCurrentUrl}
		{t}
		onConfirm={handleLinkConfirm}
		onClose={closeLinkModal}
	/>
{/if}

{#if mediaPickerOpen}
	<MediaLibraryModal
		onSelect={handleMediaSelect}
		onClose={() => (mediaPickerOpen = false)}
	/>
{/if}

<style>
	:global(.tiptap-comment-editor .ProseMirror) {
		min-height: 2.5rem;
		outline: none;
	}
	:global(.tiptap-comment-editor .ProseMirror p) {
		margin: 0;
	}
	:global(.tiptap-comment-editor .ProseMirror img) {
		max-width: 100%;
		height: auto;
		border-radius: var(--radius);
		margin: 0.5rem 0;
	}
	:global(.tiptap-comment-editor .ProseMirror-placeholder::before) {
		color: var(--color-muted);
		content: attr(data-placeholder);
		pointer-events: none;
		float: left;
		height: 0;
	}
	:global(.tiptap-comment-editor .mention) {
		color: var(--color-primary);
		font-weight: 500;
	}
</style>
