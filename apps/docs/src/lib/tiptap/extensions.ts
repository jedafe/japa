import StarterKit from '@tiptap/starter-kit';
import Link from '@tiptap/extension-link';
import Image from '@tiptap/extension-image';
import Youtube from '@tiptap/extension-youtube';
import { Table } from '@tiptap/extension-table';
import TableRow from '@tiptap/extension-table-row';
import TableCell from '@tiptap/extension-table-cell';
import TableHeader from '@tiptap/extension-table-header';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';
import { LinkPreviewNode, VideoNode } from '@japaconnect/ui';

/**
 * TipTap extensions shared between the server-side prose renderer
 * (lib/tiptap/serialize.ts) and the in-app editor (DocEditor.svelte →
 * @japaconnect/ui RichTextEditor). Kept identical so what the editor
 * produces renders 1:1 (§44).
 *
 * This stack covers every node the shared editor can emit on this surface
 * (StarterKit formatting, links, images, YouTube embeds, tables) plus the
 * optional feature nodes (linkPreview/video) so documents containing them
 * — e.g. written through the API — still render instead of throwing.
 */
export const docExtensions = [
	// TipTap v3 StarterKit bundles Link — disable it so the single
	// explicitly-configured instance below is the only one registered.
	StarterKit.configure({ link: false }),
	// openOnClick only affects the editor surface (clicks place the caret);
	// harmless for the server-side generateHTML renderer.
	Link.configure({ openOnClick: false, autolink: true }),
	Image.configure({ inline: true, allowBase64: false }),
	Youtube.configure({
		HTMLAttributes: { class: 'youtube-embed' },
		allowFullscreen: true,
		nocookie: true
	}),
	Table,
	TableRow,
	TableCell,
	TableHeader,
	TaskList,
	TaskItem.configure({ nested: true }),
	LinkPreviewNode,
	VideoNode
];
