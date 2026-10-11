import type { Editor } from '@tiptap/core';

/** Guard a URL for safe use in href/src — same policy as the apps'
 *  safeHref helpers (http/https/mailto/tel/relative only). */
function safeHref(url: string | undefined | null): string {
	if (!url || typeof url !== 'string') return '';
	const trimmed = url.trim();
	if (!trimmed) return '';
	if (trimmed.startsWith('//')) return '';
	if (/^(javascript|data|vbscript):/i.test(trimmed)) return '';
	return trimmed;
}

/**
 * Optional context passed to slash block `run` handlers.
 * Allows the host component to intercept certain blocks (e.g. image)
 * and open a rich picker instead of a URL prompt.
 */
export interface SlashBlockContext {
	/** Open the host's image picker; fallback is a URL prompt modal. */
	onInsertImage?: () => void;
	/** Open a URL input modal for YouTube. */
	onInsertYoutube?: () => void;
	/** Open a URL input modal for a link. */
	onInsertLink?: () => void;
	/** Open a coupon search modal. Host calls back with "id|label". */
	onInsertCoupon?: () => void;
}

export interface SlashBlock {
	key: string;
	/** i18n key the host's t() resolves; `label`/`desc` are the fallbacks. */
	labelKey: string;
	descKey: string;
	label: string;
	desc: string;
	icon: string;
	run: (editor: Editor, ctx?: SlashBlockContext) => void;
}

export const SLASH_BLOCKS: SlashBlock[] = [
	{
		key: 'text',
		labelKey: 'create.slash.text',
		descKey: 'create.slash.text_desc',
		label: 'Text',
		desc: 'Plain paragraph',
		icon: 'Type',
		run: (e) => e.chain().focus().setParagraph().run()
	},
	{
		key: 'heading2',
		labelKey: 'create.slash.heading2',
		descKey: 'create.slash.heading2_desc',
		label: 'Heading 2',
		desc: 'Large section heading',
		icon: 'Heading2',
		run: (e) =>
			e.chain().focus().toggleNode('heading', 'paragraph', { level: 2 }).run()
	},
	{
		key: 'heading3',
		labelKey: 'create.slash.heading3',
		descKey: 'create.slash.heading3_desc',
		label: 'Heading 3',
		desc: 'Smaller section heading',
		icon: 'Heading3',
		run: (e) =>
			e.chain().focus().toggleNode('heading', 'paragraph', { level: 3 }).run()
	},
	{
		key: 'bulletList',
		labelKey: 'create.slash.bullet_list',
		descKey: 'create.slash.bullet_list_desc',
		label: 'Bullet list',
		desc: 'Unordered list',
		icon: 'List',
		run: (e) => e.chain().focus().toggleBulletList().run()
	},
	{
		key: 'orderedList',
		labelKey: 'create.slash.ordered_list',
		descKey: 'create.slash.ordered_list_desc',
		label: 'Ordered list',
		desc: 'Numbered list',
		icon: 'ListOrdered',
		run: (e) => e.chain().focus().toggleOrderedList().run()
	},
	{
		key: 'blockquote',
		labelKey: 'create.slash.blockquote',
		descKey: 'create.slash.blockquote_desc',
		label: 'Quote',
		desc: 'Blockquote callout',
		icon: 'Quote',
		run: (e) => e.chain().focus().toggleBlockquote().run()
	},
	{
		key: 'codeBlock',
		labelKey: 'create.slash.code_block',
		descKey: 'create.slash.code_block_desc',
		label: 'Code block',
		desc: 'Syntax-highlighted code',
		icon: 'Code2',
		run: (e) => e.chain().focus().toggleCodeBlock().run()
	},
	{
		key: 'divider',
		labelKey: 'create.slash.divider',
		descKey: 'create.slash.divider_desc',
		label: 'Divider',
		desc: 'Horizontal rule',
		icon: 'Minus',
		run: (e) => e.chain().focus().setHorizontalRule().run()
	},
	{
		key: 'image',
		labelKey: 'create.slash.image',
		descKey: 'create.slash.image_desc',
		label: 'Image',
		desc: 'Insert an image',
		icon: 'ImagePlus',
		run: (e, ctx) => {
			if (ctx?.onInsertImage) {
				ctx.onInsertImage();
				return;
			}
			const raw = window.prompt('Image URL');
			const url = raw ? safeHref(raw) : '';
			if (url) e.chain().focus().setImage({ src: url }).run();
		}
	},
	{
		key: 'youtube',
		labelKey: 'create.slash.youtube',
		descKey: 'create.slash.youtube_desc',
		label: 'YouTube',
		desc: 'Embed a video',
		icon: 'Youtube',
		run: (e, ctx) => {
			if (ctx?.onInsertYoutube) {
				ctx.onInsertYoutube();
				return;
			}
			const raw = window.prompt('YouTube URL');
			const url = raw ? safeHref(raw) : '';
			// TipTap's setYoutubeVideo accepts youtu.be or youtube.com URLs.
			if (url && /^(https?:\/\/)?(www\.)?(youtube\.com|youtu\.be)\//i.test(url)) {
				e.chain().focus().setYoutubeVideo({ src: url }).run();
			}
		}
	},
	{
		key: 'link',
		labelKey: 'create.slash.link',
		descKey: 'create.slash.link_desc',
		label: 'Link',
		desc: 'Link or preview card',
		icon: 'Link',
		run: (e, ctx) => {
			if (ctx?.onInsertLink) {
				ctx.onInsertLink();
				return;
			}
			const raw = window.prompt('URL');
			const url = raw ? safeHref(raw) : '';
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
		label: 'Table',
		desc: 'Insert a table',
		icon: 'Table',
		run: (e) =>
			e.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
	},
	{
		key: 'coupon',
		labelKey: 'create.slash.coupon',
		descKey: 'create.slash.coupon_desc',
		label: 'Coupon',
		desc: 'Embed a coupon card',
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
