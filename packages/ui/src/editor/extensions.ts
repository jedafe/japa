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
import type { EditorSearchHit } from './types';
import { createSuggestionRenderer } from './suggestionRenderer';
import { LinkPreviewNode } from './linkPreviewNode';
import { VideoNode } from './nodes/VideoNode';

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
			label: { default: null }
		};
	},
	parseHTML() {
		return [{ tag: 'div[data-type="coupon-embed"]' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', HTMLAttributes, 0];
	}
});

export interface EditorExtensionOptions {
	/** Placeholder text shown in an empty editor. */
	placeholder?: string;
	/**
	 * Suggestion-search provider for the trigger-char pickers.
	 * When absent, all four suggestion extensions are omitted entirely —
	 * the schema then rejects mention/hashtag/interlink nodes on paste,
	 * which is the correct degradation for surfaces without those APIs.
	 */
	onSearch?: (
		kind: 'users' | 'content' | 'tags' | 'literals',
		query: string
	) => Promise<EditorSearchHit[]>;
	/** Suggestion popup copy. */
	startTypingText?: string;
	noResultsText?: string;
	/** Feature nodes — off by default. */
	features?: {
		coupon?: boolean;
		linkPreview?: boolean;
		video?: boolean;
		image?: boolean;
		youtube?: boolean;
		markdown?: boolean;
	};
}

// Unique plugin keys per suggestion trigger — without these, multiple
// Mention-based extensions (mention, hashtag, interlink, literal) share the
// same plugin key and can conflict.
const mentionPluginKey = new PluginKey('mention');
const hashtagPluginKey = new PluginKey('hashtag');
const interlinkPluginKey = new PluginKey('interlink');
const literalPluginKey = new PluginKey('literal');

// Shared renderer config for the trigger-char pickers (@, #, [[).
// minQueryLength must mirror the suggestion's option so the popup shows
// "Start typing…" (not "No results") while the query is still too short
// to have triggered a fetch.
const TRIGGER_MIN_QUERY = 2;

/**
 * Assemble the TipTap extension stack for the shared RichTextEditor.
 * Core: StarterKit (formatting), Image, Link, Placeholder, Table, YouTube,
 * Markdown. Optional via onSearch: @ mention, # hashtag, [[ interlink,
 * {{ literal. Feature nodes (coupon/linkPreview/video) are opt-in.
 */
