// packages/landing-blocks/src/types.ts
// Page Builder block types — shared by apps/main (rendering) and apps/admin (editors).

/** A media library reference (imgproxy URL + ID for re-selection). */
export interface MediaRef {
  media_id: string;
  url: string;
  alt: string;
}

/** Background configuration shared by all blocks. */
export interface BackgroundConfig {
  type: 'none' | 'color' | 'gradient' | 'image' | 'video';
  color?: string;
  gradient?: { from: string; to: string; angle: number | 'radial' };
  image?: MediaRef & { fit: 'cover' | 'contain' | 'repeat' | 'fixed'; overlay?: { color: string; opacity: number } };
  video?: MediaRef & { poster?: string; loop: boolean; muted: boolean; autoplay: boolean };
  blur?: 'none' | 'sm' | 'md' | 'lg' | 'xl';
  opacity?: number;
}

/** Motion configuration for scroll-triggered reveals. */
export interface MotionConfig {
  preset:
    | 'none'
    | 'fade-up' | 'fade-in' | 'fade-down'
    | 'fly-in-left' | 'fly-in-right' | 'fly-in-up' | 'fly-in-down'
    | 'slide-in' | 'slide-in-left' | 'slide-in-right'
    | 'scale-in' | 'scale-out'
    | 'zoom-in' | 'zoom-out'
    | 'blur-in' | 'blur-out'
    | 'flip-up' | 'flip-down'
    | 'bounce-in';
  delay: number;
  /** Duration in ms (default 700). */
  duration?: number;
  /** CSS easing function (default 'ease-out'). */
  easing?: string;
  /** Stagger child elements within the block. */
  stagger_children?: boolean;
  /** Stagger delay between children in ms (default 80). */
  stagger_delay?: number;
  /** Viewport threshold (0–1) to trigger the animation (default 0.1). */
  threshold?: number;
  /** Only animate once (default true). */
  once?: boolean;
}

/** Visibility rules for geo-targeted sections. */
export interface VisibilityRules {
  geo_countries?: string[];
  hide_if_empty?: boolean;
}

/** The universal style object carried by every block. */
export interface BlockStyle {
  layout?: {
    position?: 'static' | 'relative' | 'absolute' | 'sticky' | 'fixed';
    top?: number | 'auto' | string;
    right?: number | 'auto' | string;
    bottom?: number | 'auto' | string;
    left?: number | 'auto' | string;
    z_index?: 'auto' | 0 | 10 | 20 | 30 | 40 | 50 | 100 | 999;
    width?: 'auto' | 'full' | 'contained' | 'fit' | number | string;
    min_width?: number | string;
    max_width?: number | string;
    min_height?: number | 'auto' | string;
    height?: 'auto' | 'full' | number | string;
    max_height?: number | 'auto' | string;
    display?: 'block' | 'flex' | 'grid' | 'inline-flex' | 'inline-block' | 'hidden';
    flex_direction?: 'row' | 'column' | 'row-reverse' | 'column-reverse';
    flex_wrap?: 'nowrap' | 'wrap' | 'wrap-reverse';
    align_items?: 'start' | 'center' | 'end' | 'stretch' | 'baseline';
    justify_content?: 'start' | 'center' | 'end' | 'between' | 'around' | 'evenly';
    grid_template_columns?: number | string;
    grid_template_rows?: number | string;
    gap?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | number;
    order?: number;
  };
  typography?: {
    heading_size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl' | '5xl' | number;
    heading_weight?: 100 | 200 | 300 | 400 | 500 | 600 | 700 | 800 | 900;
    heading_family?: 'sans' | 'serif' | 'mono';
    heading_color?: string;
    body_size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | number;
    body_weight?: 100 | 200 | 300 | 400 | 500 | 600 | 700 | 800 | 900;
    body_family?: 'sans' | 'serif' | 'mono';
    body_color?: string;
    text_align?: 'left' | 'center' | 'right' | 'justify';
  };
  color?: { text?: string; heading?: string; accent?: string };
  background?: BackgroundConfig;
  border?: {
    width?: 0 | 1 | 2 | 4 | 8;
    style?: 'none' | 'solid' | 'dashed' | 'dotted' | 'double';
    color?: string;
    radius?: 'none' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | 'full' | number;
    sides?: 'all' | 'top' | 'bottom' | 'left' | 'right' | 'x' | 'y';
  };
  spacing?: {
    padding?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | { top?: string; right?: string; bottom?: string; left?: string };
    margin?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | { top?: string; right?: string; bottom?: string; left?: string };
  };
  effects?: {
    shadow?: 'none' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'inner';
    opacity?: number;
    backdrop_blur?: 'none' | 'sm' | 'md' | 'lg' | 'xl';
    overflow?: 'visible' | 'hidden' | 'clip' | 'scroll' | 'auto';
  };
  transform?: {
    scale?: number;
    rotate?: number;
    translate_x?: number;
    translate_y?: number;
  };
  transition?: {
    property?: 'none' | 'all' | 'colors' | 'opacity' | 'transform' | 'shadow';
    duration?: number;
    easing?: 'linear' | 'ease' | 'ease-in' | 'ease-out' | 'ease-in-out';
    delay?: number;
  };
  icon?: {
    name?: string;
    size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | number;
    color?: string;
    background_color?: string;
    shape?: 'none' | 'circle' | 'square' | 'rounded';
    position?: 'left' | 'top' | 'right' | 'none';
  };
  /** Custom CSS injected as a scoped style on the block's shell. Use `&` as a placeholder for the block's scoped selector (e.g. `& { color: red; }`). */
  custom_css?: string;
  /**
   * Per-device responsive style overrides. Each key holds a partial BlockStyle
   * whose values override the base style at that breakpoint.
   * - `tablet` applies at `max-width: 1024px`
   * - `mobile` applies at `max-width: 768px`
   * Desktop uses the base style (no override needed).
   */
  responsive?: {
    tablet?: Partial<BlockStyle>;
    mobile?: Partial<BlockStyle>;
  };
}

