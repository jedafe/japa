import { Node } from '@tiptap/core';

/**
 * LinkPreview — a block-level TipTap node that stores a URL and renders a
 * placeholder div. The PostEditor hydrates the placeholder into a live
 * LinkPreviewCard Svelte component after each editor update. The saved
 * HTML contains the placeholder div with the URL, so the view page can
 * hydrate it the same way.
 *
 * Markdown round-trip: the `renderMarkdown` method emits the placeholder
 * div as raw HTML. When the markdown is parsed back (via `@tiptap/markdown`'s
 * `parseHTMLToken` → `generateJSON`), the `parseHTML` rule reconstructs the
 * `linkPreview` node from the `div[data-type="link-preview"]` tag.
 *
 * Ref: japaconnect-engineering/sections/15-block-editor-tiptap.md
 *      japaconnect-engineering/roadmaps/ROADMAP-MAIN.md (task 07.09)
 */
export const LinkPreviewNode = Node.create({
	name: 'linkPreview',
	group: 'block',
	atom: true,
	draggable: true,

	addAttributes() {
		return {
			url: {
				default: null,
				renderHTML: (attrs: { url: string }) => ({ 'data-url': attrs.url }),
				parseHTML: (el: HTMLElement) => el.getAttribute('data-url'),
			},
		};
	},

	parseHTML() {
		return [{ tag: 'div[data-type="link-preview"]' }];
	},

	renderHTML({ HTMLAttributes }) {
		return ['div', { 'data-type': 'link-preview', ...HTMLAttributes }];
	},

	/**
	 * Serialize to markdown as a raw HTML block. The `@tiptap/markdown`
	 * extension's parser will use `parseHTML` to reconstruct the node
	 * when switching back from markdown mode.
	 */
	renderMarkdown(node: { attrs?: { url?: string } }) {
		const url = node.attrs?.url ?? '';
		const escaped = url.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		return `<div data-type="link-preview" data-url="${escaped}"></div>`;
	},

	addCommands() {
		return {
			insertLinkPreview:
				(url: string) =>
				({ commands }: { commands: any }) => {
					return commands.insertContent({
						type: 'linkPreview',
						attrs: { url },
					});
				},
		};
	},
});

declare module '@tiptap/core' {
	interface Commands<ReturnType> {
		linkPreview: {
			insertLinkPreview: (url: string) => ReturnType;
		};
	}
}
