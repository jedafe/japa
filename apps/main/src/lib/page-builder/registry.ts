/**
 * Client-side block type registry for the Page Builder.
 *
 * Mirrors the backend's BlockTemplateRegistry so the editor can
 * instantiate new blocks without a round-trip. The backend remains
 * the source of truth — this registry is a local cache for instant
 * block creation in the editor UI.
 */

import type { BlockDefinition, BlockInstance, BlockCategory } from './types';

const BLOCK_TYPES: BlockDefinition[] = [
	{
		type: 'hero',
		name: 'Hero Section',
		description: 'Hero section with title, subtitle, background image, and call-to-action',
		icon: 'layout',
		default_props: {
			title: '',
			subtitle: '',
			background_image: '',
			cta_text: '',
			cta_url: '',
			overlay_opacity: 0.4,
		},
		category: 'layout',
	},
	{
		type: 'rich_text',
		name: 'Rich Text',
		description: 'Rich text content powered by TipTap editor',
		icon: 'type',
		default_props: {
			content: '',
		},
		category: 'content',
	},
	{
		type: 'image',
		name: 'Image / Gallery',
		description: 'Single image or image gallery',
		icon: 'image',
		default_props: {
			images: [],
			caption: '',
			layout: 'single',
		},
		category: 'media',
	},
	{
		type: 'cta',
		name: 'Call to Action',
		description: 'Call-to-action button or button group',
		icon: 'mouse-pointer-click',
		default_props: {
			text: '',
			url: '',
			style: 'primary',
			variant: 'filled',
		},
		category: 'interactive',
	},
	{
		type: 'stats',
		name: 'Statistics',
		description: 'Statistics display with numbers and labels',
		icon: 'bar-chart-3',
		default_props: {
			items: [],
			layout: 'horizontal',
		},
		category: 'data',
	},
	{
		type: 'faq',
		name: 'FAQ Accordion',
		description: 'Frequently asked questions in an accordion layout',
		icon: 'help-circle',
		default_props: {
			questions: [],
			title: '',
		},
		category: 'interactive',
	},
	{
		type: 'testimonial',
		name: 'Testimonial',
		description: 'Testimonial quote with attribution',
		icon: 'quote',
		default_props: {
			quote: '',
			author: '',
			role: '',
			avatar_url: '',
		},
		category: 'content',
	},
	{
		type: 'comparison',
		name: 'Comparison',
		description: 'Comparison embed linking to the comparison system (Section 30)',
		icon: 'git-compare',
		default_props: {
			comparison_id: '',
			display_mode: 'table',
		},
		category: 'data',
	},
	{
		type: 'form',
		name: 'Form Embed',
		description: 'Form embed linking to the custom form system (Section 06)',
		icon: 'file-text',
		default_props: {
			form_id: '',
			show_title: true,
			show_border: true,
		},
		category: 'interactive',
	},
	{
		type: 'spacer',
		name: 'Spacer / Divider',
		description: 'Vertical spacer or visual divider between blocks',
		icon: 'minus',
		default_props: {
			height: 32,
			style: 'space',
		},
		category: 'layout',
	},
	{
		type: 'dynamic_listing',
		name: 'Dynamic Listing',
		description: 'Dynamic content listing driven by filters',
		icon: 'list',
		default_props: {
			content_type: '',
			space_id: '',
			limit: 10,
			sort_by: 'created_at',
			sort_order: 'desc',
		},
		category: 'data',
	},
];

/** Returns all registered block definitions. */
export function getBlockTypes(): BlockDefinition[] {
	return [...BLOCK_TYPES];
}

/** Returns the block definition for the given type, or undefined if not found. */
export function getBlockTypeByType(type: string): BlockDefinition | undefined {
	return BLOCK_TYPES.find((bt) => bt.type === type);
}

/** Returns all block definitions in the given category. */
export function getBlockTypesByCategory(category: BlockCategory | string): BlockDefinition[] {
	return BLOCK_TYPES.filter((bt) => bt.category === category);
}

/**
 * Creates a new BlockInstance with a generated UUID and default props
 * copied from the registry. Throws if the block type is not registered.
 */
export function createBlockInstance(type: string): BlockInstance {
	const definition = getBlockTypeByType(type);
	if (!definition) {
		throw new Error(`Unknown block type: ${type}`);
	}

	return {
		block_id: crypto.randomUUID(),
		block_type: type,
		order: 0,
		props: structuredClone(definition.default_props),
	};
}