/** Geo copy tokens for a single country. */
export interface GeoCopyTokens {
  country: string;
  country_short: string;
  country_adjective: string;
  continent: string;
  region: string;
  diaspora_phrase: string;
  destination_phrase: string;
  locale_hint: string;
  names: { en: string; fr: string };
}

/** Geo context for copy resolution. */
export interface GeoContext {
  contentCountry: string;
  tokens: GeoCopyTokens;
  locale: 'en' | 'fr';
  /**
   * Server-resolved template literal values (e.g. Current_Year).
   * These are merged with geo tokens and take precedence for non-geo keys.
   */
  literals?: Record<string, string>;
}

/** A CTA button. */
export interface CTA {
  label: string;
  href: string;
  variant: 'primary' | 'ghost' | 'outline' | 'link';
  icon?: string;
}

/** The base shape every block instance shares. */
export interface BaseBlock {
  id: string;
  visible: boolean;
  locked?: boolean;
  label?: string;
  style?: BlockStyle;
  motion?: MotionConfig;
  visibility_rules?: VisibilityRules;
  /**
   * Grid column span (1–12) when this block is a child of a `container`
   * block in `grid` mode. Ignored for top-level blocks and non-grid
   * containers. Falls back to the container's `columns` value when unset.
   */
  span?: number;
  /**
   * Per-device visibility overrides. When set to `false`, the block is
   * hidden at that breakpoint via a CSS container query (the block still
   * renders in the DOM but is `display: none`).
   * - `desktop` applies at min-width > 1024px (hide on desktop, show on smaller)
   * - `tablet` applies at max-width ≤ 1024px
   * - `mobile` applies at max-width ≤ 768px
   * When unset, the block is visible at that breakpoint (default).
   */
  responsive_visibility?: {
    desktop?: boolean;
    tablet?: boolean;
    mobile?: boolean;
  };
}

// ─── Block prop shapes (core set) ─────────────────────────────────────────

export interface HeroProps {
  eyebrow?: string;
  headline: string;
  /** Punchy one-liner below the headline, e.g. "Study. Work. Live. Thrive Anywhere." */
  tagline?: string;
  subhead?: string;
  /** The word or phrase inside the headline to accent, e.g. "Simplified." */
  highlight?: string;
  ctas?: CTA[];
  stats?: Array<{ label: string; value: string }>;
  floating_card?: { enabled: boolean; content_ref?: string };
  /** Layout mode. `split` renders the text on the left and a media/illustration on the right. */
  layout?: 'center' | 'split';
  /** Optional image/illustration used by the split layout when no slides are provided. */
  media?: MediaRef;
  /** Optional carousel slides for the split layout. If set, the right side becomes a hero slider. */
  slides?: MediaRef[];
  /** Supporting text below the CTAs, e.g. "Join 10,000+ movers." */
  trust_text?: string;
}

export interface CtaBannerProps {
  headline: string;
  subhead?: string;
  ctas?: CTA[];
  alignment?: 'left' | 'center' | 'right';
}

export interface CardGridProps {
  cards: Array<{
    icon?: string;
    icon_background_color?: string;
    icon_color?: string;
    title: string;
    description?: string;
    media?: MediaRef;
    cta?: CTA;
    rating?: number;
    stat_value?: string;
    stat_label?: string;
  }>;
  card_variant: 'feature' | 'testimonial' | 'stat' | 'custom' | 'overlay';
  layout: 'grid' | 'alternating' | 'masonry' | 'carousel' | 'marquee';
  columns: number;
  /**
   * Per-device column overrides (grid layout only).
   * - `tablet` applies at max-width ≤ 1024px
   * - `mobile` applies at max-width ≤ 768px
   * Desktop uses `columns`.
   */
  responsive_columns?: {
    tablet?: number;
    mobile?: number;
  };
}

