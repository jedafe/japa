import type { Editor } from '@tiptap/core';

/**
 * Optional context passed to slash block `run` handlers.
 * Allows the host component (PostEditor) to intercept certain blocks
 * (e.g. image) and open a rich media picker instead of window.prompt.
 */
export interface SlashBlockContext {
	/** Open the media library picker. Host inserts the selected image. */
	onInsertImage?: () => void;
	/** Open a URL input modal for YouTube. Host calls back with the URL. */
	onInsertYoutube?: () => void;
	/** Open a URL input modal for a link. Host calls back with the URL. */
	onInsertLink?: () => void;
	/** Open a coupon search modal. Host calls back with "id|label". */
	onInsertCoupon?: () => void;
}

export interface SlashBlock {
	key: string;
	labelKey: string;
	descKey: string;
	icon: string;
	run: (editor: Editor, ctx?: SlashBlockContext) => void;
}

export const SLASH_BLOCKS: SlashBlock[] = [
	{
		key: 'text',
		labelKey: 'create.slash.text',
		descKey: 'create.slash.text_desc',
		icon: 'Type',
		run: (e) => e.chain().focus().setParagraph().run()
	},
	{
		key: 'heading2',
		labelKey: 'create.slash.heading2',
		descKey: 'create.slash.heading2_desc',
		icon: 'Heading2',
		run: (e) => e.chain().focus().toggleNode('heading', 'paragraph', { level: 2 }).run()
	},
	{
		key: 'heading3',
		labelKey: 'create.slash.heading3',
		descKey: 'create.slash.heading3_desc',
		icon: 'Heading3',
		run: (e) => e.chain().focus().toggleNode('heading', 'paragraph', { level: 3 }).run()
	},
	{
		key: 'bulletList',
		labelKey: 'create.slash.bullet_list',
		descKey: 'create.slash.bullet_list_desc',
		icon: 'List',
		run: (e) => e.chain().focus().toggleBulletList().run()
	},
	{
		key: 'orderedList',
		labelKey: 'create.slash.ordered_list',
		descKey: 'create.slash.ordered_list_desc',
		icon: 'ListOrdered',
		run: (e) => e.chain().focus().toggleOrderedList().run()
	},
	{
		key: 'blockquote',
		labelKey: 'create.slash.blockquote',
		descKey: 'create.slash.blockquote_desc',
		icon: 'Quote',
		run: (e) => e.chain().focus().toggleBlockquote().run()
	},
	{
		key: 'codeBlock',
		labelKey: 'create.slash.code_block',
		descKey: 'create.slash.code_block_desc',
		icon: 'Code2',
		run: (e) => e.chain().focus().toggleCodeBlock().run()
	},
	{
		key: 'divider',
		labelKey: 'create.slash.divider',
		descKey: 'create.slash.divider_desc',
		icon: 'Minus',
		run: (e) => e.chain().focus().setHorizontalRule().run()
	},
	{
		key: 'image',
		labelKey: 'create.slash.image',
		descKey: 'create.slash.image_desc',
		icon: 'ImagePlus',
		run: (e, ctx) => {
			if (ctx?.onInsertImage) {
				ctx.onInsertImage();
				return;
			}
			const url = window.prompt('Image URL');
			if (url) e.chain().focus().setImage({ src: url }).run();
		}
	},
	{
		key: 'youtube',
		labelKey: 'create.slash.youtube',
		descKey: 'create.slash.youtube_desc',
		icon: 'Youtube',
		run: (e, ctx) => {
			if (ctx?.onInsertYoutube) {
				ctx.onInsertYoutube();
				return;
			}
			const url = window.prompt('YouTube URL');
			if (url) e.chain().focus().setYoutubeVideo({ src: url }).run();
		}
	},
	{
		key: 'link',
		labelKey: 'create.slash.link',
		descKey: 'create.slash.link_desc',
		icon: 'Link',
		run: (e, ctx) => {
			if (ctx?.onInsertLink) {
				ctx.onInsertLink();
				return;
			}
			const url = window.prompt('URL');
			if (!url) return;
			if (/^https?:\/\//i.test(url)) {
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				(e.commands as any).insertLinkPreview?.(url);
			} else {
				e.chain().focus().insertContent({
					type: 'text',
					text: url,
					marks: [{ type: 'link', attrs: { href: url } }]
				}).run();
			}
		}
	},
	{
		key: 'table',
		labelKey: 'create.slash.table',
		descKey: 'create.slash.table_desc',
		icon: 'Table',
		run: (e) => e.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
	},
	{
		key: 'coupon',
		labelKey: 'create.slash.coupon',
		descKey: 'create.slash.coupon_desc',
		icon: 'Ticket',
		run: (e, ctx) => {
			if (ctx?.onInsertCoupon) {
				ctx.onInsertCoupon();
				return;
			}
			// Without a host context, coupon insert is not available.
		}
	}
];
