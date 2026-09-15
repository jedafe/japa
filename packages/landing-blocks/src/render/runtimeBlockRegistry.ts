// packages/landing-blocks/src/render/runtimeBlockRegistry.ts
// Runtime block registry — merges the static `blockRegistry` (compiled-in
// blocks) with DB-driven custom block definitions fetched from the
// `pagebuilder_blocks` table. The admin editor and public renderer both
// consult this registry to discover available block types and their metadata.
//
// Custom blocks use `type: 'custom'` in the Block union and carry a
// `block_def_id` linking back to the DB row. Their rendering and editing is
// schema-driven (see CustomBlock.svelte + CustomBlockEditor.svelte).

import type { CustomBlockDef, BlockConfigSchema, ConfigSchemaField } from '../types';

export interface RuntimeBlockMeta {
  /** The block type string used in the Block union. For custom blocks this is `custom`. */
  type: string;
  /** Human-readable label. */
  label: string;
  /** Category for palette grouping. */
  category: string;
  /** Lucide icon name. */
  icon: string;
  /** Whether this is a DB-driven custom block (vs a compiled-in static block). */
  isCustom: boolean;
  /** For custom blocks: the DB definition ID. */
  blockDefId?: string;
  /** For custom blocks: the config schema (drives the editor form). */
  configSchema?: BlockConfigSchema;
  /** For custom blocks: the default config (seeded into new instances). */
  defaultConfig?: Record<string, unknown>;
  /** For custom blocks: the slug (unique platform-wide). */
  slug?: string;
}

// In-memory cache of fetched custom block definitions, keyed by block def ID.
// This is a plain module-level Map — not reactive. Callers that need
// reactivity should wrap their consumption in a Svelte `$state`/`$derived`.
const customBlockDefs = new Map<string, CustomBlockDef>();

// Track whether definitions have been loaded to avoid redundant fetches.
let loaded = false;

/** Register custom block definitions (typically from an API fetch). */
export function setCustomBlockDefs(defs: CustomBlockDef[]): void {
	customBlockDefs.clear();
	for (const def of defs) {
		if (def.is_enabled) {
			customBlockDefs.set(def.id, def);
		}
	}
	loaded = true;
}

/** Get all registered custom block definitions. */
export function getCustomBlockDefs(): CustomBlockDef[] {
	return Array.from(customBlockDefs.values());
}

/** Look up a single custom block definition by ID. */
export function getCustomBlockDef(id: string): CustomBlockDef | undefined {
	return customBlockDefs.get(id);
}

/** Look up a custom block definition by slug. */
export function getCustomBlockDefBySlug(slug: string): CustomBlockDef | undefined {
	for (const def of customBlockDefs.values()) {
		if (def.slug === slug) return def;
	}
	return undefined;
}

/** Whether custom block definitions have been loaded. */
export function isRuntimeRegistryLoaded(): boolean {
	return loaded;
}

/** Reset the runtime registry (useful for tests / logout). */
export function resetRuntimeRegistry(): void {
	customBlockDefs.clear();
	loaded = false;
}

/**
 * Build runtime metadata for all custom blocks, suitable for merging into
 * the palette. Returns an array of `RuntimeBlockMeta` entries.
 */
export function getCustomBlockMetas(): RuntimeBlockMeta[] {
	return getCustomBlockDefs().map((def) => ({
		type: 'custom',
		label: def.name,
		category: def.category || 'custom',
		icon: 'Puzzle',
		isCustom: true,
		blockDefId: def.id,
		configSchema: normalizeSchema(def.config_schema),
		defaultConfig: def.default_config ?? {},
		slug: def.slug,
	}));
}

/**
 * Resolve the effective props for a custom block instance, merging defaults
 * from the block definition with the instance's stored props.
 */
export function resolveCustomBlockProps(
	blockDefId: string,
	instanceProps: Record<string, unknown>,
): Record<string, unknown> {
	const def = getCustomBlockDef(blockDefId);
	if (!def) return instanceProps;
	const schema = normalizeSchema(def.config_schema);
	const defaults: Record<string, unknown> = {};
	for (const field of schema.fields) {
		if (field.default !== undefined) defaults[field.key] = field.default;
	}
	return { ...defaults, ...(def.default_config ?? {}), ...instanceProps };
}

/** Normalize a raw config_schema (could be `{}` or missing `fields`) into a valid schema. */
function normalizeSchema(schema: BlockConfigSchema | undefined | null): BlockConfigSchema {
	if (!schema || !schema.fields || !Array.isArray(schema.fields)) {
		return { fields: [] };
	}
	return { fields: schema.fields.filter(isValidField) };
}

function isValidField(f: unknown): f is ConfigSchemaField {
	return (
		typeof f === 'object' &&
		f !== null &&
		typeof (f as ConfigSchemaField).key === 'string' &&
		typeof (f as ConfigSchemaField).label === 'string' &&
		typeof (f as ConfigSchemaField).type === 'string'
	);
}