export interface MediaGridProps {
  items: Array<{ media: MediaRef; caption?: string; href?: string }>;
  card_style: 'default' | 'logo' | 'overlay';
  lightbox: boolean;
  columns: number;
}

export interface AccordionProps {
  items: Array<{ title: string; content_blocks?: Block[] }>;
  preset: 'faq' | 'generic';
  layout: 'accordion' | 'expanded';
  searchable: boolean;
}

export interface CarouselProps {
  slides: Block[];
  autoplay: boolean;
  dots: boolean;
  arrows: boolean;
  transition: 'slide' | 'fade';
  interval: number;
  pause_on_hover: boolean;
}

export type EntityType = 'post' | 'blog' | 'thread' | 'space' | 'trending_space' | 'organization' | 'university' | 'job' | 'scholarship' | 'tool' | 'remittance_rate';

export interface EntityCollectionProps {
  entity_type: EntityType;
  query: { filters?: Record<string, string>; sort?: string; limit?: number; geo?: boolean };
  layout: 'grid' | 'list' | 'slider' | 'marquee';
  card_variant: 'feature' | 'compact' | 'minimal';
  columns: number;
}

export interface EntityFeatureProps {
  entity_type: EntityType;
  ref?: { id?: string; slug?: string };
  query?: { space?: string; tag?: string; sort?: string };
  layout: 'hero' | 'card' | 'split' | 'banner';
  media_override?: MediaRef;
}

export interface VideoProps {
  provider: 'youtube' | 'facebook' | 'vimeo' | 'file';
  url?: string;
  id?: string;
  aspect_ratio: '16:9' | '4:3' | '1:1' | '21:9';
  autoplay: boolean;
  mute: boolean;
  captions: boolean;
  loop: boolean;
  thumbnail?: MediaRef;
  title?: string;
  description?: string;
}

export interface IconProps {
  icon: string;
  size: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | number;
  color: string;
  background_color: string;
  shape: 'none' | 'circle' | 'square' | 'rounded';
  label?: string;
  href?: string;
}

export interface ContentProps {
  html: string;
  prose_style: 'default' | 'minimal' | 'article' | 'documentation';
}

export interface QuoteProps {
  quote: string;
  author: string;
  role?: string;
  avatar?: MediaRef;
  layout: 'centered' | 'inline' | 'large';
}

export interface SpacerDividerProps {
  height: number;
  divider_style: 'none' | 'line' | 'gradient' | 'dots';
  divider_color?: string;
}

export interface CountdownProps {
  target_date: string;
  label?: string;
  layout: 'inline' | 'stacked' | 'card';
  show_days: boolean;
  show_hours: boolean;
  show_minutes: boolean;
  show_seconds: boolean;
  on_expire: 'hide' | 'show_message';
  expired_message?: string;
}

export interface ComingSoonProps {
  title: string;
  description?: string;
  icon?: string;
  estimated_date?: string;
  waitlist_form?: { enabled: boolean; cta_label: string; endpoint: string };
  background?: BackgroundConfig;
}

export interface FormEmbedProps {
  preset: 'newsletter' | 'waitlist' | 'contact' | 'custom';
  fields: Array<{ name: string; label: string; type: 'text' | 'email' | 'textarea' | 'select'; required: boolean; placeholder?: string; options?: string[] }>;
  submit_endpoint: string;
  submit_label: string;
  success_message: string;
  error_message: string;
  title?: string;
  subtitle?: string;
}

export interface TimelineProps {
  steps: Array<{ icon?: string; title: string; description?: string; media?: MediaRef }>;
  layout: 'horizontal' | 'vertical' | 'zigzag';
}

export interface MapProps {
  mode: 'static' | 'dynamic';
  pins: Array<{ country: string; label: string; href?: string }>;
  geo_source: 'manual' | 'trending_spaces';
  highlight_visitor_country: boolean;
}

export interface TableProps {
  columns: Array<{ key: string; label: string; align?: 'left' | 'center' | 'right' }>;
  rows: Array<Record<string, string>>;
  sortable: boolean;
  striped: boolean;
}

export interface AdvertisementProps {
  slot_key: string;
}

export interface LiveRateTickerProps {
  corridors?: string[];
  limit: number;
  geo_aware: boolean;
  layout: 'horizontal' | 'vertical';
  speed?: number;
}

export interface TabsProps {
  tabs: Array<{ label: string; icon?: string; content_blocks?: Block[] }>;
  layout: 'underline' | 'pills' | 'boxed';
  orientation: 'horizontal' | 'vertical';
  initial_tab: number;
}

export interface MediaBlockProps {
  media: MediaRef;
  layout: 'full' | 'contained' | 'split';
  caption?: string;
  lightbox: boolean;
}

/**
 * Props for the Widget block — embeds a `widget_configurations` record
 * (Section 16) into a page. The widget's rendered data is fetched from
 * `/api/v1/widgets/{widget_id}` (Valkey-cached) and injected into the
 * block at render time.
 *
 * When the page is rendered server-side, the widget data may be pre-resolved
 * and injected via `_resolved` to avoid a client-side fetch.
 */
