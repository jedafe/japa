<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Editor } from '@tiptap/core';
	import StarterKit from '@tiptap/starter-kit';
	import Underline from '@tiptap/extension-underline';
	import Link from '@tiptap/extension-link';
	import Image from '@tiptap/extension-image';
	import Mention from '@tiptap/extension-mention';
	import Placeholder from '@tiptap/extension-placeholder';
	import CharacterCount from '@tiptap/extension-character-count';
	import { Table } from '@tiptap/extension-table';
	import { TableRow } from '@tiptap/extension-table-row';
	import { TableCell } from '@tiptap/extension-table-cell';
	import { TableHeader } from '@tiptap/extension-table-header';
	import { TaskList } from '@tiptap/extension-task-list';
	import { TaskItem } from '@tiptap/extension-task-item';
	import { CodeBlockLowlight } from '@tiptap/extension-code-block-lowlight';
	import { common, createLowlight } from 'lowlight';
	import SlashCommand from './SlashCommand.svelte';
	import MentionPopup from './MentionPopup.svelte';
	import TagPopup from './TagPopup.svelte';
	import InterlinkPopup from './InterlinkPopup.svelte';
	import { api } from '$lib/api';

	const lowlight = createLowlight(common);

	// AI assist state
	let aiLoading = $state(false);
	let aiError = $state('');

	interface Props {
		content?: string;
		onChange?: (json: string) => void;
		readOnly?: boolean;
		placeholder?: string;
		maxLength?: number;
	}

	let {
		content = '',
		onChange,
		readOnly = false,
		placeholder = 'Start writing...',
		maxLength
	}: Props = $props();

	let editorElement: HTMLDivElement;
	let editor: Editor | null = $state(null);
	let showSlashCommand = $state(false);
	let showMentionPopup = $state(false);
	let showTagPopup = $state(false);
	let showInterlinkPopup = $state(false);
	let slashCommandPos = $state({ top: 0, left: 0 });
	let mentionPos = $state({ top: 0, left: 0 });
	let tagPos = $state({ top: 0, left: 0 });
	let interlinkPos = $state({ top: 0, left: 0 });

	onMount(() => {
		editor = new Editor({
			element: editorElement,
			extensions: [
				StarterKit.configure({
					codeBlock: false,
				}),
				Underline,
				Link.configure({
					openOnClick: false,
					HTMLAttributes: {
						class: 'text-blue-600 hover:underline',
					},
				}),
				Image.configure({
					HTMLAttributes: {
						class: 'max-w-full rounded-lg',
					},
				}),
				Mention.configure({
					HTMLAttributes: {
						class: 'mention',
					},
				}),
				Placeholder.configure({
					placeholder,
				}),
				CharacterCount.configure({
					limit: maxLength,
				}),
				Table.configure({
					resizable: true,
				}),
				TableRow,
				TableCell,
				TableHeader,
				TaskList,
				TaskItem.configure({
					nested: true,
				}),
				CodeBlockLowlight.configure({
					lowlight,
				}),
			],
			content: content ? safeParseJSON(content) : undefined,
			editable: !readOnly,
			onUpdate: ({ editor }) => {
				const json = JSON.stringify(editor.getJSON());
				onChange?.(json);
			},
		});

		editorElement.addEventListener('keydown', handleKeyDown);
	});

	onDestroy(() => {
		if (editor) {
			editor.destroy();
		}
	});

	function safeParseJSON(str: string): object | undefined {
		try {
			return JSON.parse(str);
		} catch {
			return undefined;
		}
	}

	/** Returns true when the cursor is at the start of a line or after whitespace. */
	function isTriggerContext(): boolean {
		if (!editor) return false;
		const { state } = editor;
		const anchor = state.selection.$anchor;
		const textBefore = anchor.parent.textContent.slice(0, anchor.parentOffset);
		return textBefore.length === 0 || /\s$/.test(textBefore);
	}

	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			showSlashCommand = false;
			showMentionPopup = false;
			showTagPopup = false;
			showInterlinkPopup = false;
			return;
		}

		if (!isTriggerContext()) return;

		if (e.key === '/' && !showMentionPopup && !showTagPopup && !showInterlinkPopup) {
			showSlashCommand = true;
			updatePopupPosition();
		} else if (e.key === '@' && !showSlashCommand && !showTagPopup && !showInterlinkPopup) {
			showMentionPopup = true;
			updatePopupPosition('mention');
		} else if (e.key === '#' && !showSlashCommand && !showMentionPopup && !showInterlinkPopup) {
			showTagPopup = true;
			updatePopupPosition('tag');
		} else if (e.key === '[' && editor) {
			const { state } = editor;
			const anchor = state.selection.$anchor;
			const textBefore = anchor.parent.textContent.slice(0, anchor.parentOffset);
			if (textBefore.endsWith('[') && !showSlashCommand && !showMentionPopup && !showTagPopup) {
				showInterlinkPopup = true;
				updatePopupPosition('interlink');
			}
		}
	}

	function updatePopupPosition(type: 'slash' | 'mention' | 'tag' | 'interlink' = 'slash') {
		const selection = window.getSelection();
		if (selection && selection.rangeCount > 0) {
			const range = selection.getRangeAt(0);
			const rect = range.getBoundingClientRect();
			const pos = { top: rect.bottom + 8, left: rect.left };
			if (type === 'slash') slashCommandPos = pos;
			else if (type === 'mention') mentionPos = pos;
			else if (type === 'tag') tagPos = pos;
			else if (type === 'interlink') interlinkPos = pos;
		}
	}

	function insertMention(user: { id: string; label: string }) {
		if (!editor) return;
		const { state } = editor;
		const anchor = state.selection.$anchor;
		const textBefore = anchor.parent.textContent.slice(0, anchor.parentOffset);
		const atIndex = textBefore.lastIndexOf('@');
		if (atIndex === -1) { showMentionPopup = false; return; }

		const from = anchor.start() + atIndex;
		const to = anchor.pos;
		editor.chain().focus().deleteRange({ from, to }).run();
		editor.chain().focus().insertContent({
			type: 'mention',
			attrs: { id: user.id, label: user.label },
		}).run();
		showMentionPopup = false;
	}

	function insertTag(tag: { id: string; label: string }) {
		if (!editor) return;
		const { state } = editor;
		const anchor = state.selection.$anchor;
		const textBefore = anchor.parent.textContent.slice(0, anchor.parentOffset);
		const hashIndex = textBefore.lastIndexOf('#');
		if (hashIndex === -1) { showTagPopup = false; return; }

		const from = anchor.start() + hashIndex;
		const to = anchor.pos;
		editor.chain().focus().deleteRange({ from, to }).run();
		editor.chain().focus().insertContent(`#${tag.label} `).run();
		showTagPopup = false;
	}

	function insertInterlink(item: { id: string; type: string; label: string }) {
		if (!editor) return;
		const { state } = editor;
		const anchor = state.selection.$anchor;
		const textBefore = anchor.parent.textContent.slice(0, anchor.parentOffset);
		const bracketIndex = textBefore.lastIndexOf('[[');
		if (bracketIndex === -1) { showInterlinkPopup = false; return; }

		const from = anchor.start() + bracketIndex;
		const to = anchor.pos;
		editor.chain().focus().deleteRange({ from, to }).run();
		editor.chain().focus().insertContent(`[[${item.label}]]`).run();
		showInterlinkPopup = false;
	}

	async function insertSlashCommand(command: string) {
		if (!editor) return;
		
		switch (command) {
			case 'heading1':
				editor.chain().focus().toggleHeading({ level: 1 }).run();
				break;
			case 'heading2':
				editor.chain().focus().toggleHeading({ level: 2 }).run();
				break;
			case 'heading3':
				editor.chain().focus().toggleHeading({ level: 3 }).run();
				break;
			case 'bulletList':
				editor.chain().focus().toggleBulletList().run();
				break;
			case 'orderedList':
				editor.chain().focus().toggleOrderedList().run();
				break;
			case 'blockquote':
				editor.chain().focus().toggleBlockquote().run();
				break;
			case 'codeBlock':
				editor.chain().focus().toggleCodeBlock().run();
				break;
			case 'horizontalRule':
				editor.chain().focus().setHorizontalRule().run();
				break;

			// ── AI assist commands (AUDIT-FE wiring) ─────────────────────────
			case 'ai:improve':
			case 'ai:summarize':
			case 'ai:translate_fr': {
				const content = editor.getText();
				if (!content.trim()) break;
				aiLoading = true;
				aiError = '';
				try {
					let result = '';
					if (command === 'ai:improve') {
						const r = await api.aiImprove(content);
						result = r.result;
					} else if (command === 'ai:summarize') {
						const r = await api.aiSummarize(content);
						result = r.result;
					} else {
						const r = await api.aiTranslate(content, 'fr');
						result = r.result;
					}
					if (result) {
						editor.chain().focus().clearContent().insertContent(result).run();
					}
				} catch {
					aiError = 'AI assist failed — please try again.';
				} finally {
					aiLoading = false;
				}
				break;
			}
		}
		
		showSlashCommand = false;
	}
