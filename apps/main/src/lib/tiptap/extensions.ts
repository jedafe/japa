import { StarterKit } from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
import Link from '@tiptap/extension-link';
import Placeholder from '@tiptap/extension-placeholder';
import type { Extensions } from '@tiptap/core';
import type { SearchHit } from '$lib/types';

export interface ExtensionOptions {
	onSearch?: (kind: 'users' | 'content' | 'tags', query: string) => Promise<SearchHit[]>;
}

/**
 * Assemble the TipTap extension stack for the PostEditor.
 * Task 16 adds @ mention, # hashtag, and [[ interlink suggestion plugins
 * via the onSearch callback; this base includes formatting + image + link.
 */
export function buildExtensions(opts: ExtensionOptions = {}): Extensions {
	const extensions: Extensions = [
		StarterKit.configure({
			heading: { levels: [1, 2, 3] }
		}),
		Image.configure({ inline: true, allowBase64: false }),
		Link.configure({ openOnClick: false, autolink: true }),
		Placeholder.configure({ placeholder: 'Write something…' })
	];

	// Task 16 will push Mention/Hashtag/Interlink suggestion extensions here,
	// wired to opts.onSearch via the searchHits helper.

	return extensions;
}
