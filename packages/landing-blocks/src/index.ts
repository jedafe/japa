// @japaconnect/landing-blocks — shared block library for landing pages.
// Consumed by apps/main (public rendering) and apps/admin (live preview + editors).
export const PACKAGE_NAME = '@japaconnect/landing-blocks';

export { setAssetBaseUrl, resolveAssetUrl } from './assetBase';

export type {
  Block, BaseBlock, BlockStyle, MotionConfig, GeoContext, GeoCopyTokens,
  BackgroundConfig, MediaRef, VisibilityRules, CTA, LandingPage,
  HeroProps, CtaBannerProps, CardGridProps, MediaGridProps,
  AccordionProps, CarouselProps, EntityCollectionProps, EntityFeatureProps,
  EntityType,
  VideoProps, IconProps, ContentProps, QuoteProps, SpacerDividerProps,
  CountdownProps, ComingSoonProps, FormEmbedProps, TimelineProps,
  MapProps, TableProps, AdvertisementProps, LiveRateTickerProps,
  ExploreMapProps, ExploreMapView, ExploreMapCategory, ExploreMapStat,
  ExploreMapPin, ExploreMapBand,
  TabsProps, MediaBlockProps,
  HeadingProps,
  CtaButtonProps,
  ContainerProps,
  CustomBlockProps, CustomBlockDef, BlockConfigSchema, ConfigSchemaField,
  WidgetBlockProps,
  GuestHomeProps,
  GuestHomeCopy,
  GuestHomeData,
  GuestHomeStat,
  GuestHomeCategory,
  GuestHomeOpportunity,
  GuestHomeTool,
  GuestHomeStory,
  GuestHomeDestination,
} from './types';

export { geoCopy, GLOBAL_TOKENS, getGeoTokens } from './geo/copyRegistry';
export { EXPLORE_MAP_DEFAULTS } from './blocks/explore-map/exploreMapDefaults';
export { WORLD_COUNTRIES, WORLD_VIEWBOX, REGION_BOUNDS, COUNTRY_BOUNDS } from './geo/worldMapData';
export { ADMIN1_CENTROIDS } from './geo/worldAdmin1';
export { CITY_POSITIONS, CITY_SHAPES } from './geo/worldCities';
export { default as ExploreMapBlock } from './blocks/explore-map/ExploreMapBlock.svelte';

export { styleToCss, responsiveStyleToCss, type ResolvedStyle } from './shell/styleToCss';

export { resolveString, resolveGeoCopy } from './geo/resolveGeoCopy';

export { fetchEntityCollection, fetchEntityFeature, fetchRemittanceRates, TOOL_REGISTRY } from './data/entityFetchers';
export type { FetchContext, JobSummary, ScholarshipSummary, UniversitySummary, SpaceSummary, BlogSummary, FeedItemSummary, RemittanceRate, ToolSummary } from './data/types';

export { landingPalettes, type PaletteDefinition } from './palettes/landingPalettes';

export { blockRegistry, type BlockMeta } from './render/blockRegistry';
export {
  setCustomBlockDefs, getCustomBlockDefs, getCustomBlockDef,
  getCustomBlockDefBySlug, getCustomBlockMetas, resolveCustomBlockProps,
  isRuntimeRegistryLoaded, resetRuntimeRegistry, type RuntimeBlockMeta,
} from './render/runtimeBlockRegistry';
export { default as LandingRenderer } from './render/LandingRenderer.svelte';
export { setGuestHomeData, getGuestHomeData } from './data/guestHomeContext';
export { containsGuestBlocks } from './render/blockVisibility';
export { normalizeLegacyBlocks, normalizeLegacyBlock, type LegacyBlock } from './render/legacyCompat';
export { default as BlockShell } from './shell/BlockShell.svelte';
export { default as Reveal } from './shell/Reveal.svelte';

// Editors (tree-shaken from production build — only used by admin)
export { default as GeoText } from './editors/GeoText.svelte';
export { default as MediaPicker } from './editors/MediaPicker.svelte';
export { default as CTAEditor } from './editors/CTAEditor.svelte';
export { default as BackgroundPicker } from './editors/BackgroundPicker.svelte';
export { default as IconPicker } from './editors/IconPicker.svelte';
export { default as EntityRefPicker } from './editors/EntityRefPicker.svelte';
export { default as NestedBlocksEditor } from './editors/NestedBlocksEditor.svelte';
export { default as BlockStylePanel } from './editors/BlockStylePanel.svelte';
export { contentEditorRegistry } from './editors/content/index';
