import type { Editor } from '@tiptap/core';

export interface SlashBlock {
	key: string;
	labelKey: string;
	run: (editor: Editor) => void;
}

export const SLASH_BLOCKS: SlashBlock[] = [
	{
		key: 'heading1',
		labelKey: 'create.slash.heading1',
		run: (e) => e.chain().focus().toggleNode('heading', 'paragraph', { level: 1 }).run()
	},
	{
		key: 'heading2',
		labelKey: 'create.slash.heading2',
		run: (e) => e.chain().focus().toggleNode('heading', 'paragraph', { level: 2 }).run()
	},
	{
		key: 'bulletList',
		labelKey: 'create.slash.bullet_list',
		run: (e) => e.chain().focus().toggleBulletList().run()
	},
	{
		key: 'orderedList',
		labelKey: 'create.slash.ordered_list',
		run: (e) => e.chain().focus().toggleOrderedList().run()
	},
	{
		key: 'blockquote',
		labelKey: 'create.slash.blockquote',
		run: (e) => e.chain().focus().toggleBlockquote().run()
	},
	{
		key: 'codeBlock',
		labelKey: 'create.slash.code_block',
		run: (e) => e.chain().focus().toggleCodeBlock().run()
	},
	{
		key: 'image',
		labelKey: 'create.slash.image',
		run: (e) => {
			const url = window.prompt('Image URL');
			if (url) e.chain().focus().setImage({ src: url }).run();
		}
	}
];
