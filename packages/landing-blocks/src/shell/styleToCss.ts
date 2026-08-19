import type { BlockStyle } from '../types';

export interface ResolvedStyle {
  classes: string;
  inline: Record<string, string>;
  /** CSS text for responsive media-query overrides, keyed by a scope selector. */
  responsiveCss?: string;
}

const RADIUS_MAP: Record<string, string> = {
  none: 'rounded-none', sm: 'rounded-sm', md: 'rounded-md', lg: 'rounded-lg',
  xl: 'rounded-xl', '2xl': 'rounded-2xl', '3xl': 'rounded-3xl', full: 'rounded-full',
};

const SHADOW_MAP: Record<string, string> = {
  none: 'shadow-none', sm: 'shadow-sm', md: 'shadow-md', lg: 'shadow-lg',
  xl: 'shadow-xl', '2xl': 'shadow-2xl', inner: 'shadow-inner',
};

export const BLUR_MAP: Record<string, string> = { sm: '4px', md: '8px', lg: '12px', xl: '16px' };

const TRANSITION_PROP_MAP: Record<string, string> = {
  all: 'all',
  colors: 'color, background-color, border-color, text-decoration-color, fill, stroke',
  opacity: 'opacity',
  transform: 'transform',
  shadow: 'box-shadow',
};

const HEADING_SIZE_MAP: Record<string, string> = {
  xs: '0.75rem', sm: '0.875rem', md: '1rem', lg: '1.125rem', xl: '1.25rem',
  '2xl': '1.5rem', '3xl': '1.875rem', '4xl': '2.25rem', '5xl': '3rem',
};

const FONT_FAMILY_STACK: Record<string, string> = {
  sans: 'var(--font-sans, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica Neue, Arial, sans-serif)',
  serif: 'var(--font-serif, ui-serif, Georgia, Cambria, Times New Roman, Times, serif)',
  mono: 'var(--font-mono, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, Courier New, monospace)',
};

