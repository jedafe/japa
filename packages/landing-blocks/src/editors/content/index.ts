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
import ContainerContentEditor from './ContainerContentEditor.svelte';
import CustomBlockEditor from './CustomBlockEditor.svelte';
import WidgetContentEditor from './WidgetContentEditor.svelte';
import GuestHomeContentEditor from './guest/GuestHomeContentEditor.svelte';
import GuestHeroContainerContentEditor from './guest/GuestHeroContainerContentEditor.svelte';
import GuestHeroTextContentEditor from './guest/GuestHeroTextContentEditor.svelte';
import GuestHeroImageContentEditor from './guest/GuestHeroImageContentEditor.svelte';
import GuestHeroStatsContentEditor from './guest/GuestHeroStatsContentEditor.svelte';
import GuestHeroDestinationsContentEditor from './guest/GuestHeroDestinationsContentEditor.svelte';
import GuestHeroDestinationItemContentEditor from './guest/GuestHeroDestinationItemContentEditor.svelte';
import GuestHeroCommunityContentEditor from './guest/GuestHeroCommunityContentEditor.svelte';
import GuestHeroCommunityMemberContentEditor from './guest/GuestHeroCommunityMemberContentEditor.svelte';
import GuestCategoriesContentEditor from './guest/GuestCategoriesContentEditor.svelte';
import GuestOpportunitiesContentEditor from './guest/GuestOpportunitiesContentEditor.svelte';
import GuestStoriesContentEditor from './guest/GuestStoriesContentEditor.svelte';
import GuestToolsContentEditor from './guest/GuestToolsContentEditor.svelte';
import GuestNewsletterContentEditor from './guest/GuestNewsletterContentEditor.svelte';
import GuestFooterContentEditor from './guest/GuestFooterContentEditor.svelte';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const contentEditorRegistry: Record<Block['type'], Component<any>> = {
  heading: HeadingContentEditor,
  cta_button: CtaButtonContentEditor,
  container: ContainerContentEditor,
  custom: CustomBlockEditor,
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
  guest_home: GuestHomeContentEditor,
  guest_hero: GuestHeroContainerContentEditor,
  guest_hero_text: GuestHeroTextContentEditor,
  guest_hero_image: GuestHeroImageContentEditor,
  guest_hero_stats: GuestHeroStatsContentEditor,
  guest_hero_destinations: GuestHeroDestinationsContentEditor,
  guest_hero_destination_item: GuestHeroDestinationItemContentEditor,
  guest_hero_community: GuestHeroCommunityContentEditor,
  guest_hero_community_member: GuestHeroCommunityMemberContentEditor,
  guest_categories: GuestCategoriesContentEditor,
  guest_opportunities: GuestOpportunitiesContentEditor,
  guest_stories: GuestStoriesContentEditor,
  guest_tools: GuestToolsContentEditor,
  guest_newsletter: GuestNewsletterContentEditor,
  guest_footer: GuestFooterContentEditor,
  widget: WidgetContentEditor,
};
