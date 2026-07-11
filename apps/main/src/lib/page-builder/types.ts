/**
 * Page Builder type definitions.
 *
 * These mirror the backend's BlockTemplate shape so the editor can
 * construct, serialise, and persist page schemas without translation.
 */

/** A block type definition from the backend registry. */
export interface BlockDefinition {
	type: string;
	name: string;
	description: string;
	icon: string;
	default_props: Record<string, unknown>;
	category: string;
}

/** A single block instance placed on a page. */
export interface BlockInstance {
	block_id: string;
	block_type: string;
	order: number;
	props: Record<string, unknown>;
}

/** The full page schema stored in contents.field_values. */
export interface PageSchema {
	schema_version: number;
	blocks: BlockInstance[];
}

/** Valid block categories used to group blocks in the picker UI. */
export type BlockCategory = 'layout' | 'content' | 'media' | 'interactive' | 'data' | 'navigation';