/** Convert a BlockStyle object into Tailwind classes + inline CSS custom properties. */
export function styleToCss(style: BlockStyle | undefined): ResolvedStyle {
  if (!style) return { classes: '', inline: {} };
  const classes: string[] = [];
  const inline: Record<string, string> = {};

  // Layout
  const l = style.layout;
  if (l) {
    if (l.position === 'sticky') classes.push('sticky');
    else if (l.position === 'absolute') classes.push('absolute');
    else if (l.position === 'fixed') classes.push('fixed');
    else if (l.position === 'relative') classes.push('relative');
    if (l.top !== undefined && l.top !== 'auto') inline['top'] = `${l.top}px`;
    if (l.right !== undefined && l.right !== 'auto') inline['right'] = `${l.right}px`;
    if (l.bottom !== undefined && l.bottom !== 'auto') inline['bottom'] = `${l.bottom}px`;
    if (l.left !== undefined && l.left !== 'auto') inline['left'] = `${l.left}px`;
    if (l.z_index !== undefined && l.z_index !== 'auto') inline['z-index'] = String(l.z_index);
    if (l.width === 'contained') classes.push('max-w-5xl mx-auto');
    if (l.width === 'full') classes.push('w-full');
    if (l.width === 'fit') inline['width'] = 'fit-content';
    if (typeof l.width === 'number') inline['width'] = `${l.width}px`;
    if (typeof l.max_width === 'number') inline['max-width'] = `${l.max_width}px`;
    if (l.min_height !== undefined && l.min_height !== 'auto') {
      inline['min-height'] = typeof l.min_height === 'number' ? `${l.min_height}px` : l.min_height;
    }
    if (l.height === 'full') inline['height'] = '100%';
    else if (typeof l.height === 'number') inline['height'] = `${l.height}px`;
    if (l.display === 'flex') classes.push('flex');
    if (l.display === 'grid') classes.push('grid');
    if (l.display === 'hidden') classes.push('hidden');
    if (l.flex_direction === 'column') classes.push('flex-col');
    if (l.flex_direction === 'row-reverse') classes.push('flex-row-reverse');
    if (l.flex_wrap) inline['flex-wrap'] = l.flex_wrap;
    if (l.align_items) classes.push(`items-${l.align_items}`);
    if (l.justify_content) {
      const j = l.justify_content === 'between' ? 'between' : l.justify_content === 'around' ? 'around' : l.justify_content === 'evenly' ? 'evenly' : l.justify_content;
      classes.push(`justify-${j}`);
    }
    if (l.gap !== undefined) {
      inline['gap'] = typeof l.gap === 'number' ? `${l.gap}px` : GAP_PX_MAP[l.gap] ?? '0px';
    }
    if (typeof l.grid_template_columns === 'number') {
      classes.push('grid');
      inline['grid-template-columns'] = `repeat(${l.grid_template_columns}, 1fr)`;
    } else if (typeof l.grid_template_columns === 'string' && l.grid_template_columns) {
      classes.push('grid');
      inline['grid-template-columns'] = l.grid_template_columns;
    }
    if (typeof l.grid_template_rows === 'number') {
      inline['grid-template-rows'] = `repeat(${l.grid_template_rows}, 1fr)`;
    } else if (typeof l.grid_template_rows === 'string' && l.grid_template_rows) {
      inline['grid-template-rows'] = l.grid_template_rows;
    }
    if (typeof l.order === 'number') inline['order'] = String(l.order);
  }

  // Typography
  const t = style.typography;
  if (t) {
    if (t.text_align) classes.push(`text-${t.text_align}`);
    if (t.heading_family === 'serif') classes.push('font-serif');
    if (t.heading_family === 'mono') classes.push('font-mono');
    if (t.heading_family) inline['--block-heading-family'] = FONT_FAMILY_STACK[t.heading_family];
    if (t.heading_weight) inline['--block-heading-weight'] = String(t.heading_weight);
    if (t.heading_size) {
      inline['--block-heading-size'] = typeof t.heading_size === 'string'
        ? HEADING_SIZE_MAP[t.heading_size] ?? '1.5rem'
        : `${t.heading_size}rem`;
    }
    if (t.heading_color) inline['--block-heading-color'] = t.heading_color;
    if (t.body_color) inline['--block-body-color'] = t.body_color;
    if (t.body_family) inline['--block-body-family'] = FONT_FAMILY_STACK[t.body_family];
    if (t.body_weight) inline['--block-body-weight'] = String(t.body_weight);
    if (t.body_size) {
      inline['--block-body-size'] = typeof t.body_size === 'string'
        ? HEADING_SIZE_MAP[t.body_size] ?? '1rem'
        : `${t.body_size}rem`;
    }
  }

  // Color — expose both legacy --block-* variables and theme-aware --color-* variables
  const textColor = style.color?.text;
  const bodyColor = style.typography?.body_color;
  const headingColor = style.typography?.heading_color ?? style.color?.heading;
  const accentColor = style.color?.accent;
  if (textColor) {
    inline['--block-text'] = textColor;
    inline['--color-text'] = textColor;
  }
  if (bodyColor) inline['--block-body-color'] = bodyColor;
  if (textColor || bodyColor) inline['--color-text-subtle'] = (bodyColor ?? textColor) as string;
  if (headingColor) {
    inline['--block-heading-color'] = headingColor;
    inline['--color-heading'] = headingColor;
  }
  if (accentColor) {
    inline['--block-accent'] = accentColor;
    inline['--color-primary'] = accentColor;
  }

  // Border
  const b = style.border;
  if (b) {
    if (b.radius && typeof b.radius === 'string') classes.push(RADIUS_MAP[b.radius] ?? 'rounded-md');
    if (b.width !== undefined && b.width > 0) {
      const side = b.sides ?? 'all';
      const width = `${b.width}px`;
      const styleValue = b.style ?? 'solid';
      const color = b.color ?? 'currentColor';
      if (side === 'all') {
        inline['border'] = `${width} ${styleValue} ${color}`;
      } else {
        const sides = side === 'x' ? ['left', 'right'] : side === 'y' ? ['top', 'bottom'] : [side];
        for (const s of sides) {
          inline[`border-${s}`] = `${width} ${styleValue} ${color}`;
        }
      }
    }
  }

  // Spacing — use inline sizes because the named tokens (xs, sm, md, lg, …)
  // are not guaranteed to exist as Tailwind utility classes.
  const s = style.spacing;
  if (s) {
    if (s.padding) {
      if (typeof s.padding === 'string') {
        inline['padding'] = SPACING_PX_MAP[s.padding] ?? '0px';
      } else {
        if (s.padding.top) inline['padding-top'] = s.padding.top;
        if (s.padding.right) inline['padding-right'] = s.padding.right;
        if (s.padding.bottom) inline['padding-bottom'] = s.padding.bottom;
        if (s.padding.left) inline['padding-left'] = s.padding.left;
      }
    }
    if (s.margin) {
      if (typeof s.margin === 'string') {
        inline['margin'] = SPACING_PX_MAP[s.margin] ?? '0px';
      } else {
        if (s.margin.top) inline['margin-top'] = s.margin.top;
        if (s.margin.right) inline['margin-right'] = s.margin.right;
        if (s.margin.bottom) inline['margin-bottom'] = s.margin.bottom;
        if (s.margin.left) inline['margin-left'] = s.margin.left;
      }
    }
  }

  // Effects
  const e = style.effects;
  if (e) {
    if (e.shadow && typeof e.shadow === 'string') classes.push(SHADOW_MAP[e.shadow] ?? '');
    if (e.opacity !== undefined) inline['opacity'] = String(e.opacity / 100);
    if (e.backdrop_blur && e.backdrop_blur !== 'none') {
      inline['backdrop-filter'] = `blur(${BLUR_MAP[e.backdrop_blur] ?? '8px'})`;
    }
    if (e.overflow && e.overflow !== 'visible') classes.push(`overflow-${e.overflow}`);
  }

  // Transform
  const tr = style.transform;
  if (tr) {
    const parts: string[] = [];
    if (tr.scale !== undefined) parts.push(`scale(${tr.scale})`);
    if (tr.rotate !== undefined) parts.push(`rotate(${tr.rotate}deg)`);
    if (tr.translate_x !== undefined) parts.push(`translateX(${tr.translate_x}px)`);
    if (tr.translate_y !== undefined) parts.push(`translateY(${tr.translate_y}px)`);
    if (parts.length) inline['transform'] = parts.join(' ');
  }

  // Transition
  const tx = style.transition;
  if (tx && tx.property && tx.property !== 'none') {
    const prop = TRANSITION_PROP_MAP[tx.property] ?? tx.property;
    const duration = tx.duration ?? 300;
    const easing = tx.easing ?? 'ease';
    const delay = tx.delay ?? 0;
    inline['transition'] = `${prop} ${duration}ms ${easing} ${delay}ms`;
  }

  return { classes: classes.filter(Boolean).join(' '), inline };
}

