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
