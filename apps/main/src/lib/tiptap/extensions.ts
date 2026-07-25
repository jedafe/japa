import { StarterKit } from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
import Link from '@tiptap/extension-link';
import Placeholder from '@tiptap/extension-placeholder';
import Mention from '@tiptap/extension-mention';
import type { Extensions } from '@tiptap/core';
import type { SearchHit } from '$lib/types';
import { searchHits } from '$lib/api/create';
import { createSuggestionRenderer } from './suggestionRenderer';

export interface ExtensionOptions {
	onSearch?: (kind: 'users' | 'content' | 'tags', query: string) => Promise<SearchHit[]>;
}

/**
 * Assemble the TipTap extension stack for the PostEditor.
 * Includes StarterKit (formatting), Image, Link, Placeholder, and three
 * suggestion-trigger extensions: @ mention, # hashtag, [[ interlink.
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
		StarterKit.configure({ heading: { levels: [1, 2, 3] } }),
		Image.configure({ inline: true, allowBase64: false }),
		Link.configure({ openOnClick: false, autolink: true }),
		Placeholder.configure({ placeholder: 'Write something…' }),
		mentionExt,
		hashtagExt,
		interlinkExt
	];
}
