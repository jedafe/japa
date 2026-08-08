import { StarterKit } from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
import Link from '@tiptap/extension-link';
import Placeholder from '@tiptap/extension-placeholder';
import Mention from '@tiptap/extension-mention';
import { Table } from '@tiptap/extension-table';
import { TableRow } from '@tiptap/extension-table-row';
import { TableCell } from '@tiptap/extension-table-cell';
import { TableHeader } from '@tiptap/extension-table-header';
import Youtube from '@tiptap/extension-youtube';
import { Markdown } from '@tiptap/markdown';
import { PluginKey } from '@tiptap/pm/state';
import { Node, mergeAttributes, type Extensions } from '@tiptap/core';
import type { SearchHit } from '$lib/types';
import { searchHits } from '$lib/api/create';
import { createSuggestionRenderer } from './suggestionRenderer';
import { LinkPreviewNode } from './linkPreviewNode';

/**
 * Coupon embed node (Section 55 — render-time coupon/affiliate URL
 * resolution). Stores the coupon ID; the Go backend resolves the
 * current code and tracking URL at render time (P4). This is a
 * block-level node inserted via the slash menu, not a trigger-char
 * suggestion.
 */
const CouponEmbed = Node.create({
	name: 'couponEmbed',
	group: 'block',
	content: 'inline*',
	addAttributes() {
		return {
			coupon_id: { default: null },
			label: { default: null },
		};
	},
	parseHTML() {
		return [{ tag: 'div[data-type="coupon-embed"]' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', HTMLAttributes, 0];
	}
});

export interface ExtensionOptions {
	onSearch?: (kind: 'users' | 'content' | 'tags', query: string) => Promise<SearchHit[]>;
	placeholder?: string;
}

// Unique plugin keys per suggestion trigger — without these, multiple
// Mention-based extensions (mention, hashtag, interlink) share the same
// plugin key and can conflict.
const mentionPluginKey = new PluginKey('mention');
const hashtagPluginKey = new PluginKey('hashtag');
const interlinkPluginKey = new PluginKey('interlink');

/**
 * Assemble the TipTap extension stack for the PostEditor.
 * Includes StarterKit (formatting), Image, Link, Placeholder,
 * Table, YouTube, and three suggestion-trigger extensions: @ mention,
 * # hashtag, [[ interlink.
 */
export function buildExtensions(opts: ExtensionOptions = {}): Extensions {
	const search = opts.onSearch ?? searchHits;

	const mentionExt = Mention.configure({
		HTMLAttributes: { class: 'mention' },
		suggestion: {
			char: '@',
			allowSpaces: true,
			pluginKey: mentionPluginKey,
			minQueryLength: 2,
			items: ({ query }) => search('users', query),
			render: () => createSuggestionRenderer({ entityType: 'user' })
		}
	}).extend({
		name: 'mention',
		// Add 'handle' (username) and 'entity_type' attributes so the
		// mention node carries enough data for the view-page hydrate
		// action to wrap it in a profile link (<a href="/u/{handle}">).
		// The editor renders just the <span>; the link is added outside
		// the editor to avoid conflicts with the Link mark extension.
		addAttributes() {
			return {
				...this.parent?.(),
				id: {
					default: null,
					parseHTML: (el) => el.getAttribute('data-id') || el.getAttribute('data-user-id'),
					renderHTML: (attrs) => (attrs.id ? { 'data-id': attrs.id } : {})
				},
				label: {
					default: null,
					parseHTML: (el) => el.getAttribute('data-label') || el.textContent?.replace(/^@/, '') || null,
					renderHTML: (attrs) => (attrs.label ? { 'data-label': attrs.label } : {})
				},
				handle: {
					default: null,
					parseHTML: (el) => el.getAttribute('data-handle'),
					renderHTML: (attrs) => (attrs.handle ? { 'data-handle': attrs.handle } : {})
				},
				entity_type: {
					default: 'user',
					parseHTML: (el) => el.getAttribute('data-entity-type') || 'user',
					renderHTML: (attrs) => (attrs.entity_type ? { 'data-entity-type': attrs.entity_type } : {})
				}
			};
		},
		renderHTML({ node, HTMLAttributes }) {
			const label = node.attrs?.label ?? '';
			return [
				'span',
				mergeAttributes(HTMLAttributes, {
					class: 'mention',
					'data-type': 'mention',
					'data-id': node.attrs?.id ?? '',
					'data-label': label,
					'data-handle': node.attrs?.handle ?? ''
				}),
				`@${label}`
			];
		}
	});

	const hashtagExt = Mention.configure({
		HTMLAttributes: { class: 'hashtag' },
		suggestion: {
			char: '#',
			pluginKey: hashtagPluginKey,
			minQueryLength: 2,
			items: ({ query }) => search('tags', query),
			render: () => createSuggestionRenderer({ entityType: 'tag' })
		}
	}).extend({ name: 'hashtag' });

	const interlinkExt = Mention.configure({
		HTMLAttributes: { class: 'interlink' },
		suggestion: {
			char: '[[',
			allowSpaces: true,
			pluginKey: interlinkPluginKey,
			minQueryLength: 2,
			items: ({ query }) => search('content', query),
			render: () => createSuggestionRenderer({ entityType: 'content' })
		}
	}).extend({ name: 'interlink' });

	return [
		// StarterKit v3 ships with Link — disable it here so our separately
		// configured Link below doesn't trigger a "Duplicate extension names"
		// console warning.
		StarterKit.configure({ heading: { levels: [2, 3] }, link: false }),
		Image.configure({ inline: true, allowBase64: false }),
		Link.configure({ openOnClick: false, autolink: true }),
		Placeholder.configure({ placeholder: opts.placeholder ?? 'Write something…' }),
		Table.configure({ resizable: true }),
		TableRow,
		TableCell,
		TableHeader,
		Youtube.configure({
			HTMLAttributes: { class: 'youtube-embed' },
			addPasteHandler: true,
			allowFullscreen: true,
			nocookie: true
		}),
		mentionExt,
		hashtagExt,
		interlinkExt,
		// Coupon embed — block-level node for inserting coupon widgets
		// (Section 55). Stores coupon_id; backend resolves at render time.
		CouponEmbed,
		// Link preview — block-level node for inline link preview cards
		// (Section 15 / ROADMAP-MAIN 07.09). Stores URL; hydrated into a
		// LinkPreviewCard component by the editor and view page.
		LinkPreviewNode,
		// Bidirectional markdown support — adds editor.getMarkdown() and
		// allows setContent(..., { contentType: 'markdown' }) for the
		// Markdown editor mode round-trip.
		Markdown
	];
}
