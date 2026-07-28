import type { Editor } from '@tiptap/core';

export interface SlashBlock {
	key: string;
	labelKey: string;
	descKey: string;
	icon: string;
	run: (editor: Editor) => void;
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
		key: 'heading1',
		labelKey: 'create.slash.heading1',
		descKey: 'create.slash.heading1_desc',
		icon: 'Heading1',
		run: (e) => e.chain().focus().toggleNode('heading', 'paragraph', { level: 1 }).run()
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
		key: 'taskList',
		labelKey: 'create.slash.task_list',
		descKey: 'create.slash.task_list_desc',
		icon: 'ListChecks',
		run: (e) => e.chain().focus().toggleTaskList().run()
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
		run: (e) => {
			const url = window.prompt('Image URL');
			if (url) e.chain().focus().setImage({ src: url }).run();
		}
	},
	{
		key: 'table',
		labelKey: 'create.slash.table',
		descKey: 'create.slash.table_desc',
		icon: 'Table',
		run: (e) => e.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
	}
];
