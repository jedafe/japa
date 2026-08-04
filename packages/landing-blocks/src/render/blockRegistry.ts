// packages/landing-blocks/src/render/blockRegistry.ts
import type { Component } from 'svelte';
import type { Block } from '../types';
import HeroBlock from '../blocks/HeroBlock.svelte';
import CtaBannerBlock from '../blocks/CtaBannerBlock.svelte';
import CardGridBlock from '../blocks/CardGridBlock.svelte';
import MediaGridBlock from '../blocks/MediaGridBlock.svelte';
import AccordionBlock from '../blocks/AccordionBlock.svelte';
import CarouselBlock from '../blocks/CarouselBlock.svelte';
import EntityCollectionBlock from '../blocks/EntityCollectionBlock.svelte';
import EntityFeatureBlock from '../blocks/EntityFeatureBlock.svelte';

export interface BlockMeta {
  component: Component;
  icon: string;
  label: string;
  category: string;
}

export const blockRegistry: Record<Block['type'], BlockMeta> = {
  hero: { component: HeroBlock, icon: 'Sparkles', label: 'Hero', category: 'hero' },
  cta_banner: { component: CtaBannerBlock, icon: 'Megaphone', label: 'CTA Banner', category: 'action' },
  card_grid: { component: CardGridBlock, icon: 'LayoutGrid', label: 'Card Grid', category: 'content' },
  media_grid: { component: MediaGridBlock, icon: 'Image', label: 'Media Grid', category: 'content' },
  accordion: { component: AccordionBlock, icon: 'ChevronDown', label: 'Accordion / FAQ', category: 'content' },
  carousel: { component: CarouselBlock, icon: 'GalleryHorizontal', label: 'Carousel', category: 'layout' },
  entity_collection: { component: EntityCollectionBlock, icon: 'List', label: 'Entity Collection', category: 'data' },
  entity_feature: { component: EntityFeatureBlock, icon: 'Star', label: 'Entity Feature', category: 'data' },
};