// ─── Responsive overrides ──────────────────────────────────────────────────

/** Breakpoint max-widths for responsive overrides. */
const BREAKPOINTS: Record<string, number> = {
  tablet: 1024,
  mobile: 768,
};

/**
 * Convert a (partial) BlockStyle into raw CSS declarations suitable for use
 * inside a media query. Unlike `styleToCss` which produces Tailwind classes +
 * inline styles, this produces plain `property: value` pairs that can be
 * scoped to a selector and wrapped in `@media`.
 */
function styleToCssDeclarations(style: Partial<BlockStyle>): string[] {
  const decls: string[] = [];

  const l = style.layout;
  if (l) {
    if (l.display === 'hidden') decls.push('display: none');
    else if (l.display === 'flex') decls.push('display: flex');
    else if (l.display === 'grid') decls.push('display: grid');
    else if (l.display === 'block') decls.push('display: block');
    if (l.flex_direction) decls.push(`flex-direction: ${l.flex_direction}`);
    if (l.flex_wrap) decls.push(`flex-wrap: ${l.flex_wrap}`);
    if (l.align_items) decls.push(`align-items: ${l.align_items}`);
    if (l.justify_content) decls.push(`justify-content: ${l.justify_content === 'between' ? 'space-between' : l.justify_content === 'around' ? 'space-around' : l.justify_content === 'evenly' ? 'space-evenly' : l.justify_content}`);
    if (l.gap !== undefined) {
      const gapVal = typeof l.gap === 'number' ? `${l.gap}px` : GAP_PX_MAP[l.gap] ?? '0px';
      decls.push(`gap: ${gapVal}`);
    }
    if (typeof l.grid_template_columns === 'number') decls.push(`grid-template-columns: repeat(${l.grid_template_columns}, 1fr)`);
    else if (typeof l.grid_template_columns === 'string' && l.grid_template_columns) decls.push(`grid-template-columns: ${l.grid_template_columns}`);
    if (typeof l.grid_template_rows === 'number') decls.push(`grid-template-rows: repeat(${l.grid_template_rows}, 1fr)`);
    else if (typeof l.grid_template_rows === 'string' && l.grid_template_rows) decls.push(`grid-template-rows: ${l.grid_template_rows}`);
    if (l.width === 'full') decls.push('width: 100%');
    if (l.width === 'contained') decls.push('max-width: 64rem', 'margin-left: auto', 'margin-right: auto');
    if (l.width === 'fit') decls.push('width: fit-content');
    if (typeof l.width === 'number') decls.push(`width: ${l.width}px`);
    if (l.max_width !== undefined) decls.push(`max-width: ${l.max_width}px`);
    if (l.min_height !== undefined && l.min_height !== 'auto') decls.push(`min-height: ${typeof l.min_height === 'number' ? l.min_height + 'px' : l.min_height}`);
    if (l.height === 'full') decls.push('height: 100%');
    else if (typeof l.height === 'number') decls.push(`height: ${l.height}px`);
    if (l.flex_wrap) decls.push(`flex-wrap: ${l.flex_wrap}`);
    if (l.position && l.position !== 'static') decls.push(`position: ${l.position}`);
    if (typeof l.top === 'number') decls.push(`top: ${l.top}px`);
    if (typeof l.right === 'number') decls.push(`right: ${l.right}px`);
    if (typeof l.bottom === 'number') decls.push(`bottom: ${l.bottom}px`);
    if (typeof l.left === 'number') decls.push(`left: ${l.left}px`);
    if (l.z_index !== undefined && l.z_index !== 'auto') decls.push(`z-index: ${l.z_index}`);
    if (l.order !== undefined) decls.push(`order: ${l.order}`);
  }

  const t = style.typography;
  if (t) {
    if (t.text_align) decls.push(`text-align: ${t.text_align}`);
    if (t.heading_size) {
      decls.push(`--block-heading-size: ${typeof t.heading_size === 'string' ? HEADING_SIZE_MAP[t.heading_size] ?? '1.5rem' : t.heading_size + 'rem'}`);
    }
    if (t.heading_weight) decls.push(`--block-heading-weight: ${t.heading_weight}`);
    if (t.heading_color) decls.push(`--block-heading-color: ${t.heading_color}`);
    if (t.heading_family) decls.push(`font-family: ${t.heading_family === 'serif' ? 'serif' : t.heading_family === 'mono' ? 'monospace' : 'sans-serif'}`);
    if (t.body_size) {
      decls.push(`--block-body-size: ${typeof t.body_size === 'string' ? HEADING_SIZE_MAP[t.body_size] ?? '1rem' : t.body_size + 'rem'}`);
    }
    if (t.body_weight) decls.push(`--block-body-weight: ${t.body_weight}`);
    if (t.body_color) decls.push(`--block-body-color: ${t.body_color}`);
    if (t.body_family) decls.push(`font-family: ${t.body_family === 'serif' ? 'serif' : t.body_family === 'mono' ? 'monospace' : 'sans-serif'}`);
  }

  const textColor = style.color?.text;
  const bodyColor = style.typography?.body_color;
  const headingColor = style.typography?.heading_color ?? style.color?.heading;
  const accentColor = style.color?.accent;
  if (textColor) {
    decls.push(`--block-text: ${textColor}`);
    decls.push(`--color-text: ${textColor}`);
  }
  if (bodyColor) decls.push(`--block-body-color: ${bodyColor}`);
  if (textColor || bodyColor) decls.push(`--color-text-subtle: ${(bodyColor ?? textColor) as string}`);
  if (headingColor) {
    decls.push(`--block-heading-color: ${headingColor}`);
    decls.push(`--color-heading: ${headingColor}`);
  }
  if (accentColor) {
    decls.push(`--block-accent: ${accentColor}`);
    decls.push(`--color-primary: ${accentColor}`);
  }

  const b = style.border;
  if (b) {
    if (b.radius !== undefined) {
      const r = typeof b.radius === 'number' ? b.radius + 'px' : RADIUS_PX_MAP[b.radius] ?? '6px';
      decls.push(`border-radius: ${r}`);
    }
    if (b.width !== undefined && b.width > 0) {
      const side = b.sides ?? 'all';
      const width = `${b.width}px`;
      const styleValue = b.style ?? 'solid';
      const color = b.color ?? 'currentColor';
      if (side === 'all') {
        decls.push(`border: ${width} ${styleValue} ${color}`);
      } else {
        const sides = side === 'x' ? ['left', 'right'] : side === 'y' ? ['top', 'bottom'] : [side];
        for (const s of sides) {
          decls.push(`border-${s}: ${width} ${styleValue} ${color}`);
        }
      }
    }
  }

  const s = style.spacing;
  if (s) {
    if (s.padding) {
      if (typeof s.padding === 'string') decls.push(`padding: ${SPACING_PX_MAP[s.padding] ?? '0px'}`);
      else {
        if (s.padding.top) decls.push(`padding-top: ${s.padding.top}`);
        if (s.padding.right) decls.push(`padding-right: ${s.padding.right}`);
        if (s.padding.bottom) decls.push(`padding-bottom: ${s.padding.bottom}`);
        if (s.padding.left) decls.push(`padding-left: ${s.padding.left}`);
      }
    }
    if (s.margin) {
      if (typeof s.margin === 'string') decls.push(`margin: ${SPACING_PX_MAP[s.margin] ?? '0px'}`);
      else {
        if (s.margin.top) decls.push(`margin-top: ${s.margin.top}`);
        if (s.margin.right) decls.push(`margin-right: ${s.margin.right}`);
        if (s.margin.bottom) decls.push(`margin-bottom: ${s.margin.bottom}`);
        if (s.margin.left) decls.push(`margin-left: ${s.margin.left}`);
      }
    }
  }

  const e = style.effects;
  if (e) {
    if (e.shadow && e.shadow !== 'none') decls.push(`box-shadow: ${SHADOW_CSS_MAP[e.shadow] ?? ''}`);
    if (e.opacity !== undefined) decls.push(`opacity: ${e.opacity / 100}`);
    if (e.backdrop_blur && e.backdrop_blur !== 'none') decls.push(`backdrop-filter: blur(${BLUR_MAP[e.backdrop_blur] ?? '8px'})`);
    if (e.overflow && e.overflow !== 'visible') decls.push(`overflow: ${e.overflow}`);
  }

  const tr = style.transform;
  if (tr) {
    const parts: string[] = [];
    if (tr.scale !== undefined) parts.push(`scale(${tr.scale})`);
    if (tr.rotate !== undefined) parts.push(`rotate(${tr.rotate}deg)`);
    if (tr.translate_x !== undefined) parts.push(`translateX(${tr.translate_x}px)`);
    if (tr.translate_y !== undefined) parts.push(`translateY(${tr.translate_y}px)`);
    if (parts.length) decls.push(`transform: ${parts.join(' ')}`);
  }

  const tx = style.transition;
  if (tx && tx.property && tx.property !== 'none') {
    const prop = TRANSITION_PROP_MAP[tx.property] ?? tx.property;
    const duration = tx.duration ?? 300;
    const easing = tx.easing ?? 'ease';
    const delay = tx.delay ?? 0;
    decls.push(`transition: ${prop} ${duration}ms ${easing} ${delay}ms`);
  }

  return decls;
}

