// @japaconnect/landing-blocks — shared block library for landing pages.
// Consumed by apps/main (public rendering) and apps/admin (live preview + editors).
export const PACKAGE_NAME = '@japaconnect/landing-blocks';

export type {
  Block, BaseBlock, BlockStyle, MotionConfig, GeoContext, GeoCopyTokens,
  BackgroundConfig, MediaRef, VisibilityRules, CTA, LandingPage,
  HeroProps, CtaBannerProps, CardGridProps, MediaGridProps,
  AccordionProps, CarouselProps, EntityCollectionProps, EntityFeatureProps,
  EntityType,
  VideoProps, IconProps, ContentProps, QuoteProps, SpacerDividerProps,
  CountdownProps, ComingSoonProps, FormEmbedProps, TimelineProps,
  MapProps, TableProps, AdvertisementProps, LiveRateTickerProps,
  TabsProps, MediaBlockProps,
  HeadingProps,
  CtaButtonProps,
} from './types';

export { geoCopy, GLOBAL_TOKENS, getGeoTokens } from './geo/copyRegistry';

export { styleToCss, type ResolvedStyle } from './shell/styleToCss';

export { resolveString, resolveGeoCopy } from './geo/resolveGeoCopy';

export { fetchEntityCollection, fetchEntityFeature, fetchRemittanceRates, TOOL_REGISTRY } from './data/entityFetchers';
export type { FetchContext, JobSummary, ScholarshipSummary, UniversitySummary, SpaceSummary, BlogSummary, FeedItemSummary, RemittanceRate, ToolSummary } from './data/types';

export { landingPalettes, type PaletteDefinition } from './palettes/landingPalettes';

export { blockRegistry, type BlockMeta } from './render/blockRegistry';
export { default as LandingRenderer } from './render/LandingRenderer.svelte';
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
