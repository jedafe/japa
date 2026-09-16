// packages/landing-blocks/src/render/blockVisibility.ts
// Visibility rule evaluation for landing blocks — geo targeting and hide-if-empty.

import type { Block, VisibilityRules } from '../types';

const contentFields = new Set<string>([
  'headline',
  'subhead',
  'eyebrow',
  'text',
  'subheading',
  'quote',
  'author',
  'html',
  'title',
  'description',
  'content',
  'slot_key',
  'target_date',
  'media',
  'image',
  'video',
  'thumbnail',
  'icon',
  'poster',
  'name',
  'image_url',
]);

/** Block types that load their content dynamically. */
const dynamicBlockTypes = new Set<string>(['widget', 'entity_collection', 'entity_feature', 'live_rate_ticker']);

function hasContentValue(value: unknown): boolean {
  if (value == null || value === '' || value === false) return false;
  if (typeof value === 'string') return value.trim().length > 0;
  if (typeof value === 'number' || typeof value === 'boolean') return true;
  if (Array.isArray(value)) return value.length > 0 && value.some((v) => hasContentValue(v));
  if (typeof value === 'object' && value !== null) {
    if ('url' in value && typeof (value as any).url === 'string' && (value as any).url) return true;
    if ('media_id' in value && typeof (value as any).media_id === 'string' && (value as any).media_id) return true;
    if ('data' in value && (value as any).data != null && (Array.isArray((value as any).data) ? (value as any).data.length > 0 : true)) return true;
    if ('children' in value && Array.isArray((value as any).children) && (value as any).children.length > 0) return true;
    if ('enabled' in value && (value as any).enabled === true) return true;
    if ('label' in value && typeof (value as any).label === 'string' && (value as any).label) return true;
    if ('content' in value && typeof (value as any).content === 'string' && (value as any).content) return true;
    if ('title' in value && typeof (value as any).title === 'string' && (value as any).title) return true;
    if ('headline' in value && typeof (value as any).headline === 'string' && (value as any).headline) return true;
    if ('name' in value && typeof (value as any).name === 'string' && (value as any).name) return true;
    if ('url' in value || 'id' in value || 'icon' in value || 'href' in value) {
      return Object.values(value).some((v) => hasContentValue(v));
    }
    return false;
  }
  return false;
}

/**
 * Best-effort static emptiness check for a block. Returns `true` when the
 * block's props contain no meaningful content (text, media, cards, children, etc.).
 * Dynamic data blocks (widgets, entity collections) that rely on fetched data
 * may still render as empty at runtime; this helper is intended for the
 * `hide_if_empty` visibility rule and favours not hiding blocks that look
 * configured.
 */
export function isBlockEmpty(block: Block): boolean {
  const props = (block as any).props as Record<string, unknown> | undefined;
  if (!props) return true;

  const type = block.type;
  const isDynamic = dynamicBlockTypes.has(type);

  // If a dynamic block already has server-resolved data, use it to decide.
  if (isDynamic) {
    const resolved = (props as any)._resolved;
    if (resolved) {
      const data = resolved.data;
      if (data == null) return true;
      if (Array.isArray(data) && data.length === 0) return true;
      return false;
    }
    const items = (props as any)._items;
    if (items !== undefined) {
      return !Array.isArray(items) || items.length === 0;
    }
    // No resolved data yet: let it render and fetch, then the component reports.
    return false;
  }

  // Static blocks: any recognised content field or array with real content makes it non-empty.
  for (const [key, value] of Object.entries(props)) {
    if (contentFields.has(key) && hasContentValue(value)) return false;
    if (Array.isArray(value) && value.some(hasContentValue)) return false;
  }

  return true;
}

/** Determine whether the current geo context passes the rule. */
export function isBlockGeoVisible(rules: VisibilityRules | undefined, geoCountry?: string): boolean {
  if (!rules?.geo_countries || rules.geo_countries.length === 0) return true;
  if (!geoCountry || geoCountry === 'global') return false;
  return rules.geo_countries.includes(geoCountry);
}

/** Combined visibility decision including `visible`, geo rules, and `hide_if_empty`. */
export function isBlockVisible(block: Block, geoCountry?: string): boolean {
  if (!block.visible) return false;
  if (!isBlockGeoVisible(block.visibility_rules, geoCountry)) return false;
  if (block.visibility_rules?.hide_if_empty && isBlockEmpty(block)) return false;
  return true;
}
