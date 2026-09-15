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
import VideoBlock from '../blocks/VideoBlock.svelte';
import IconBlock from '../blocks/IconBlock.svelte';
import ContentBlock from '../blocks/ContentBlock.svelte';
import QuoteBlock from '../blocks/QuoteBlock.svelte';
import SpacerDividerBlock from '../blocks/SpacerDividerBlock.svelte';
import CountdownBlock from '../blocks/CountdownBlock.svelte';
import ComingSoonBlock from '../blocks/ComingSoonBlock.svelte';
import FormEmbedBlock from '../blocks/FormEmbedBlock.svelte';
import TimelineBlock from '../blocks/TimelineBlock.svelte';
import MapBlock from '../blocks/MapBlock.svelte';
import TableBlock from '../blocks/TableBlock.svelte';
import AdvertisementBlock from '../blocks/AdvertisementBlock.svelte';
import LiveRateTickerBlock from '../blocks/LiveRateTickerBlock.svelte';
import TabsBlock from '../blocks/TabsBlock.svelte';
import MediaBlock from '../blocks/MediaBlock.svelte';
import HeadingBlock from '../blocks/HeadingBlock.svelte';
import CtaButtonBlock from '../blocks/CtaButtonBlock.svelte';
import ContainerBlock from '../blocks/ContainerBlock.svelte';
import CustomBlock from '../blocks/CustomBlock.svelte';
import WidgetBlock from '../blocks/WidgetBlock.svelte';
import GuestHomeContainerBlock from '../blocks/guest/GuestHomeContainerBlock.svelte';

export interface BlockMeta {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  component: Component<any>;
  icon: string;
  label: string;
  category: string;
}

export const blockRegistry: Record<Block['type'], BlockMeta> = {
  heading: { component: HeadingBlock, icon: 'Heading', label: 'Heading', category: 'layout' },
  cta_button: { component: CtaButtonBlock, icon: 'MousePointerClick', label: 'CTA Button', category: 'action' },
  hero: { component: HeroBlock, icon: 'Sparkles', label: 'Hero', category: 'hero' },
  cta_banner: { component: CtaBannerBlock, icon: 'Megaphone', label: 'CTA Banner', category: 'action' },
  card_grid: { component: CardGridBlock, icon: 'LayoutGrid', label: 'Card Grid', category: 'content' },
  media_grid: { component: MediaGridBlock, icon: 'Image', label: 'Media Grid', category: 'content' },
  accordion: { component: AccordionBlock, icon: 'ChevronDown', label: 'Accordion / FAQ', category: 'content' },
  carousel: { component: CarouselBlock, icon: 'GalleryHorizontal', label: 'Carousel', category: 'layout' },
  entity_collection: { component: EntityCollectionBlock, icon: 'List', label: 'Entity Collection', category: 'data' },
  entity_feature: { component: EntityFeatureBlock, icon: 'Star', label: 'Entity Feature', category: 'data' },
  video: { component: VideoBlock, icon: 'Video', label: 'Video', category: 'media' },
  icon: { component: IconBlock, icon: 'Icon', label: 'Icon', category: 'media' },
  content: { component: ContentBlock, icon: 'FileText', label: 'Rich Text', category: 'content' },
  quote: { component: QuoteBlock, icon: 'Quote', label: 'Quote', category: 'content' },
  spacer_divider: { component: SpacerDividerBlock, icon: 'SeparatorHorizontal', label: 'Spacer / Divider', category: 'layout' },
  countdown: { component: CountdownBlock, icon: 'Timer', label: 'Countdown', category: 'utility' },
  coming_soon: { component: ComingSoonBlock, icon: 'Clock', label: 'Coming Soon', category: 'utility' },
  form_embed: { component: FormEmbedBlock, icon: 'FormInput', label: 'Form Embed', category: 'action' },
  timeline: { component: TimelineBlock, icon: 'GitBranch', label: 'Timeline', category: 'content' },
  map: { component: MapBlock, icon: 'Map', label: 'Map', category: 'geo' },
  table: { component: TableBlock, icon: 'Table', label: 'Table', category: 'content' },
  advertisement: { component: AdvertisementBlock, icon: 'Megaphone', label: 'Advertisement', category: 'ads' },
  live_rate_ticker: { component: LiveRateTickerBlock, icon: 'TrendingUp', label: 'Live Rate Ticker', category: 'data' },
  tabs: { component: TabsBlock, icon: 'PanelTop', label: 'Tabs', category: 'layout' },
  media: { component: MediaBlock, icon: 'Image', label: 'Single Media', category: 'media' },
  container: { component: ContainerBlock, icon: 'LayoutPanelTop', label: 'Container', category: 'layout' },
  guest_home: { component: GuestHomeContainerBlock, icon: 'Home', label: 'Guest Home', category: 'guest' },
  custom: { component: CustomBlock, icon: 'Puzzle', label: 'Custom Block', category: 'custom' },
  widget: { component: WidgetBlock, icon: 'LayoutDashboard', label: 'Widget', category: 'data' },
} as unknown as Record<Block['type'], BlockMeta>;
