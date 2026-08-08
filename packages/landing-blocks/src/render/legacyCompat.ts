// packages/landing-blocks/src/render/legacyCompat.ts
//
// Compatibility layer for legacy Page Builder block data (pre-landing-blocks
// package). The legacy /pages/[slug] route stored blocks with simpler type
// names and prop shapes. This module normalizes legacy blocks into the new
// Block format so LandingRenderer can render them without a separate
// BlockRenderer.
//
// Used by the /pages/[slug] route to consolidate rendering onto
// LandingRenderer (single rendering path).
import type { Block } from '../types';

/** Legacy block shape from the old Page Builder (LandingBlock). */
export interface LegacyBlock {
  id?: string;
  type: string;
  props: Record<string, unknown>;
}

/**
 * Maps legacy block type names to the new canonical type names used by
 * blockRegistry. Blocks not in this map are passed through unchanged (the
 * registry will skip unknown types).
 */
const legacyTypeMap: Record<string, Block['type']> = {
  // Renamed types
  rich_text: 'content',
  dynamic_listing: 'entity_collection',
  image_gallery: 'media_grid',
  gallery: 'media_grid',
  slider: 'carousel',
  faq: 'accordion',
  faq_accordion: 'accordion',
  testimonial: 'quote',
  testimonial_quote: 'quote',
  cta: 'cta_button',
  ad: 'advertisement',
  grid_row: 'container',
  // Direct matches (no rename needed, but listed for clarity)
  hero: 'hero',
  cta_button: 'cta_button',
  spacer: 'spacer_divider',
  spacer_divider: 'spacer_divider',
  countdown: 'countdown',
  form_embed: 'form_embed',
  table: 'table',
  advertisement: 'advertisement',
  container: 'container',
};

/**
 * Transforms legacy prop shapes to the new prop shapes for blocks where
 * the props differ. Returns the props unchanged for blocks with compatible
 * shapes.
 */
function migrateProps(
  newType: Block['type'],
  legacyProps: Record<string, unknown>,
): Record<string, unknown> {
  const p = { ...legacyProps };

  switch (newType) {
    case 'hero': {
      // Legacy: { title, subtitle, cta_text, cta_url, background_image }
      // New: { headline, subhead, ctas: [{label, href, variant}], ... }
      if (!('headline' in p) && 'title' in p) {
        p.headline = p.title;
        delete p.title;
      }
      if (!('subhead' in p) && 'subtitle' in p) {
        p.subhead = p.subtitle;
        delete p.subtitle;
      }
      if (!('ctas' in p) && ('cta_text' in p || 'cta_url' in p)) {
        const label = (p.cta_text as string) ?? '';
        const href = (p.cta_url as string) ?? '';
        if (label || href) {
          p.ctas = [{ label, href, variant: 'primary' }];
        }
        delete p.cta_text;
        delete p.cta_url;
      }
      break;
    }
    case 'content': {
      // Legacy: { content: "<html>" } → New: { html: "<html>" }
      if (!('html' in p) && 'content' in p) {
        p.html = p.content;
        delete p.content;
      }
      break;
    }
    case 'entity_collection': {
      // Legacy: { feed_type, limit } → New: { entity_type, limit }
      if (!('entity_type' in p) && 'feed_type' in p) {
        p.entity_type = p.feed_type;
        delete p.feed_type;
      }
      break;
    }
    case 'spacer_divider': {
      // Legacy: { variant: 'spacer' | 'divider' } → New handles both
      // via props.variant (compatible).
      break;
    }
    case 'quote': {
      // Legacy: { quote, author_name, author_title } → New: { text, author: { name, title } }
      if (!('text' in p) && 'quote' in p) {
        p.text = p.quote;
        delete p.quote;
      }
      if (!('author' in p) && ('author_name' in p || 'author_title' in p)) {
        p.author = {
          name: (p.author_name as string) ?? '',
          title: (p.author_title as string) ?? '',
        };
        delete p.author_name;
        delete p.author_title;
      }
      break;
    }
  }

  return p;
}

/**
 * Normalizes a single legacy block into the new Block format. If the block
 * type is unknown or has no mapping, it is returned with the type mapped to
 * 'custom' (which the registry will skip if no custom block def is found).
 */
export function normalizeLegacyBlock(legacy: LegacyBlock, index: number): Block {
  const newType = legacyTypeMap[legacy.type] ?? legacy.type;
  const props = migrateProps(newType, legacy.props);
  // Build the block with a type cast — the Block union includes 'custom'
  // which requires block_def_id, but legacy blocks never produce 'custom'
  // (unknown types are passed through as-is and skipped by the registry).
  const block: Record<string, unknown> = {
    id: legacy.id ?? `legacy-${index}`,
    type: newType,
    visible: true,
    props,
  };
  return block as unknown as Block;
}

/**
 * Normalizes an array of legacy blocks into the new Block[] format for
 * LandingRenderer. Unknown legacy types are preserved as-is (the registry
 * will skip them gracefully).
 */
export function normalizeLegacyBlocks(legacy: LegacyBlock[]): Block[] {
  return legacy.map((b, i) => normalizeLegacyBlock(b, i));
}