</script>

<div class="editor-container">
	<div bind:this={editorElement} class="editor-content prose prose-sm max-w-none dark:prose-invert"></div>
	
	{#if showSlashCommand}
		<SlashCommand
			position={slashCommandPos}
			onSelect={insertSlashCommand}
			onClose={() => showSlashCommand = false}
		/>
	{/if}
	
	{#if showMentionPopup}
		<MentionPopup
			position={mentionPos}
			onSelect={insertMention}
			onClose={() => showMentionPopup = false}
		/>
	{/if}
	
	{#if showTagPopup}
		<TagPopup
			position={tagPos}
			onSelect={insertTag}
			onClose={() => showTagPopup = false}
		/>
	{/if}
	
	{#if showInterlinkPopup}
		<InterlinkPopup
			position={interlinkPos}
			onSelect={insertInterlink}
			onClose={() => showInterlinkPopup = false}
		/>
	{/if}
</div>

<style>
	.editor-container {
		position: relative;
		border: 1px solid var(--color-muted);
		border-radius: 0.5rem;
		background: var(--color-background);
	}
	
	.editor-content {
		min-height: 200px;
		padding: 1rem;
	}
	
	.editor-content :global(.ProseMirror) {
		outline: none;
	}
	
	.editor-content :global(.ProseMirror p.is-editor-empty:first-child::before) {
		content: attr(data-placeholder);
		float: left;
		color: var(--color-muted);
		pointer-events: none;
		height: 0;
	}
	
	.editor-content :global(.mention) {
		background: color-mix(in srgb, var(--color-primary) 20%, transparent);
		color: var(--color-primary);
		padding: 0.125rem 0.25rem;
		border-radius: 0.25rem;
		font-weight: 500;
	}
</style>
