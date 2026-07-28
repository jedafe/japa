import { StarterKit } from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
import Link from '@tiptap/extension-link';
import Placeholder from '@tiptap/extension-placeholder';
import Mention from '@tiptap/extension-mention';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';
import { Table } from '@tiptap/extension-table';
import { TableRow } from '@tiptap/extension-table-row';
import { TableCell } from '@tiptap/extension-table-cell';
import { TableHeader } from '@tiptap/extension-table-header';
import { Markdown } from '@tiptap/markdown';
import type { Extensions } from '@tiptap/core';
import type { SearchHit } from '$lib/types';
import { searchHits } from '$lib/api/create';
import { createSuggestionRenderer } from './suggestionRenderer';

export interface ExtensionOptions {
	onSearch?: (kind: 'users' | 'content' | 'tags', query: string) => Promise<SearchHit[]>;
	placeholder?: string;
}

/**
 * Assemble the TipTap extension stack for the PostEditor.
 * Includes StarterKit (formatting), Image, Link, Placeholder, TaskList,
 * Table, and three suggestion-trigger extensions: @ mention, # hashtag, [[ interlink.
 */
export function buildExtensions(opts: ExtensionOptions = {}): Extensions {
	const search = opts.onSearch ?? searchHits;

	const mentionExt = Mention.configure({
		suggestion: {
			char: '@',
			allowSpaces: true,
			render: () => createSuggestionRenderer({ onSearch: (q) => search('users', q) })
		}
	}).extend({ name: 'mention' });

	const hashtagExt = Mention.configure({
		HTMLAttributes: { class: 'hashtag' },
		suggestion: {
			char: '#',
			render: () => createSuggestionRenderer({ onSearch: (q) => search('tags', q) })
		}
	}).extend({ name: 'hashtag' });

	const interlinkExt = Mention.configure({
		HTMLAttributes: { class: 'interlink' },
		suggestion: {
			char: '[',
			allowSpaces: true,
			render: () => createSuggestionRenderer({ onSearch: (q) => search('content', q) })
		}
	}).extend({ name: 'interlink' });

	return [
		// StarterKit v3 ships with Link — disable it here so our separately
		// configured Link below doesn't trigger a "Duplicate extension names"
		// console warning.
		StarterKit.configure({ heading: { levels: [1, 2, 3] }, link: false }),
		Image.configure({ inline: true, allowBase64: false }),
		Link.configure({ openOnClick: false, autolink: true }),
		Placeholder.configure({ placeholder: opts.placeholder ?? 'Write something…' }),
		TaskList,
		TaskItem.configure({ nested: true }),
		Table.configure({ resizable: true }),
		TableRow,
		TableCell,
		TableHeader,
		mentionExt,
		hashtagExt,
		interlinkExt,
		// Bidirectional markdown support — adds editor.getMarkdown() and
		// allows setContent(..., { contentType: 'markdown' }) for the
		// Markdown editor mode round-trip.
		Markdown
	];
}
