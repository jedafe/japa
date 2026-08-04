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
      'entity_feature', 'accordion', 'cta_banner', 'video', 'icon', 'content',
      'quote', 'spacer_divider', 'countdown', 'coming_soon', 'form_embed',
      'timeline', 'map', 'table', 'advertisement', 'live_rate_ticker',
      'tabs', 'media',
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
          stats: [
            { label: 'Members', value: '50K+' },
            { label: 'Destinations', value: '12+' },
            { label: 'Stories shared', value: '10K+' },
          ],
        },
      },
      {
        type: 'card_grid',
        label: 'Feature Highlights',
        props: {
          cards: [
            { icon: 'MessageCircle', title: 'Community Discussions', description: 'Ask questions and share experiences with {{country_adjective}} diaspora.', cta: { label: 'Join discussions', href: '/forum', variant: 'link' } },
            { icon: 'GraduationCap', title: 'University Comparisons', description: 'Compare universities and programs for your destination.', cta: { label: 'Compare universities', href: '/universities', variant: 'link' } },
            { icon: 'Briefcase', title: 'Jobs & Opportunities', description: 'Find jobs with visa sponsorship in {{country}}.', cta: { label: 'Browse jobs', href: '/jobs', variant: 'link' } },
            { icon: 'TrendingUp', title: 'Remittance Rates', description: 'Track live remittance rates across corridors.', cta: { label: 'Check rates', href: '/tools/remittance-tracker', variant: 'link' } },
            { icon: 'Calculator', title: 'Cost of Living', description: 'Compare living costs across destination cities.', cta: { label: 'Compare costs', href: '/tools/cost-of-living', variant: 'link' } },
            { icon: 'FileCheck', title: 'Visa Checklist', description: 'Generate your visa document checklist by destination.', cta: { label: 'Get checklist', href: '/tools/visa-checklist', variant: 'link' } },
          ],
          card_variant: 'feature', layout: 'grid', columns: 3,
        },
      },
      {
        type: 'entity_collection',
        label: 'Destinations',
        props: { entity_type: 'space', query: { limit: 6, geo: true, filters: { hide_if_empty: 'true' } }, layout: 'grid', card_variant: 'compact', columns: 3 },
      },
      {
        type: 'entity_collection',
        label: 'Trending Discussions',
        props: { entity_type: 'thread', query: { limit: 5, geo: true, filters: { hide_if_empty: 'true' } }, layout: 'list', card_variant: 'minimal', columns: 1 },
      },
      {
        type: 'timeline',
        label: 'Japa Journey',
        props: {
          layout: 'horizontal',
          steps: [
            { icon: 'Plane', title: 'Plan', description: 'Research destinations and prepare your documents.' },
            { icon: 'FileCheck', title: 'Apply', description: 'Submit visa, school, or job applications.' },
            { icon: 'Plane', title: 'Move', description: 'Travel to your new country and settle in.' },
            { icon: 'Home', title: 'Settle', description: 'Build your new life abroad with community support.' },
          ],
        },
      },
      {
        type: 'entity_collection',
        label: 'Latest Blog Posts',
        props: { entity_type: 'blog', query: { limit: 6, filters: { hide_if_empty: 'true' } }, layout: 'grid', card_variant: 'feature', columns: 3 },
      },
      {
        type: 'entity_collection',
        label: 'Featured Jobs',
        props: { entity_type: 'job', query: { limit: 6, geo: true, filters: { hide_if_empty: 'true' } }, layout: 'grid', card_variant: 'compact', columns: 3 },
      },
      {
        type: 'map',
        label: 'Diaspora Map',
        props: {
          mode: 'static',
          geo_source: 'trending_spaces',
          highlight_visitor_country: true,
          pins: [
            { country: 'NG', label: 'Nigeria', href: '/explore' },
            { country: 'GB', label: 'United Kingdom', href: '/explore?geo=GB' },
            { country: 'US', label: 'United States', href: '/explore?geo=US' },
            { country: 'CA', label: 'Canada', href: '/explore?geo=CA' },
            { country: 'DE', label: 'Germany', href: '/explore?geo=DE' },
            { country: 'AU', label: 'Australia', href: '/explore?geo=AU' },
            { country: 'IE', label: 'Ireland', href: '/explore?geo=IE' },
            { country: 'ZA', label: 'South Africa', href: '/explore?geo=ZA' },
            { country: 'AE', label: 'UAE', href: '/explore?geo=AE' },
          ],
        },
      },
      {
        type: 'accordion',
        label: 'FAQ',
        props: {
          items: [
            { title: 'What is JapaConnect?', content_blocks: [{ id: 'faq-1', type: 'content', visible: true, props: { html: '<p>JapaConnect is the community hub for the Nigerian diaspora. Share your journey, ask questions, compare destinations, and connect with fellow Nigerians abroad.</p>', prose_style: 'default' } }] },
            { title: 'Is it free to join?', content_blocks: [{ id: 'faq-2', type: 'content', visible: true, props: { html: '<p>Yes! JapaConnect is free to join. Create an account to participate in discussions, save content, and access our tools.</p>', prose_style: 'default' } }] },
            { title: 'How does geo-aware content work?', content_blocks: [{ id: 'faq-3', type: 'content', visible: true, props: { html: '<p>We detect your country and tailor content to your diaspora community. You can also manually set your country using the geo selector.</p>', prose_style: 'default' } }] },
            { title: 'What tools are available?', content_blocks: [{ id: 'faq-4', type: 'content', visible: true, props: { html: '<p>Visa checklist generator, japa readiness assessment, cost of living calculator, scholarship finder, remittance rate tracker, and more.</p>', prose_style: 'default' } }] },
          ],
          preset: 'faq', layout: 'accordion', searchable: true,
        },
      },
      {
        type: 'form_embed',
        label: 'Newsletter Signup',
        props: {
          preset: 'newsletter',
          title: 'Stay in the loop',
          subtitle: 'Get the latest diaspora stories and guides in your inbox.',
          fields: [
            { name: 'email', label: 'Email', type: 'email', required: true, placeholder: 'you@example.com' },
          ],
          submit_endpoint: '/newsletter/subscribe',
          submit_label: 'Subscribe',
          success_message: 'You\'re subscribed! Check your inbox for a confirmation.',
          error_message: 'Something went wrong. Please try again.',
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
      {
        type: 'spacer_divider',
        label: 'Footer Spacer',
        props: { height: 64, divider_style: 'gradient' },
      },
    ],
  },
};