const GAP_PX_MAP: Record<string, string> = {
  none: '0px', xs: '4px', sm: '8px', md: '16px', lg: '24px', xl: '32px', '2xl': '48px',
};
const RADIUS_PX_MAP: Record<string, string> = {
  none: '0px', sm: '2px', md: '6px', lg: '8px', xl: '12px', '2xl': '16px', '3xl': '24px', full: '9999px',
};
const SPACING_PX_MAP: Record<string, string> = {
  none: '0px', xs: '4px', sm: '8px', md: '16px', lg: '24px', xl: '32px', '2xl': '48px', '3xl': '64px',
};
const SHADOW_CSS_MAP: Record<string, string> = {
  sm: '0 1px 2px 0 rgb(0 0 0 / 0.05)',
  md: '0 4px 6px -1px rgb(0 0 0 / 0.1)',
  lg: '0 10px 15px -3px rgb(0 0 0 / 0.1)',
  xl: '0 20px 25px -5px rgb(0 0 0 / 0.1)',
  '2xl': '0 25px 50px -12px rgb(0 0 0 / 0.25)',
  inner: 'inset 0 2px 4px 0 rgb(0 0 0 / 0.05)',
};

/**
 * Generate responsive container-query CSS for a block's `responsive` overrides.
 * Uses @container queries (not @media) so they respond to the preview
 * container's width, not the browser viewport — making them work in the
 * editor's fixed-width device preview.
 * The `scopeSelector` (e.g. `[data-bs="abc123"]`) is used to scope the rules.
 */
export function responsiveStyleToCss(style: BlockStyle | undefined, scopeSelector: string): string | undefined {
  if (!style?.responsive) return undefined;
  const parts: string[] = [];
  for (const [breakpoint, override] of Object.entries(style.responsive)) {
    if (!override) continue;
    const decls = styleToCssDeclarations(override);
    if (decls.length === 0) continue;
    const query = breakpointQuery(breakpoint);
    if (!query) continue;
    parts.push(`${query} {\n  ${scopeSelector} {\n    ${decls.join(';\n    ')};\n  }\n}`);
  }
  return parts.length > 0 ? parts.join('\n') : undefined;
}

function breakpointQuery(breakpoint: string): string | undefined {
  switch (breakpoint) {
    case 'desktop':
      return `@container (min-width: ${BREAKPOINTS.tablet + 1}px)`;
    case 'tablet':
      return `@container (min-width: ${BREAKPOINTS.mobile + 1}px) and (max-width: ${BREAKPOINTS.tablet}px)`;
    case 'mobile':
      return `@container (max-width: ${BREAKPOINTS.mobile}px)`;
    default:
      return undefined;
  }
}
