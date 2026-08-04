import type { Component } from 'svelte';
import type { Block } from '../../types';
import HeroContentEditor from './HeroContentEditor.svelte';
import CtaBannerContentEditor from './CtaBannerContentEditor.svelte';
import CardGridContentEditor from './CardGridContentEditor.svelte';
import MediaGridContentEditor from './MediaGridContentEditor.svelte';
import AccordionContentEditor from './AccordionContentEditor.svelte';
import CarouselContentEditor from './CarouselContentEditor.svelte';
import EntityCollectionContentEditor from './EntityCollectionContentEditor.svelte';
import EntityFeatureContentEditor from './EntityFeatureContentEditor.svelte';
import VideoContentEditor from './VideoContentEditor.svelte';
import IconContentEditor from './IconContentEditor.svelte';
import ContentEditor from './ContentEditor.svelte';
import QuoteContentEditor from './QuoteContentEditor.svelte';
import SpacerDividerContentEditor from './SpacerDividerContentEditor.svelte';
import CountdownContentEditor from './CountdownContentEditor.svelte';
import ComingSoonContentEditor from './ComingSoonContentEditor.svelte';
import FormEmbedContentEditor from './FormEmbedContentEditor.svelte';
import TimelineContentEditor from './TimelineContentEditor.svelte';
import MapContentEditor from './MapContentEditor.svelte';
import TableContentEditor from './TableContentEditor.svelte';
import AdvertisementContentEditor from './AdvertisementContentEditor.svelte';
import LiveRateTickerContentEditor from './LiveRateTickerContentEditor.svelte';
import TabsContentEditor from './TabsContentEditor.svelte';
import MediaContentEditor from './MediaContentEditor.svelte';
import HeadingContentEditor from './HeadingContentEditor.svelte';
import CtaButtonContentEditor from './CtaButtonContentEditor.svelte';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const contentEditorRegistry: Record<Block['type'], Component<any>> = {
  heading: HeadingContentEditor,
  cta_button: CtaButtonContentEditor,
  hero: HeroContentEditor,
  cta_banner: CtaBannerContentEditor,
  card_grid: CardGridContentEditor,
  media_grid: MediaGridContentEditor,
  accordion: AccordionContentEditor,
  carousel: CarouselContentEditor,
  entity_collection: EntityCollectionContentEditor,
  entity_feature: EntityFeatureContentEditor,
  video: VideoContentEditor,
  icon: IconContentEditor,
  content: ContentEditor,
  quote: QuoteContentEditor,
  spacer_divider: SpacerDividerContentEditor,
  countdown: CountdownContentEditor,
  coming_soon: ComingSoonContentEditor,
  form_embed: FormEmbedContentEditor,
  timeline: TimelineContentEditor,
  map: MapContentEditor,
  table: TableContentEditor,
  advertisement: AdvertisementContentEditor,
  live_rate_ticker: LiveRateTickerContentEditor,
  tabs: TabsContentEditor,
  media: MediaContentEditor,
};