export interface WidgetBlockProps {
  /** UUID of the widget_configurations row. */
  widget_id: string;
  /** Display layout for the widget data. */
  layout: 'grid' | 'list' | 'compact' | 'ghost' | 'magazine' | 'reddit';
  /** Number of columns when layout is 'grid'. */
  columns: number;
  /** Optional heading rendered above the widget data. */
  heading?: string;
  /** Optional "view all" link href. */
  view_all_href?: string;
  /** Pre-resolved widget data injected by the server (internal — not edited). */
  _resolved?: {
    widget_type: string;
    data: unknown;
    card_type?: string;
    cached_at?: string;
  };
}

/**
 * Props for the Container block — a flexible layout wrapper that groups
 * child blocks with stack / grid / flex layouts and arbitrary nesting.
 *
 * In `grid` mode, each child's `span` (1–12) controls how many of the 12
 * grid columns it occupies, enabling side-by-side sections (e.g. a hero
 * with span 8 next to a CTA with span 4). When `span` is unset, children
 * share the grid evenly based on `columns`.
 *
 * In `stack` mode, children render vertically (equivalent to the flat
 * section model, but nestable).
 *
 * In `flex` mode, children flow horizontally with wrap, aligned by
 * `align_items` / `justify_content`.
 */
export interface ContainerProps {
  layout: 'stack' | 'grid' | 'flex';
  /** Number of grid columns (1–12). Only used in `grid` mode. */
  columns: number;
  /** Gap between children. Maps to the spacing preset scale. */
  gap: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
  /** Child blocks — arbitrary nesting depth (containers inside containers). */
  children: Block[];
  /** Vertical alignment of children. */
  align_items?: 'start' | 'center' | 'end' | 'stretch';
  /** Horizontal distribution of children (flex mode). */
  justify_content?: 'start' | 'center' | 'end' | 'between' | 'around' | 'evenly';
  /** Allow children to wrap to new rows (flex mode). */
  wrap?: boolean;
  /** Minimum height in px (useful for hero-sized containers). */
  min_height?: number | 'auto';
  /** Constrain max width like other full-width sections. */
  max_width?: 'sm' | 'md' | 'lg' | 'xl' | 'full';
  /**
   * Per-device column overrides (grid mode only).
   * - `tablet` applies at max-width ≤ 1024px
   * - `mobile` applies at max-width ≤ 768px
   * Desktop uses `columns`.
   */
  responsive_columns?: {
    tablet?: number;
    mobile?: number;
  };
}

/** Props for a standalone CTA Button block — a single prominent button link. */
export interface CtaButtonProps {
  /** Button label (supports {{geo}} tokens). */
  label: string;
  /** Destination URL. */
  href: string;
  /** Visual variant. */
  variant: 'primary' | 'outline' | 'ghost' | 'link' | 'gradient';
  /** Button size. */
  size: 'sm' | 'md' | 'lg' | 'xl';
  /** Optional icon name (lucide) shown before the label. */
  icon?: string;
  /** Optional icon shown after the label (e.g. arrow). */
  icon_right?: string;
  /** Alignment within the section. */
  align: 'left' | 'center' | 'right';
  /** Make the button full-width. */
  full_width?: boolean;
  /** Open link in new tab. */
  new_tab?: boolean;
  /** Optional subtext shown below the button. */
  subtext?: string;
  /** Gradient from color (for variant: 'gradient'). */
  gradient_from?: string;
  /** Gradient to color (for variant: 'gradient'). */
  gradient_to?: string;
}

/** Props for the Heading block — a stylable section heading. */
export interface HeadingProps {
  /** The heading text (supports {{geo}} tokens). */
  text: string;
  /** Optional eyebrow/overline above the heading. */
  eyebrow?: string;
  /** Optional subheading/description below the heading. */
  subheading?: string;
  /** Heading level (h1–h6). */
  level: 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6';
  /** Visual size preset (independent of semantic level). */
  size: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl';
  /** Text alignment. */
  align: 'left' | 'center' | 'right';
  /** Optional accent style for the heading. */
  accent?: 'none' | 'underline' | 'highlight' | 'gradient';
  /** Optional accent color (CSS value or var). */
  accent_color?: string;
  /** Optional icon name (lucide) shown before the eyebrow. */
  icon?: string;
  /** Optional CTA link shown on the right of the heading. */
  cta?: { label: string; href: string };
  /** Max width constraint: 'sm' | 'md' | 'lg' | 'xl' | 'full'. */
  max_width?: 'sm' | 'md' | 'lg' | 'xl' | 'full';
}

// ─── Guest homepage data types ─────────────────────────────────────────

/** A destination space surfaced in the guest hero floating card. */
export interface GuestHomeDestination {
  id: string;
  slug?: string;
  name: string;
  member_count?: number;
  icon_url?: string | null;
  href?: string;
}

