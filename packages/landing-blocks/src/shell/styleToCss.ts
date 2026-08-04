import type { BlockStyle } from '../types';

export interface ResolvedStyle {
  classes: string;
  inline: Record<string, string>;
}

const RADIUS_MAP: Record<string, string> = {
  none: 'rounded-none', sm: 'rounded-sm', md: 'rounded-md', lg: 'rounded-lg',
  xl: 'rounded-xl', '2xl': 'rounded-2xl', '3xl': 'rounded-3xl', full: 'rounded-full',
};

const SHADOW_MAP: Record<string, string> = {
  none: 'shadow-none', sm: 'shadow-sm', md: 'shadow-md', lg: 'shadow-lg',
  xl: 'shadow-xl', '2xl': 'shadow-2xl', inner: 'shadow-inner',
};

const BLUR_MAP: Record<string, string> = { sm: '4px', md: '8px', lg: '12px', xl: '16px' };

const HEADING_SIZE_MAP: Record<string, string> = {
  xs: '0.75rem', sm: '0.875rem', md: '1rem', lg: '1.125rem', xl: '1.25rem',
  '2xl': '1.5rem', '3xl': '1.875rem', '4xl': '2.25rem', '5xl': '3rem',
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
    if (l.display === 'flex') classes.push('flex');
    if (l.display === 'grid') classes.push('grid');
    if (l.display === 'hidden') classes.push('hidden');
    if (l.flex_direction === 'column') classes.push('flex-col');
    if (l.flex_direction === 'row-reverse') classes.push('flex-row-reverse');
    if (l.align_items) classes.push(`items-${l.align_items}`);
    if (l.justify_content) {
      const j = l.justify_content === 'between' ? 'between' : l.justify_content === 'around' ? 'around' : l.justify_content === 'evenly' ? 'evenly' : l.justify_content;
      classes.push(`justify-${j}`);
    }
    if (l.gap && typeof l.gap === 'string') classes.push(`gap-${l.gap}`);
    if (typeof l.grid_template_columns === 'number') {
      classes.push('grid');
      inline['grid-template-columns'] = `repeat(${l.grid_template_columns}, 1fr)`;
    }
  }

  // Typography
  const t = style.typography;
  if (t) {
    if (t.text_align) classes.push(`text-${t.text_align}`);
    if (t.heading_family === 'serif') classes.push('font-serif');
    if (t.heading_family === 'mono') classes.push('font-mono');
    if (t.heading_weight) inline['--block-heading-weight'] = String(t.heading_weight);
    if (t.heading_size) {
      inline['--block-heading-size'] = typeof t.heading_size === 'string'
        ? HEADING_SIZE_MAP[t.heading_size] ?? '1.5rem'
        : `${t.heading_size}rem`;
    }
    if (t.heading_color) inline['--block-heading-color'] = t.heading_color;
    if (t.body_color) inline['--block-body-color'] = t.body_color;
  }

  // Color
  if (style.color?.accent) inline['--block-accent'] = style.color.accent;
  if (style.color?.text) inline['--block-text'] = style.color.text;

  // Border
  const b = style.border;
  if (b) {
    if (b.radius && typeof b.radius === 'string') classes.push(RADIUS_MAP[b.radius] ?? 'rounded-md');
    if (b.width && b.width > 0) {
      classes.push(`border${b.width === 1 ? '' : '-' + b.width}`);
      if (b.style && b.style !== 'none') classes.push(`border-${b.style}`);
      if (b.color) inline['border-color'] = b.color;
    }
  }

  // Spacing
  const s = style.spacing;
  if (s) {
    if (s.padding && typeof s.padding === 'string') classes.push(`p-${s.padding}`);
    if (s.margin && typeof s.margin === 'string') classes.push(`m-${s.margin}`);
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

  return { classes: classes.filter(Boolean).join(' '), inline };
}
