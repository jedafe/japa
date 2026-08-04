import { describe, it, expect } from 'vitest';
import { resolveString, resolveGeoCopy } from './resolveGeoCopy';
import { getGeoTokens } from './copyRegistry';
import type { Block, GeoContext } from '../types';

describe('resolveString', () => {
  const ngTokens = getGeoTokens('NG', 'en');
  const gbTokens = getGeoTokens('GB', 'en');
  const globalTokens = getGeoTokens('global', 'en');

  it('replaces {{country}} with the country name', () => {
    expect(resolveString('Your journey from {{country}} starts here', ngTokens))
      .toBe('Your journey from Nigeria starts here');
  });

  it('replaces {{country}} for UK visitor', () => {
    expect(resolveString('Settling in {{country}}?', gbTokens))
      .toBe('Settling in the UK?');
  });

  it('replaces {{diaspora_phrase}}', () => {
    expect(resolveString('Join {{diaspora_phrase}}', gbTokens))
      .toBe('Join Nigerians in the UK');
  });

  it('replaces {{destination}} alias', () => {
    expect(resolveString('Explore content for {{destination}}', gbTokens))
      .toBe('Explore content for Nigeria');
  });

  it('uses fallback in {{token|fallback}} syntax for global', () => {
    expect(resolveString('Settling in {{country|your country}}', globalTokens))
      .toBe('Settling in your country');
  });

  it('uses actual value when token resolves for {{token|fallback}}', () => {
    expect(resolveString('Settling in {{country|your country}}', gbTokens))
      .toBe('Settling in the UK');
  });

  it('leaves strings without placeholders unchanged', () => {
    expect(resolveString('No placeholders here', ngTokens))
      .toBe('No placeholders here');
  });

  it('handles multiple tokens in one string', () => {
    expect(resolveString('{{country_adjective}} diaspora in {{continent}}', gbTokens))
      .toBe('British diaspora in Europe');
  });
});

describe('resolveGeoCopy', () => {
  const ctx: GeoContext = {
    contentCountry: 'GB',
    tokens: getGeoTokens('GB', 'en'),
    locale: 'en',
  };

  it('resolves placeholders in hero block props', () => {
    const blocks: Block[] = [{
      id: '1', type: 'hero', visible: true,
      props: { headline: 'Settling in {{country}}?', subhead: 'Join {{diaspora_phrase}}' },
    }];
    const resolved = resolveGeoCopy(blocks, ctx);
    expect((resolved[0] as any).props.headline).toBe('Settling in the UK?');
    expect((resolved[0] as any).props.subhead).toBe('Join Nigerians in the UK');
  });

  it('does not mutate the original blocks', () => {
    const blocks: Block[] = [{
      id: '1', type: 'hero', visible: true,
      props: { headline: 'Settling in {{country}}?' },
    }];
    resolveGeoCopy(blocks, ctx);
    expect((blocks[0] as any).props.headline).toBe('Settling in {{country}}?');
  });

  it('resolves nested strings in card arrays', () => {
    const blocks: Block[] = [{
      id: '1', type: 'card_grid', visible: true,
      props: {
        cards: [{ title: 'For {{country_adjective}} students' }],
        card_variant: 'feature', layout: 'grid', columns: 3,
      },
    }];
    const resolved = resolveGeoCopy(blocks, ctx);
    expect((resolved[0] as any).props.cards[0].title).toBe('For British students');
  });
});
