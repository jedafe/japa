<script lang="ts">
  import type { GuestFooterProps } from '../../types';
  import { getLucideIcon } from './guestIconMap';
  import { resolveAssetUrl } from '../../assetBase';
  import SocialIcon from './SocialIcon.svelte';

  interface Props {
    props: GuestFooterProps;
    fetchCtx?: unknown;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-footer' }: Props = $props();

  const logo = $derived(
    props.logo ?? {
      light: '/logos/homepage-logo-light.png',
      dark: '/logos/homepage-logo-dark.png'
    }
  );

  const logoSrc = $derived(resolveAssetUrl(logo.dark ?? logo.light ?? ''));

  const tagline = $derived(
    props.tagline ??
      'Your trusted partner in navigating global opportunities and building a better future.'
  );

  const defaultSocials: NonNullable<GuestFooterProps['socials']> = [
    { id: 'facebook', icon: 'Facebook', url: 'https://facebook.com/japaconnect', label: 'Facebook' },
    { id: 'twitter', icon: 'Twitter', url: 'https://twitter.com/japaconnect', label: 'Twitter' },
    { id: 'instagram', icon: 'Instagram', url: 'https://instagram.com/japaconnect', label: 'Instagram' },
    { id: 'youtube', icon: 'Youtube', url: 'https://youtube.com/japaconnect', label: 'YouTube' },
    { id: 'linkedin', icon: 'Linkedin', url: 'https://linkedin.com/company/japaconnect', label: 'LinkedIn' },
  ];

  const defaultColumns: NonNullable<GuestFooterProps['columns']> = [
    {
      id: 'explore',
      title: 'Explore',
      links: [
        { label: 'Jobs Abroad', url: '/jobs' },
        { label: 'Study Abroad', url: '/scholarships' },
        { label: 'Visa & Relocation', url: '/guides' },
        { label: 'Business', url: '/jobs' },
        { label: 'Countries', url: '/compare' },
      ],
    },
    {
      id: 'resources',
      title: 'Resources',
      links: [
        { label: 'Blog', url: '/blog' },
        { label: 'Guides', url: '/guides' },
        { label: 'Tools', url: '/tools' },
        { label: 'Compare Countries', url: '/compare' },
        { label: 'Success Stories', url: '/stories' },
      ],
    },
    {
      id: 'company',
      title: 'Company',
      links: [
        { label: 'About Us', url: '/' },
        { label: 'How It Works', url: '/' },
        { label: 'Pricing', url: '/pricing' },
        { label: 'FAQs', url: '/' },
        { label: 'Contact Us', url: '/' },
      ],
    },
    {
      id: 'help',
      title: 'Need Help?',
      links: [
        { label: 'support@japaconnect.com', url: 'mailto:support@japaconnect.com', icon: 'Mail' },
        { label: '+1 (234) 567-8900', url: 'tel:+12345678900', icon: 'Phone' },
        { label: 'Mon - Fri: 9AM - 6PM (EST)', icon: 'Clock' },
      ],
    },
  ];

  const defaultBottomLinks: NonNullable<GuestFooterProps['bottom_links']> = [
    { label: 'Privacy Policy', url: '/' },
    { label: 'Terms of Service', url: '/' },
    { label: 'Cookies Policy', url: '/' },
  ];

  const socials = $derived(props.socials ?? defaultSocials);
  const columns = $derived(props.columns ?? defaultColumns);
  const copyright = $derived(
    props.copyright ?? `© ${new Date().getFullYear()} JapaConnect. All rights reserved.`
  );
  const bottomLinks = $derived(props.bottom_links ?? defaultBottomLinks);

  function isExternal(url: string) {
    return /^https?:\/\//.test(url) || /^\/\//.test(url);
  }

  function linkAttrs(url: string | undefined, target: string | undefined) {
    const shouldBlank = target === '_blank' || (!!url && isExternal(url));
    return {
      href: url ?? '',
      target: shouldBlank ? '_blank' : undefined,
      rel: shouldBlank ? 'noopener noreferrer' : undefined,
    };
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<footer class="guest-footer-block" {id} aria-labelledby="guest-footer-heading-{id}">
  <h2 id="guest-footer-heading-{id}" class="sr-only">Footer</h2>
  <div class="guest-footer-inner">
    <div class="guest-footer-brand">
      <img src={logoSrc} alt="JapaConnect" class="footer-logo footer-logo-dark" />
      <p class="footer-tagline">{tagline}</p>
      {#if socials.length > 0}
        <div class="footer-socials">
          {#each socials as social (social.id ?? social.url)}
            <a href={social.url} aria-label={social.label} target="_blank" rel="noopener noreferrer">
              <SocialIcon name={social.icon} size={18} />
            </a>
          {/each}
        </div>
      {/if}
    </div>

    <div class="guest-footer-columns">
      {#each columns as col (col.id ?? col.title)}
        <div class="footer-column">
          <h3 class="footer-column-title">{col.title}</h3>
          <ul class="footer-column-list">
            {#each col.links as link, i (`${link.url ?? ''}:${link.label}:${i}`)}
              {@const LinkIcon = getLucideIcon(link.icon)}
              <li>
                {#if link.url}
                  {@const attrs = linkAttrs(link.url, link.target)}
                  <a {...attrs}>
                    {#if LinkIcon}
                      <LinkIcon size={14} />
                    {/if}
                    <span>{link.label}</span>
                  </a>
                {:else}
                  <span class="footer-column-text">
                    {#if LinkIcon}
                      <LinkIcon size={14} />
                    {/if}
                    <span>{link.label}</span>
                  </span>
                {/if}
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    </div>
  </div>

  <div class="guest-footer-bottom">
    <p class="footer-copyright">{copyright}</p>
    {#if bottomLinks.length > 0}
      <div class="footer-legal">
        {#each bottomLinks as link (link.label)}
          {@const attrs = linkAttrs(link.url, link.target)}
          {#if link.url}
            <a {...attrs}>
              {link.label}
            </a>
          {:else}
            <span>{link.label}</span>
          {/if}
        {/each}
      </div>
    {/if}
  </div>
</footer>
