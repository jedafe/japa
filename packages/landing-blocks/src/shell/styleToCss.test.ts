import { describe, it, expect } from 'vitest';
import { styleToCss, responsiveStyleToCss } from './styleToCss';

describe('styleToCss', () => {
  it('returns empty for undefined style', () => {
    const result = styleToCss(undefined);
    expect(result.classes).toBe('');
    expect(result.inline).toEqual({});
  });

  it('converts position to Tailwind classes', () => {
    const result = styleToCss({ layout: { position: 'sticky', top: 80, z_index: 20 } });
    expect(result.classes).toContain('sticky');
    expect(result.inline['top']).toBe('80px');
    expect(result.inline['z-index']).toBe('20');
  });

  it('converts display flex with alignment and gap', () => {
    const result = styleToCss({ layout: { display: 'flex', align_items: 'center', justify_content: 'between', gap: 'lg' } });
    expect(result.classes).toContain('flex');
    expect(result.classes).toContain('items-center');
    expect(result.classes).toContain('justify-between');
    expect(result.inline['gap']).toBe('24px');
  });

  it('converts spacing padding/margin presets to inline sizes', () => {
    const result = styleToCss({ spacing: { padding: 'lg', margin: 'xl' } });
    expect(result.inline['padding']).toBe('24px');
    expect(result.inline['margin']).toBe('32px');
  });

  it('converts border radius', () => {
    const result = styleToCss({ border: { radius: 'xl' } });
    expect(result.classes).toContain('rounded-xl');
  });

  it('converts shadow', () => {
    const result = styleToCss({ effects: { shadow: 'xl' } });
    expect(result.classes).toContain('shadow-xl');
  });

  it('converts typography text align', () => {
    const result = styleToCss({ typography: { text_align: 'center' } });
    expect(result.classes).toContain('text-center');
  });

  it('puts custom hex colors in inline styles', () => {
    const result = styleToCss({ color: { accent: '#ff5500' } });
    expect(result.inline['--block-accent']).toBe('#ff5500');
  });

  it('converts transform to inline styles', () => {
    const result = styleToCss({ transform: { scale: 1.5, rotate: 45 } });
    expect(result.inline['transform']).toContain('scale(1.5)');
    expect(result.inline['transform']).toContain('rotate(45deg)');
  });

  it('converts backdrop blur', () => {
    const result = styleToCss({ effects: { backdrop_blur: 'lg' } });
    expect(result.inline['backdrop-filter']).toContain('blur');
  });

  it('applies side-specific borders', () => {
    const result = styleToCss({ border: { width: 4, style: 'dashed', color: '#ff0000', sides: 'top' } });
    expect(result.inline['border-top-width'] || result.inline['border-top']).toBeTruthy();
    expect(result.inline['border-bottom-width'] || result.inline['border-bottom']).toBeFalsy();
  });

  it('applies min-height, height, max-width, order, and flex-wrap', () => {
    const result = styleToCss({
      layout: { min_height: 300, height: 'full', max_width: 900, order: 2, flex_wrap: 'wrap' },
    });
    expect(result.inline['min-height']).toBe('300px');
    expect(result.inline['height']).toBe('100%');
    expect(result.inline['max-width']).toBe('900px');
    expect(result.inline['order']).toBe('2');
    expect(result.inline['flex-wrap']).toBe('wrap');
  });

  it('applies grid-template rows and columns', () => {
    const result = styleToCss({
      layout: { grid_template_columns: '1fr 2fr', grid_template_rows: 3 },
    });
    expect(result.inline['grid-template-columns']).toBe('1fr 2fr');
    expect(result.inline['grid-template-rows']).toBe('repeat(3, 1fr)');
  });

  it('sets theme color variables from block colors', () => {
    const result = styleToCss({
      color: { text: '#ff0000', heading: '#00ff00', accent: '#0000ff' },
      typography: { heading_color: '#aa00aa', body_color: '#00aaaa' },
    });
    expect(result.inline['--color-text']).toBe('#ff0000');
    expect(result.inline['--color-heading']).toBe('#aa00aa');
    expect(result.inline['--color-text-subtle']).toBe('#00aaaa');
    expect(result.inline['--color-primary']).toBe('#0000ff');
  });

  it('sets heading family variable', () => {
    const result = styleToCss({ typography: { heading_family: 'mono' } });
    expect(result.inline['--block-heading-family']).toContain('monospace');
  });

  it('converts transition config to transition style', () => {
    const result = styleToCss({ transition: { property: 'colors', duration: 250, easing: 'ease-in', delay: 50 } });
    expect(result.inline['transition']).toContain('color, background-color, border-color');
    expect(result.inline['transition']).toContain('250ms ease-in 50ms');
  });

  it('maps all border sides and styles', () => {
    const all = styleToCss({ border: { width: 2, style: 'dashed', color: '#0000ff' } });
    expect(all.inline['border']).toBe('2px dashed #0000ff');

    const top = styleToCss({ border: { width: 4, style: 'solid', color: '#ff0000', sides: 'top' } });
    expect(top.inline['border-top']).toBe('4px solid #ff0000');

    const x = styleToCss({ border: { width: 3, style: 'dotted', color: '#00ff00', sides: 'x' } });
    expect(x.inline['border-left']).toBe('3px dotted #00ff00');
    expect(x.inline['border-right']).toBe('3px dotted #00ff00');
    expect(x.inline['border-top']).toBeUndefined();

    const y = styleToCss({ border: { width: 1, style: 'double', color: '#000000', sides: 'y' } });
    expect(y.inline['border-top']).toBe('1px double #000000');
    expect(y.inline['border-bottom']).toBe('1px double #000000');

    const none = styleToCss({ border: { width: 4, style: 'none' } });
    expect(none.inline['border']).toBe('4px none currentColor');
  });

  it('maps typography heading and body fields', () => {
    const result = styleToCss({
      typography: {
        heading_size: '2xl',
        heading_weight: 700,
        heading_family: 'sans',
        heading_color: '#112233',
        body_size: 'sm',
        body_weight: 400,
        body_family: 'serif',
        body_color: '#445566',
        text_align: 'right',
      },
    });
    expect(result.classes).toContain('text-right');
    expect(result.inline['--block-heading-size']).toBe('1.5rem');
    expect(result.inline['--block-heading-weight']).toBe('700');
    expect(result.inline['--block-heading-family']).toContain('ui-sans');
    expect(result.inline['--block-heading-color']).toBe('#112233');
    expect(result.inline['--block-body-size']).toBe('0.875rem');
    expect(result.inline['--block-body-weight']).toBe('400');
    expect(result.inline['--block-body-family']).toContain('ui-serif');
    expect(result.inline['--block-body-color']).toBe('#445566');
    expect(result.inline['--color-text-subtle']).toBe('#445566');
  });

  it('maps layout width and display variants', () => {
    expect(styleToCss({ layout: { width: 'fit' } }).inline['width']).toBe('fit-content');
    expect(styleToCss({ layout: { width: 500 } }).inline['width']).toBe('500px');
    expect(styleToCss({ layout: { width: 'full' } }).classes).toContain('w-full');
    expect(styleToCss({ layout: { width: 'contained' } }).classes).toContain('max-w-5xl');
    expect(styleToCss({ layout: { display: 'grid' } }).classes).toContain('grid');
    expect(styleToCss({ layout: { display: 'hidden' } }).classes).toContain('hidden');
  });

  it('maps effects and transform fields', () => {
    const fx = styleToCss({
      effects: { shadow: '2xl', opacity: 65, backdrop_blur: 'md', overflow: 'hidden' },
    });
    expect(fx.classes).toContain('shadow-2xl');
    expect(fx.inline['opacity']).toBe('0.65');
    expect(fx.inline['backdrop-filter']).toContain('blur(8px)');
    expect(fx.classes).toContain('overflow-hidden');

    const tx = styleToCss({
      transform: { scale: 120, rotate: -15, translate_x: 30, translate_y: -10 },
    });
    expect(tx.inline['transform']).toContain('scale(120)');
    expect(tx.inline['transform']).toContain('rotate(-15deg)');
    expect(tx.inline['transform']).toContain('translateX(30px)');
    expect(tx.inline['transform']).toContain('translateY(-10px)');
  });

  it('maps all transition properties', () => {
    expect(styleToCss({ transition: { property: 'all' } }).inline['transition']).toContain('all');
    expect(styleToCss({ transition: { property: 'opacity' } }).inline['transition']).toContain('opacity');
    expect(styleToCss({ transition: { property: 'transform' } }).inline['transition']).toContain('transform');
    expect(styleToCss({ transition: { property: 'shadow' } }).inline['transition']).toContain('box-shadow');
    expect(styleToCss({ transition: { property: 'none' } }).inline['transition']).toBeUndefined();
  });

  it('maps color group to theme variables', () => {
    const result = styleToCss({
      color: { text: '#123456', heading: '#abcdef', accent: '#ff00ff' },
    });
    expect(result.inline['--color-text']).toBe('#123456');
    expect(result.inline['--color-heading']).toBe('#abcdef');
    expect(result.inline['--color-primary']).toBe('#ff00ff');
  });
});

describe('responsiveStyleToCss', () => {
  it('emits container queries per breakpoint', () => {
    const css = responsiveStyleToCss(
      {
        responsive: {
          tablet: { layout: { width: 'full' } },
          mobile: { spacing: { padding: 'md' } },
        },
      },
      '[data-bs="abc"]'
    );
    expect(css).toContain('@container (min-width: 769px) and (max-width: 1024px)');
    expect(css).toContain('@container (max-width: 768px)');
    expect(css).toContain('[data-bs="abc"]');
    expect(css).toContain('width: 100%');
    expect(css).toContain('padding: 16px');
  });

  it('emits desktop override query', () => {
    const css = responsiveStyleToCss(
      { responsive: { desktop: { border: { width: 2, style: 'solid', color: '#000000' } } } },
      '[data-bs="abc"]'
    );
    expect(css).toContain('@container (min-width: 1025px)');
  });
});
