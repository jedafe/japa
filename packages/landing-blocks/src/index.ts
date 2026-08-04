// @japaconnect/landing-blocks — shared block library for landing pages.
// Consumed by apps/main (public rendering) and apps/admin (live preview + editors).
export const PACKAGE_NAME = '@japaconnect/landing-blocks';

export type {
  Block, BaseBlock, BlockStyle, MotionConfig, GeoContext, GeoCopyTokens,
  BackgroundConfig, MediaRef, VisibilityRules, CTA, LandingPage,
  HeroProps, CtaBannerProps, CardGridProps, MediaGridProps,
  AccordionProps, CarouselProps, EntityCollectionProps, EntityFeatureProps,
  EntityType,
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
