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
  preset: 'none' | 'fade-up' | 'fade-in' | 'slide-in' | 'scale-in';
  delay: number;
  stagger_children?: boolean;
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
    top?: number | 'auto';
    right?: number | 'auto';
    bottom?: number | 'auto';
    left?: number | 'auto';
    z_index?: 'auto' | 0 | 10 | 20 | 30 | 40 | 50 | 100 | 999;
    width?: 'auto' | 'full' | 'contained' | 'fit' | number;
    max_width?: number;
    min_height?: number | 'auto';
    height?: 'auto' | 'full' | number;
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
}

// ─── Block prop shapes (core set) ─────────────────────────────────────────

export interface HeroProps {
  eyebrow?: string;
  headline: string;
  subhead?: string;
  ctas?: CTA[];
  stats?: Array<{ label: string; value: string }>;
  floating_card?: { enabled: boolean; content_ref?: string };
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
    title: string;
    description?: string;
    media?: MediaRef;
    cta?: CTA;
    rating?: number;
    stat_value?: string;
    stat_label?: string;
  }>;
  card_variant: 'feature' | 'testimonial' | 'stat' | 'custom';
  layout: 'grid' | 'alternating' | 'masonry' | 'carousel' | 'marquee';
  columns: number;
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

export type EntityType = 'post' | 'blog' | 'thread' | 'space' | 'organization' | 'university' | 'job' | 'scholarship' | 'tool' | 'remittance_rate';

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

/** Discriminated union of all block types. */
export type Block =
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
  | (BaseBlock & { type: 'media'; props: MediaBlockProps });

/** A landing page as returned by GET /pages/{slug}. */
export interface LandingPage {
  id: string;
  title: string;
  slug: string;
  description?: string | null;
  blocks: Block[];
  meta?: Record<string, unknown>;
}
