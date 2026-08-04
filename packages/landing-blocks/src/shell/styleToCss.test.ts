import { describe, it, expect } from 'vitest';
import { styleToCss } from './styleToCss';

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

  it('converts display flex with alignment', () => {
    const result = styleToCss({ layout: { display: 'flex', align_items: 'center', justify_content: 'between' } });
    expect(result.classes).toContain('flex');
    expect(result.classes).toContain('items-center');
    expect(result.classes).toContain('justify-between');
  });

  it('converts spacing padding preset', () => {
    const result = styleToCss({ spacing: { padding: 'lg' } });
    expect(result.classes).toContain('p-lg');
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
});