/** A marketing stat block on the guest landing page. */
export interface GuestHomeStat {
  id?: string;
  value: string;
  label: string;
  icon: string;
}

/** A category card on the guest landing page. */
export interface GuestHomeCategory {
  id: string;
  title: string;
  description: string;
  href: string;
  icon: string;
  color: string;
}

/** A featured-opportunity card on the guest landing page. */
export interface GuestHomeOpportunity {
  id: string;
  type: 'job' | 'scholarship' | 'visa' | 'business' | 'internship' | string;
  title: string;
  organization: string;
  location: string;
  location_code?: string;
  tag: string;
  badge?: string;
  value?: string;
  logo_url?: string | null;
  href: string;
  published_at?: string;
}

/** A success-story card on the guest landing page. */
export interface GuestHomeStory {
  id: string;
  slug?: string;
  title: string;
  quote: string;
  author_name: string;
  author_role?: string;
  author_avatar_url?: string | null;
  href?: string;
  destination?: string;
}

/** A curated tool card surfaced on the guest landing page. */
export interface GuestHomeTool {
  id: string;
  name: string;
  slug?: string;
  description?: string;
  href?: string;
  icon?: string;
  badge?: string;
  accent?: string;
}

/** SSR payload for the default guest homepage. */
export interface GuestHomeData {
  topDestinations: GuestHomeDestination[];
  opportunities: GuestHomeOpportunity[];
  stories: GuestHomeStory[];
  tools: GuestHomeTool[];
  stats: GuestHomeStat[];
  categories: GuestHomeCategory[];
}

/** Copy overrides for the guest homepage block. Keys mirror the original
 *  `landing.home.*` translation keys. When a key is absent, the block falls
 *  back to the default English string baked into the component. */
export interface GuestHomeCopy {
  // Page meta
  title?: string;
  description?: string;

  // Hero
  hero_label?: string;
  eyebrow?: string;
  headline_prefix?: string;
  headline_accent?: string;
  subhead?: string;
  cta_start?: string;
  cta_explore?: string;
  hero_image_alt?: string;
  top_destinations?: string;
  see_all_countries?: string;
  join_title?: string;
  join_subtitle?: string;
  join_cta_label?: string;

  // Explore
  explore_title?: string;
  explore_subtitle?: string;
  explore_link?: string;

  // Opportunities
  opportunities_title?: string;
  opportunities_view_all?: string;
  carousel_prev?: string;
  carousel_next?: string;

  // Stories
  stories_eyebrow?: string;
  stories_title?: string;
  stories_subtitle?: string;
  stories_read_more?: string;
  story_dot?: string;

  // Tools
  tools_title?: string;
  tools_subtitle?: string;

  // Newsletter
  newsletter_title?: string;
  newsletter_subtitle?: string;
  newsletter_placeholder?: string;
  newsletter_cta?: string;
  newsletter_success?: string;
  newsletter_error?: string;

  // Footer
  footer_label?: string;
  footer_tagline?: string;
  footer_explore?: string;
  footer_jobs?: string;
  footer_study?: string;
  footer_visa?: string;
  footer_business?: string;
  footer_countries?: string;
  footer_resources?: string;
  footer_blog?: string;
  footer_guides?: string;
  footer_tools?: string;
  footer_compare?: string;
  footer_stories?: string;
  footer_company?: string;
  footer_about?: string;
  footer_how?: string;
  footer_pricing?: string;
  footer_faqs?: string;
  footer_contact?: string;
  footer_help?: string;
  footer_hours?: string;
  footer_rights?: string;
  footer_privacy?: string;
  footer_terms?: string;
  footer_cookies?: string;

  // Social
  social_facebook?: string;
  social_twitter?: string;
  social_instagram?: string;
  social_youtube?: string;
  social_linkedin?: string;

  // Time ago
  time_recent?: string;
  time_year?: string;
  time_month?: string;
  time_week?: string;
  time_day?: string;
  time_hour?: string;
  time_minute?: string;
  time_ago?: string;
  time_just?: string;

  // App
  app_name?: string;
}

/** Props for the guest homepage block — wraps the legacy GuestHomePage
 *  layout as a single Page Builder block. */
export interface GuestHomeProps {
  /** Copy overrides; keys match `landing.home.*` translation keys. */
  copy?: GuestHomeCopy;
  /** Hero image URL. */
  hero_image?: string;
  /** Light and dark logo URLs for the footer. */
  logo?: { light?: string; dark?: string };
  /** Newsletter subscription endpoint. */
  newsletter_endpoint?: string;
}

/** Shared layout props for the guest card-list blocks. */
export interface GuestCardListBaseProps {
  title?: string;
  subtitle?: string;
  view_all_href?: string;
  view_all_label?: string;
  layout?: 'grid' | 'slider';
  columns?: number;
  responsive_columns?: { tablet?: number; mobile?: number };
  gap?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
  card_style?: 'default' | 'outlined' | 'filled';
}

