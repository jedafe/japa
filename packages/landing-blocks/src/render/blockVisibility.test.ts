// packages/landing-blocks/src/render/blockVisibility.test.ts
import { describe, it, expect } from 'vitest';
import { isBlockEmpty, isBlockGeoVisible, isBlockVisible } from './blockVisibility';
import type { Block, WidgetBlockProps } from '../types';

function makeBlock(overrides: Partial<Block> & { type: Block['type']; props?: Record<string, unknown> }): Block {
  return {
    id: 'b1',
    visible: true,
    type: overrides.type,
    props: overrides.props ?? {},
    ...overrides,
  } as Block;
}

describe('isBlockEmpty', () => {
  it('returns true for a block with no content props', () => {
    const block = makeBlock({ type: 'hero', props: {} });
    expect(isBlockEmpty(block)).toBe(true);
  });

  it('returns false when headline is present', () => {
    const block = makeBlock({ type: 'hero', props: { headline: 'Hello' } });
    expect(isBlockEmpty(block)).toBe(false);
  });

  it('returns false when cards array is present', () => {
    const block = makeBlock({ type: 'card_grid', props: { cards: [{ title: 'A' }] } });
    expect(isBlockEmpty(block)).toBe(false);
  });

  it('returns false for a media object', () => {
    const block = makeBlock({ type: 'media', props: { media: { url: 'https://example.com/a.jpg', media_id: 'm1' } } });
    expect(isBlockEmpty(block)).toBe(false);
  });

  it('returns true when headline is an empty string', () => {
    const block = makeBlock({ type: 'hero', props: { headline: '   ' } });
    expect(isBlockEmpty(block)).toBe(true);
  });

  it('returns false for a widget with resolved data', () => {
    const props: WidgetBlockProps = { widget_id: 'w1', layout: 'grid', columns: 3 };
    const block = makeBlock({ type: 'widget', props: { ...props, _resolved: { widget_type: 'list', data: [{ id: '1' }] } } as unknown as Record<string, unknown> });
    expect(isBlockEmpty(block)).toBe(false);
  });

  it('returns true for a widget with empty resolved data', () => {
    const props: WidgetBlockProps = { widget_id: 'w1', layout: 'grid', columns: 3 };
    const block = makeBlock({ type: 'widget', props: { ...props, _resolved: { widget_type: 'list', data: [] } } as unknown as Record<string, unknown> });
    expect(isBlockEmpty(block)).toBe(true);
  });

  it('returns false for an unresolved dynamic block so it can fetch and report', () => {
    const block = makeBlock({ type: 'entity_collection', props: { entity_type: 'blog', query: {}, columns: 3, card_variant: 'feature', layout: 'grid' } });
    expect(isBlockEmpty(block)).toBe(false);
  });

  it('returns true for an entity collection with empty prefetched items', () => {
    const block = makeBlock({ type: 'entity_collection', props: { _items: [] } });
    expect(isBlockEmpty(block)).toBe(true);
  });
});

describe('isBlockGeoVisible', () => {
  it('returns true when no geo_countries are set', () => {
    expect(isBlockGeoVisible(undefined, 'US')).toBe(true);
    expect(isBlockGeoVisible({}, 'US')).toBe(true);
  });

  it('hides when current country is not in the list', () => {
    expect(isBlockGeoVisible({ geo_countries: ['CA', 'GB'] }, 'US')).toBe(false);
  });

  it('shows when current country is in the list', () => {
    expect(isBlockGeoVisible({ geo_countries: ['CA', 'US'] }, 'US')).toBe(true);
  });

  it('hides for global preview when geo_countries are set', () => {
    expect(isBlockGeoVisible({ geo_countries: ['US'] }, 'global')).toBe(false);
  });
});

describe('isBlockVisible', () => {
  it('returns false when block.visible is false', () => {
    const block = makeBlock({ type: 'hero', props: { headline: 'Hello' }, visible: false });
    expect(isBlockVisible(block, 'US')).toBe(false);
  });

  it('returns false when geo rule does not match', () => {
    const block = makeBlock({ type: 'hero', props: { headline: 'Hello' }, visibility_rules: { geo_countries: ['CA'] } });
    expect(isBlockVisible(block, 'US')).toBe(false);
  });

  it('returns true when geo rule matches', () => {
    const block = makeBlock({ type: 'hero', props: { headline: 'Hello' }, visibility_rules: { geo_countries: ['US'] } });
    expect(isBlockVisible(block, 'US')).toBe(true);
  });

  it('hides when hide_if_empty is true and block is empty', () => {
    const block = makeBlock({ type: 'hero', props: {}, visibility_rules: { hide_if_empty: true } });
    expect(isBlockVisible(block, 'US')).toBe(false);
  });

  it('shows when hide_if_empty is true and block has content', () => {
    const block = makeBlock({ type: 'hero', props: { headline: 'Hello' }, visibility_rules: { hide_if_empty: true } });
    expect(isBlockVisible(block, 'US')).toBe(true);
  });
});
