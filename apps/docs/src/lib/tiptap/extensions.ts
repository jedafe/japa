import StarterKit from '@tiptap/starter-kit';
import Link from '@tiptap/extension-link';
import { Table } from '@tiptap/extension-table';
import TableRow from '@tiptap/extension-table-row';
import TableCell from '@tiptap/extension-table-cell';
import TableHeader from '@tiptap/extension-table-header';

/**
 * TipTap extensions shared between the server-side prose renderer
 * (lib/tiptap/serialize.ts) and the in-app editor (DocEditor.svelte).
 * Kept identical so what the editor produces renders 1:1 (§44).
 */
export const docExtensions = [
	// TipTap v3 StarterKit bundles Link — disable it so the single
	// explicitly-configured instance below is the only one registered.
	StarterKit.configure({ link: false }),
	// openOnClick only affects the editor surface (clicks place the caret);
	// harmless for the server-side generateHTML renderer.
	Link.configure({ openOnClick: false, autolink: true }),
	Table,
	TableRow,
	TableCell,
	TableHeader
];