/** Props for the `guest_home` container preset. */
export interface GuestHomeContainerProps {
  layout?: 'stack';
  children?: Block[];
  /** Legacy copy overrides, kept for backward compatibility. */
  copy?: GuestHomeCopy;
  /** Legacy hero image URL. */
  hero_image?: string;
  /** Legacy light/dark logo URLs for the footer. */
  logo?: { light?: string; dark?: string };
  /** Legacy newsletter subscription endpoint. */
  newsletter_endpoint?: string;
}

/** Props for the `guest_hero` container block. */
export interface GuestHeroContainerProps {
  layout?: 'grid';
  columns?: number;
  responsive_columns?: { tablet?: number; mobile?: number };
  gap?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
  children?: Block[];
}

/** Props for `guest_hero_text`. */
export interface GuestHeroTextProps {
  eyebrow?: string;
  headline_prefix?: string;
  headline_accent?: string;
  subhead?: string;
  /** Legacy CTA array; superseded by `children` (`guest_hero_cta_item` blocks). */
  ctas?: CTA[];
  /** CTA blocks rendered in the actions row. Overrides `ctas` when non-empty. */
  children?: Block[];
}

/** Props for a single CTA button inside `guest_hero_text`. */
export interface GuestHeroCtaItemProps {
  label?: string;
  href?: string;
  variant?: 'primary' | 'ghost' | 'outline' | 'link';
  icon?: string;
}

/** Props for `guest_hero_image`. */
export interface GuestHeroImageProps {
  slides?: MediaRef[];
  image_alt?: string;
  autoplay?: boolean;
  interval?: number;
  dots?: boolean;
  arrows?: boolean;
  children?: Block[];
}

/** Props for `guest_hero_stats`. */
export interface GuestHeroStatsProps {
  title?: string;
  data_source?: 'context' | 'manual';
  /** Legacy data array; superseded by `children` (`guest_hero_stat_item` blocks). */
  items?: GuestHomeStat[];
  /**
   * Stat item blocks. `data_source: 'manual'` renders them as-is; in
   * `context` mode the first child acts as a template cloned per context stat.
   */
  children?: Block[];
  max_items?: number;
}

/** Props for a single stat inside `guest_hero_stats`. */
export interface GuestHeroStatItemProps {
  value?: string;
  label?: string;
  icon?: string;
}

/** Props for `guest_hero_destinations`. */
export interface GuestHeroDestinationsProps {
  title?: string;
  see_all_href?: string;
  data_source?: 'context' | 'manual';
  /** Legacy data array; superseded by `children` for block-based items. */
  items?: GuestHomeDestination[];
  /** Nested block items used when `data_source` is `manual`. */
  children?: Block[];
  max_items?: number;
}

/** Props for a single item inside `guest_hero_destinations`. */
export interface GuestHeroDestinationItemProps {
  name?: string;
  icon_url?: string | null;
  href?: string;
  code?: string;
}

/** Props for `guest_hero_community`. */
export interface GuestHeroCommunityProps {
  title?: string;
  subtitle?: string;
  cta?: { label?: string; href?: string; icon?: string };
  data_source?: 'context' | 'manual';
  value?: string;
  label?: string;
  /** Nested member blocks used when `data_source` is `manual`. */
  children?: Block[];
  max_items?: number;
}

/** Props for a single member avatar inside `guest_hero_community`. */
export interface GuestHeroCommunityMemberProps {
  image_url?: string | null;
  name?: string;
  href?: string;
}

/** Props for `guest_categories`. */
export interface GuestCategoriesProps extends GuestCardListBaseProps {
  title?: string;
  subtitle?: string;
  view_all_href?: string;
  data_source?: 'context' | 'manual';
  /** Legacy data array; superseded by `children` (`guest_category_item` blocks). */
  items?: GuestHomeCategory[];
  /** Category item blocks; in `context` mode the first child is a per-item template. */
  children?: Block[];
  max_items?: number;
}

/** Props for a single category card inside `guest_categories`. */
export interface GuestCategoryItemProps {
  title?: string;
  description?: string;
  href?: string;
  icon?: string;
  color?: string;
}

/** Props for `guest_opportunities`. */
export interface GuestOpportunitiesProps extends GuestCardListBaseProps {
  title?: string;
  view_all_href?: string;
  data_source?: 'context' | 'manual';
  /** Legacy data array; superseded by `children` (`guest_opportunity_item` blocks). */
  items?: GuestHomeOpportunity[];
  /** Opportunity item blocks; in `context` mode the first child is a per-item template. */
  children?: Block[];
  max_items?: number;
}

/** Props for a single opportunity card inside `guest_opportunities`. */
export interface GuestOpportunityItemProps {
  type?: string;
  title?: string;
  organization?: string;
  location?: string;
  location_code?: string;
  tag?: string;
  badge?: string;
  value?: string;
  logo_url?: string | null;
  href?: string;
  published_at?: string;
}

