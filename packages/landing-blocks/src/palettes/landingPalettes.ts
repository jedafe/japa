// packages/landing-blocks/src/palettes/landingPalettes.ts
import type { Block } from '../types';

export interface PaletteDefinition {
  label: string;
  allowedBlocks: Block['type'][];
  defaultSeed: Array<{
    type: Block['type'];
    label: string;
    props: Record<string, unknown>;
    motion?: Block['motion'];
    style?: Block['style'];
    visibility_rules?: Block['visibility_rules'];
  }>;
}

export const landingPalettes: Record<string, PaletteDefinition> = {
  homepage: {
    label: 'Homepage',
    allowedBlocks: [
      'guest_home', 'heading', 'cta_button', 'hero', 'carousel', 'card_grid', 'media_grid', 'entity_collection',
      'entity_feature', 'accordion', 'cta_banner', 'video', 'icon', 'content',
      'quote', 'spacer_divider', 'countdown', 'coming_soon', 'form_embed',
      'timeline', 'map', 'table', 'advertisement', 'live_rate_ticker',
      'tabs', 'media',
    ],
    defaultSeed: [
      {
        type: 'guest_home',
        label: 'Guest Home',
        props: {},
      },
    ],
  },
};
