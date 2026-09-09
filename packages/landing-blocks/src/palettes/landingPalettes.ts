// packages/landing-blocks/src/palettes/landingPalettes.ts
import type { Block } from '../types';

export interface PaletteDefinition {
  label: string;
  allowedBlocks: Block['type'][];
  defaultSeed: Array<{
    type: Block['type'];
    label: string;
    props: Record<string, unknown>;
    motion?: Block['motion'];
    style?: Block['style'];
    visibility_rules?: Block['visibility_rules'];
  }>;
}

export const landingPalettes: Record<string, PaletteDefinition> = {
  homepage: {
    label: 'Homepage',
    allowedBlocks: [
      'heading', 'cta_button', 'hero', 'carousel', 'card_grid', 'media_grid', 'entity_collection',
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
          layout: 'split',
          headline: 'Your japa journey starts here.',
          highlight: 'starts here.',
          tagline: 'Move with clarity. Settle with confidence.',
          subhead: 'Practical guides, real opportunities, trusted services, and a community that gets the journey abroad.',
          ctas: [
            { label: 'Start your journey', href: '/signup', variant: 'primary' },
            { label: 'Explore the community', href: '/explore', variant: 'outline' },
          ],
          slides: [
            { url: '/brand/kigali-airplane-window.jpg', alt: 'Airplane window view over a new city' },
            { url: '/brand/networking.jpg', alt: 'Two professionals connecting abroad' },
            { url: '/brand/opportunity.jpg', alt: 'Working on a laptop' },
            { url: '/brand/study-abroad.jpg', alt: 'Student in a university library' },
          ],
        },
        style: {
          background: { type: 'gradient', gradient: { from: '#f4f8ff', to: '#e8f5ee', angle: 135 } },
        },
        motion: { preset: 'fade-up', delay: 0, duration: 800 },
      },
      {
        type: 'card_grid',
        label: 'How JapaConnect helps',
        props: {
          card_variant: 'overlay',
          layout: 'grid',
          columns: 4,
          responsive_columns: { tablet: 2, mobile: 1 },
          cards: [
            {
              icon: 'GraduationCap',
              icon_background_color: '#2563eb',
              icon_color: '#ffffff',
              title: 'Find your route',
              description: 'Compare study, work, business, and family routes with practical guidance.',
              media: { media_id: 'seed-study', url: '/brand/study-abroad.jpg', alt: 'Student in a university library' },
              cta: { label: 'Learn More', href: '/universities', variant: 'outline' },
            },
            {
              icon: 'Briefcase',
              icon_background_color: '#16a34a',
              icon_color: '#ffffff',
              title: 'Find opportunities',
              description: 'Discover jobs, scholarships, mentors, and opportunities built for your next move.',
              media: { media_id: 'seed-work', url: '/brand/opportunity.jpg', alt: 'Working on a laptop' },
              cta: { label: 'Learn More', href: '/jobs', variant: 'outline' },
            },
            {
              icon: 'TrendingUp',
              icon_background_color: '#2563eb',
              icon_color: '#ffffff',
              title: 'Get trusted support',
              description: 'Access services, checklists, calculators, and people who have made the move.',
              media: { media_id: 'seed-business', url: '/brand/networking.jpg', alt: 'Business meeting' },
              cta: { label: 'Learn More', href: '/explore', variant: 'outline' },
            },
            {
              icon: 'Users',
              icon_background_color: '#16a34a',
              icon_color: '#ffffff',
              title: 'Build your new life',
              description: 'Join a diaspora community, ask better questions, and settle in with confidence.',
              media: { media_id: 'seed-live', url: '/brand/kigali-airplane-window.jpg', alt: 'View from an airplane window' },
              cta: { label: 'Learn More', href: '/explore', variant: 'outline' },
            },
          ],
        },
        style: {
          background: { type: 'gradient', gradient: { from: '#f4f8ff', to: '#e8f5ee', angle: 180 } },
        },
        motion: { preset: 'fade-up', delay: 150, stagger_children: true },
      },
      {
        type: 'heading',
        label: 'Destinations Heading',
        props: {
          text: 'Popular destinations',
          eyebrow: 'Explore',
          subheading: 'Discover communities in {{country}} and beyond.',
          level: 'h2', size: 'xl', align: 'left', accent: 'underline', accent_color: 'var(--color-primary)',
          max_width: 'xl',
          cta: { label: 'View all', href: '/explore' },
        },
      },
      {
        type: 'entity_collection',
        label: 'Destinations',
        props: { entity_type: 'space', query: { limit: 6, geo: true, filters: { hide_if_empty: 'true' } }, layout: 'grid', card_variant: 'compact', columns: 3 },
      },
      {
        type: 'cta_button',
        label: 'Explore All Destinations',
        props: { label: 'Explore all destinations', href: '/explore', variant: 'outline', size: 'lg', align: 'center', icon_right: 'ArrowRight' },
        motion: { preset: 'fade-up', delay: 100 },
      },
      {
        type: 'heading',
        label: 'Discussions Heading',
        props: {
          text: 'Trending community discussions',
          eyebrow: 'Community',
          level: 'h2', size: 'xl', align: 'left', accent: 'none',
          max_width: 'xl',
          cta: { label: 'Join the conversation', href: '/explore?sort=rising' },
        },
      },
      {
        type: 'entity_collection',
        label: 'Trending Discussions',
        props: { entity_type: 'post', query: { sort: 'rising', limit: 5, geo: true, filters: { hide_if_empty: 'true' } }, layout: 'list', card_variant: 'minimal', columns: 1 },
      },
      {
        type: 'heading',
        label: 'Journey Heading',
        props: {
          text: 'Your japa journey, step by step',
          eyebrow: 'The Process',
          subheading: 'From planning to settling in — we are with you at every stage.',
          level: 'h2', size: '2xl', align: 'center', accent: 'none',
          max_width: 'lg',
        },
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
        motion: { preset: 'fly-in-left', delay: 0, duration: 600 },
      },
      {
        type: 'heading',
        label: 'Blog Heading',
        props: {
          text: 'Latest stories & guides',
          eyebrow: 'Blog',
          level: 'h2', size: 'xl', align: 'left', accent: 'none',
          max_width: 'xl',
          cta: { label: 'Read more', href: '/blog' },
        },
      },
      {
        type: 'entity_collection',
        label: 'Latest Blog Posts',
        props: { entity_type: 'blog', query: { limit: 6, filters: { hide_if_empty: 'true' } }, layout: 'grid', card_variant: 'feature', columns: 3 },
      },
      {
        type: 'heading',
        label: 'Jobs Heading',
        props: {
          text: 'Jobs with visa sponsorship',
          eyebrow: 'Opportunities',
          subheading: 'Find roles in {{country}} that sponsor visas.',
          level: 'h2', size: 'xl', align: 'left', accent: 'none',
          max_width: 'xl',
          cta: { label: 'Browse all jobs', href: '/jobs' },
        },
      },
      {
        type: 'entity_collection',
        label: 'Featured Jobs',
        props: { entity_type: 'job', query: { limit: 6, geo: true, filters: { hide_if_empty: 'true' } }, layout: 'grid', card_variant: 'compact', columns: 3 },
      },
      {
        type: 'cta_button',
        label: 'Browse All Jobs',
        props: { label: 'Browse all jobs', href: '/jobs', variant: 'primary', size: 'lg', align: 'center', icon: 'Briefcase', icon_right: 'ArrowRight' },
        motion: { preset: 'fade-up', delay: 100 },
      },
      {
        type: 'heading',
        label: 'Map Heading',
        props: {
          text: 'Where our community is thriving',
          eyebrow: 'Diaspora Map',
          subheading: 'Explore {{country_adjective}} communities around the world.',
          level: 'h2', size: '2xl', align: 'center', accent: 'none',
          max_width: 'lg',
        },
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
            { title: 'What is JapaConnect?', content_blocks: [{ id: 'faq-1', type: 'content', visible: true, props: { html: '<p>JapaConnect is the community hub for the {{diaspora_phrase}}. Share your journey, ask questions, compare destinations, and connect with fellow diaspora members abroad.</p>', prose_style: 'default' } }] },
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
          submit_endpoint: '/api/v1/newsletter/subscribe',
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
          subhead: 'Create a free account and start sharing your journey, asking questions, and connecting with fellow diaspora members abroad.',
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