/** Props for `guest_stories`. */
export interface GuestStoriesProps extends GuestCardListBaseProps {
  eyebrow?: string;
  cta_label?: string;
  cta_href?: string;
  data_source?: 'context' | 'manual';
  /** Legacy data array; superseded by `children` (`guest_story_item` blocks). */
  items?: GuestHomeStory[];
  /** Story item blocks; in `context` mode the first child is a per-item template. */
  children?: Block[];
  max_items?: number;
}

/** Props for a single story card inside `guest_stories`. */
export interface GuestStoryItemProps {
  title?: string;
  slug?: string;
  quote?: string;
  author_name?: string;
  author_role?: string;
  author_avatar_url?: string | null;
  href?: string;
  destination?: string;
}

/** Props for `guest_tools`. */
export interface GuestToolsProps extends GuestCardListBaseProps {
  title?: string;
  subtitle?: string;
  view_all_href?: string;
  data_source?: 'context' | 'manual';
  /** Legacy data array; superseded by `children` (`guest_tool_item` blocks). */
  items?: GuestHomeTool[];
  /** Tool item blocks; in `context` mode the first child is a per-item template. */
  children?: Block[];
  max_items?: number;
}

/** Props for a single tool card inside `guest_tools`. */
export interface GuestToolItemProps {
  name?: string;
  slug?: string;
  description?: string;
  href?: string;
  icon?: string;
  badge?: string;
  accent?: string;
}

/** Props for `guest_newsletter`. */
export interface GuestNewsletterProps {
  title?: string;
  subtitle?: string;
  endpoint?: string;
  placeholder?: string;
  cta?: string;
  success_message?: string;
  error_message?: string;
  /** Arbitrary child blocks rendered inside the newsletter card. */
  children?: Block[];
}

/** Props for `guest_footer`. */
export interface GuestFooterProps {
  logo?: { light?: string; dark?: string };
  tagline?: string;
  /** Legacy socials array; superseded by `guest_footer_social` children. */
  socials?: Array<{ id?: string; icon: string; url: string; label: string }>;
  /** Legacy columns array; superseded by `guest_footer_column` children. */
  columns?: Array<{
    id?: string;
    title: string;
    links: Array<{ label: string; url?: string; target?: string; icon?: string }>;
  }>;
  copyright?: string;
  /** Legacy bottom-links array; superseded by `guest_footer_link` children. */
  bottom_links?: Array<{ label: string; url: string; target?: string }>;
  /**
   * Footer children partitioned by type: `guest_footer_social` renders in the
   * brand socials row, `guest_footer_column` renders in the columns grid (each
   * column holds `guest_footer_link` children), and `guest_footer_link` renders
   * in the bottom legal bar.
   */
  children?: Block[];
}

/** Props for a single column inside `guest_footer`. */
export interface GuestFooterColumnProps {
  title?: string;
  /** `guest_footer_link` children rendered as the column's link list. */
  children?: Block[];
}

/** Props for a single link inside a `guest_footer_column` or the footer bottom bar. */
export interface GuestFooterLinkProps {
  label?: string;
  url?: string;
  target?: string;
  icon?: string;
}

/** Props for a single social icon inside `guest_footer`. */
export interface GuestFooterSocialProps {
  icon?: string;
  url?: string;
  label?: string;
}