export function buildEditorExtensions(opts: EditorExtensionOptions = {}): Extensions {
	const extensions: Extensions = [
		// StarterKit v3 ships with Link — disable it here so our separately
		// configured Link below doesn't trigger a "Duplicate extension names"
		// console warning.
		StarterKit.configure({ heading: { levels: [2, 3] }, link: false }),
		Link.configure({ openOnClick: false, autolink: true }),
		Placeholder.configure({ placeholder: opts.placeholder ?? 'Write something…' }),
		Table.configure({ resizable: true }),
		TableRow,
		TableCell,
		TableHeader
	];

	if (opts.features?.image !== false) {
		extensions.push(Image.configure({ inline: true, allowBase64: false }));
	}
	if (opts.features?.youtube !== false) {
		extensions.push(
			Youtube.configure({
				HTMLAttributes: { class: 'youtube-embed' },
				addPasteHandler: true,
				allowFullscreen: true,
				nocookie: true
			})
		);
	}
	if (opts.features?.video) {
		// §106 Video content type — canonical video_sources embed node
		// inserted by the composer's VideoField (import URL / upload).
		extensions.push(VideoNode);
	}

	const search = opts.onSearch;
	if (search) {
		const triggerRenderer = (entityType: string) =>
			createSuggestionRenderer({
				entityType,
				minQueryLength: TRIGGER_MIN_QUERY,
				startTypingText: opts.startTypingText ?? 'Start typing…',
				noResultsText: opts.noResultsText ?? 'No results found'
			});

		extensions.push(
			Mention.configure({
				HTMLAttributes: { class: 'mention' },
				suggestion: {
					char: '@',
					allowSpaces: true,
					pluginKey: mentionPluginKey,
					minQueryLength: TRIGGER_MIN_QUERY,
					items: ({ query }) => search('users', query),
					render: () => triggerRenderer('user')
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
							parseHTML: (el) =>
								el.getAttribute('data-id') || el.getAttribute('data-user-id'),
							renderHTML: (attrs) => (attrs.id ? { 'data-id': attrs.id } : {})
						},
						label: {
							default: null,
							parseHTML: (el) =>
								el.getAttribute('data-label') ||
								el.textContent?.replace(/^@/, '') ||
								null,
							renderHTML: (attrs) => (attrs.label ? { 'data-label': attrs.label } : {})
						},
						handle: {
							default: null,
							parseHTML: (el) => el.getAttribute('data-handle'),
							renderHTML: (attrs) =>
								attrs.handle ? { 'data-handle': attrs.handle } : {}
						},
						entity_type: {
							default: 'user',
							parseHTML: (el) => el.getAttribute('data-entity-type') || 'user',
							renderHTML: (attrs) =>
								attrs.entity_type ? { 'data-entity-type': attrs.entity_type } : {}
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
						['strong', {}, `@${label}`]
					];
				}
			}),
			Mention.configure({
				HTMLAttributes: { class: 'hashtag' },
				// Reference nodes render bold; options.HTMLAttributes already
				// carries data-type/data-id/data-label/data-url merged by the
				// Mention wrapper.
				renderHTML: ({ options, node }) => [
					'span',
					options.HTMLAttributes,
					['strong', {}, `#${node.attrs.label ?? node.attrs.id}`]
				],
				suggestion: {
					char: '#',
					pluginKey: hashtagPluginKey,
					minQueryLength: TRIGGER_MIN_QUERY,
					items: ({ query }) => search('tags', query),
					render: () => triggerRenderer('tag')
				}
			}).extend({
				name: 'hashtag',
				addAttributes() {
					return {
						...this.parent?.(),
						// §15: the tag_link node links to /tags/{slug} at render time —
						// the public path is stored as data-url on the rendered span.
						url: {
							default: null,
							parseHTML: (el: HTMLElement) => el.getAttribute('data-url'),
							renderHTML: (attrs: Record<string, unknown>) =>
								attrs.url ? { 'data-url': attrs.url } : {}
						}
					};
				}
			}),
			Mention.configure({
				HTMLAttributes: { class: 'interlink' },
				// [[ is the trigger only — the rendered output is the bare label
				// (bold), not wiki-bracket syntax. options.HTMLAttributes already
				// carries data-type/data-id/data-label/data-url merged by the
				// Mention node's renderHTML wrapper.
				renderText: ({ node }) => `${node.attrs.label ?? node.attrs.id}`,
				renderHTML: ({ options, node }) => [
					'span',
					options.HTMLAttributes,
					['strong', {}, `${node.attrs.label ?? node.attrs.id}`]
				],
				suggestion: {
					char: '[[',
					allowSpaces: true,
					pluginKey: interlinkPluginKey,
					minQueryLength: TRIGGER_MIN_QUERY,
					items: ({ query }) => search('content', query),
					render: () => triggerRenderer('content')
				}
			}).extend({
				name: 'interlink',
				addAttributes() {
					return {
						...this.parent?.(),
						// §15: the content_link node links to the referenced content
						// at render time — the public path is stored as data-url.
						url: {
							default: null,
							parseHTML: (el: HTMLElement) => el.getAttribute('data-url'),
							renderHTML: (attrs: Record<string, unknown>) =>
								attrs.url ? { 'data-url': attrs.url } : {}
						}
					};
				}
			}),
			// {{ template-literal token picker (§14 — global template literals).
			// The picker lists active platform tokens and inserts the raw
			// `{{Key}}` placeholder as plain text — the backend resolver
			// expands it at render time, so no node is stored in the doc.
			Mention.configure({
				suggestion: {
					char: '{{',
					allowSpaces: true,
					pluginKey: literalPluginKey,
					// The picker opens on `{{` and lists all tokens before the user
					// types — the literals endpoint serves the full active list.
					minQueryLength: 0,
					// Strip trailing `}` — a user typing `{{foo}}` would otherwise
					// filter by `foo}}` and get zero matches.
					items: ({ query }) =>
						search('literals', query.replace(/\}+$/, '').trim()),
					render: () => createSuggestionRenderer({ entityType: 'literal' }),
					// The renderer passes {id, entity_type, label} where id is the
					// token key. Replace the `{{query` text with the literal
					// placeholder.
					command: ({ editor, range, props }) => {
						const key = (props as { id?: string }).id ?? '';
						if (!key) return;
						editor.chain().focus().insertContentAt(range, `{{${key}}} `).run();
					}
				}
			}).extend({ name: 'literal' })
		);
	}

	if (opts.features?.coupon) {
		// Coupon embed — block-level node for inserting coupon widgets
		// (Section 55). Stores coupon_id; backend resolves at render time.
		extensions.push(CouponEmbed);
	}
	if (opts.features?.linkPreview) {
		// Link preview — block-level node for inline link preview cards
		// (Section 15 / ROADMAP-MAIN 07.09). Stores URL; hydrated into a
		// LinkPreviewCard component by the editor and view page.
		extensions.push(LinkPreviewNode);
	}

	if (opts.features?.markdown !== false) {
		// Bidirectional markdown support — adds editor.getMarkdown() and
		// allows setContent(..., { contentType: 'markdown' }) for the
		// Markdown editor mode round-trip.
		extensions.push(Markdown);
	}

	return extensions;
}
