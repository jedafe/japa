// packages/landing-blocks/src/palettes/landingPalettes.ts
import type { Block } from '../types';

export interface PaletteDefinition {
  label: string;
  allowedBlocks: Block['type'][];
  defaultSeed: Array<{ type: Block['type']; label: string; props: Record<string, unknown> }>;
}

export const landingPalettes: Record<string, PaletteDefinition> = {
  homepage: {
    label: 'Homepage',
    allowedBlocks: [
      'hero', 'carousel', 'card_grid', 'media_grid', 'entity_collection',
      'entity_feature', 'accordion', 'cta_banner',
    ],
    defaultSeed: [
      {
        type: 'hero',
        label: 'Hero',
        props: {
          eyebrow: 'The Nigerian diaspora community',
          headline: 'Your journey from {{country}} starts here',
          subhead: 'Join {{diaspora_phrase}} sharing stories, comparing destinations, and navigating life abroad together.',
          ctas: [
            { label: 'Join the community', href: '/signup', variant: 'primary' },
            { label: 'Explore content', href: '/explore', variant: 'outline' },
          ],
        },
      },
      {
        type: 'media_grid',
        label: 'Partner Brands',
        props: { items: [], card_style: 'logo', lightbox: false, columns: 6 },
      },
      {
        type: 'card_grid',
        label: 'Feature Highlights',
        props: {
          cards: [
            { icon: 'MessageCircle', title: 'Community Discussions', description: 'Ask questions and share experiences with {{country_adjective}} diaspora.', cta: { label: 'Join discussions', href: '/forum', variant: 'link' } },
            { icon: 'GraduationCap', title: 'University Comparisons', description: 'Compare universities and programs for your destination.', cta: { label: 'Compare universities', href: '/universities', variant: 'link' } },
            { icon: 'Briefcase', title: 'Jobs & Opportunities', description: 'Find jobs with visa sponsorship in {{country}}.', cta: { label: 'Browse jobs', href: '/jobs', variant: 'link' } },
          ],
          card_variant: 'feature', layout: 'grid', columns: 3,
        },
      },
      {
        type: 'carousel',
        label: 'Tools Slider',
        props: {
          slides: [],
          autoplay: true, dots: true, arrows: true,
          transition: 'slide', interval: 5000, pause_on_hover: true,
        },
      },
      {
        type: 'entity_collection',
        label: 'Destinations',
        props: { entity_type: 'space', query: { limit: 6, geo: true }, layout: 'grid', card_variant: 'compact', columns: 3 },
      },
      {
        type: 'entity_collection',
        label: 'Trending Discussions',
        props: { entity_type: 'thread', query: { limit: 5, geo: true }, layout: 'list', card_variant: 'minimal', columns: 1 },
      },
      {
        type: 'entity_feature',
        label: 'Featured University',
        props: { entity_type: 'university', layout: 'split' },
      },
      {
        type: 'entity_collection',
        label: 'Latest Blog',
        props: { entity_type: 'blog', query: { limit: 6 }, layout: 'grid', card_variant: 'feature', columns: 3 },
      },
      {
        type: 'entity_collection',
        label: 'Featured Jobs',
        props: { entity_type: 'job', query: { limit: 6, geo: true }, layout: 'grid', card_variant: 'compact', columns: 3 },
      },
      {
        type: 'accordion',
        label: 'FAQ',
        props: {
          items: [
            { title: 'What is JapaConnect?', content_blocks: [] },
            { title: 'Is it free to join?', content_blocks: [] },
            { title: 'How does geo-aware content work?', content_blocks: [] },
          ],
          preset: 'faq', layout: 'accordion', searchable: true,
        },
      },
      {
        type: 'cta_banner',
        label: 'Join CTA',
        props: {
          headline: 'Ready to join {{diaspora_phrase}}?',
          subhead: 'Create a free account and start sharing your journey, asking questions, and connecting with fellow Nigerians abroad.',
          ctas: [{ label: 'Create your account', href: '/signup', variant: 'primary' }],
          alignment: 'center',
        },
      },
    ],
  },
};