/** Discriminated union of all block types. */
export type Block =
  | (BaseBlock & { type: 'heading'; props: HeadingProps })
  | (BaseBlock & { type: 'cta_button'; props: CtaButtonProps })
  | (BaseBlock & { type: 'hero'; props: HeroProps })
  | (BaseBlock & { type: 'cta_banner'; props: CtaBannerProps })
  | (BaseBlock & { type: 'card_grid'; props: CardGridProps })
  | (BaseBlock & { type: 'media_grid'; props: MediaGridProps })
  | (BaseBlock & { type: 'accordion'; props: AccordionProps })
  | (BaseBlock & { type: 'carousel'; props: CarouselProps })
  | (BaseBlock & { type: 'entity_collection'; props: EntityCollectionProps })
  | (BaseBlock & { type: 'entity_feature'; props: EntityFeatureProps })
  | (BaseBlock & { type: 'video'; props: VideoProps })
  | (BaseBlock & { type: 'icon'; props: IconProps })
  | (BaseBlock & { type: 'content'; props: ContentProps })
  | (BaseBlock & { type: 'quote'; props: QuoteProps })
  | (BaseBlock & { type: 'spacer_divider'; props: SpacerDividerProps })
  | (BaseBlock & { type: 'countdown'; props: CountdownProps })
  | (BaseBlock & { type: 'coming_soon'; props: ComingSoonProps })
  | (BaseBlock & { type: 'form_embed'; props: FormEmbedProps })
  | (BaseBlock & { type: 'timeline'; props: TimelineProps })
  | (BaseBlock & { type: 'map'; props: MapProps })
  | (BaseBlock & { type: 'table'; props: TableProps })
  | (BaseBlock & { type: 'advertisement'; props: AdvertisementProps })
  | (BaseBlock & { type: 'live_rate_ticker'; props: LiveRateTickerProps })
  | (BaseBlock & { type: 'tabs'; props: TabsProps })
  | (BaseBlock & { type: 'media'; props: MediaBlockProps })
  | (BaseBlock & { type: 'widget'; props: WidgetBlockProps })
  | (BaseBlock & { type: 'guest_home'; props: GuestHomeContainerProps })
  | (BaseBlock & { type: 'guest_hero'; props: GuestHeroContainerProps })
  | (BaseBlock & { type: 'guest_hero_text'; props: GuestHeroTextProps })
  | (BaseBlock & { type: 'guest_hero_image'; props: GuestHeroImageProps })
  | (BaseBlock & { type: 'guest_hero_stats'; props: GuestHeroStatsProps })
  | (BaseBlock & { type: 'guest_hero_destinations'; props: GuestHeroDestinationsProps })
  | (BaseBlock & { type: 'guest_hero_destination_item'; props: GuestHeroDestinationItemProps })
  | (BaseBlock & { type: 'guest_hero_community'; props: GuestHeroCommunityProps })
  | (BaseBlock & { type: 'guest_hero_community_member'; props: GuestHeroCommunityMemberProps })
  | (BaseBlock & { type: 'guest_hero_cta_item'; props: GuestHeroCtaItemProps })
  | (BaseBlock & { type: 'guest_hero_stat_item'; props: GuestHeroStatItemProps })
  | (BaseBlock & { type: 'guest_categories'; props: GuestCategoriesProps })
  | (BaseBlock & { type: 'guest_category_item'; props: GuestCategoryItemProps })
  | (BaseBlock & { type: 'guest_opportunities'; props: GuestOpportunitiesProps })
  | (BaseBlock & { type: 'guest_opportunity_item'; props: GuestOpportunityItemProps })
  | (BaseBlock & { type: 'guest_stories'; props: GuestStoriesProps })
  | (BaseBlock & { type: 'guest_story_item'; props: GuestStoryItemProps })
  | (BaseBlock & { type: 'guest_tools'; props: GuestToolsProps })
  | (BaseBlock & { type: 'guest_tool_item'; props: GuestToolItemProps })
  | (BaseBlock & { type: 'guest_newsletter'; props: GuestNewsletterProps })
  | (BaseBlock & { type: 'guest_footer'; props: GuestFooterProps })
  | (BaseBlock & { type: 'guest_footer_column'; props: GuestFooterColumnProps })
  | (BaseBlock & { type: 'guest_footer_link'; props: GuestFooterLinkProps })
  | (BaseBlock & { type: 'guest_footer_social'; props: GuestFooterSocialProps })
  | (BaseBlock & { type: 'container'; props: ContainerProps })
  | (BaseBlock & { type: 'custom'; props: CustomBlockProps; block_def_id: string });

/** A landing page as returned by GET /pages/{slug}. */
export interface LandingPage {
  id: string;
  title: string;
  slug: string;
  description?: string | null;
  blocks: Block[];
  meta?: Record<string, unknown>;
  /**
   * Server-resolved template literal values for this page (e.g. Current_Year).
   * Used by the SSR resolver alongside geo tokens.
   */
  literals?: Record<string, string>;
}

// ─── Runtime (DB-driven) custom block definitions ──────────────
// These types describe the `pagebuilder_blocks` table rows and the
// schema-driven rendering/editing of custom blocks. Custom blocks have
// `type: 'custom'` and carry a `block_def_id` linking back to the DB row.

/** A single field in a custom block's config schema. */
export interface ConfigSchemaField {
  /** Field key in the props object. */
  key: string;
  /** Human-readable label. */
  label: string;
  /** Input type for the editor + rendering hint. */
  type: 'text' | 'textarea' | 'number' | 'color' | 'select' | 'checkbox' | 'media' | 'url' | 'richtext';
  /** Options for `select` type. */
  options?: Array<{ value: string; label: string }>;
  /** Default value if not set in props. */
  default?: unknown;
  /** Help text shown in the editor. */
  help?: string;
  /** Whether this field is required. */
  required?: boolean;
  /** Placeholder for text/textarea/url inputs. */
  placeholder?: string;
}

/** The config_schema JSON stored in the DB row. */
export interface BlockConfigSchema {
  fields: ConfigSchemaField[];
}

/** Props for a `custom` block instance — keyed by the schema field keys. */
export type CustomBlockProps = Record<string, unknown>;

/** A row from the `pagebuilder_blocks` table (as returned by the API). */
export interface CustomBlockDef {
  id: string;
  name: string;
  slug: string;
  category: string;
  component: string;
  default_config: CustomBlockProps;
  config_schema: BlockConfigSchema;
  preview_image: string | null;
  is_system: boolean;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
}
