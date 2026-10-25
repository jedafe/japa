<script lang="ts">
  import { siteName } from '../../site';
  import type { GuestFooterProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getLucideIcon } from './guestIconMap';
  import { resolveAssetUrl } from '../../assetBase';
  import SocialIcon from './SocialIcon.svelte';
  import GuestChildren from './GuestChildren.svelte';
  import defaultLogoLight from '../../assets/wordmark-light.webp';
  import defaultLogoDark from '../../assets/wordmark-dark.webp';

  interface Props {
    props: GuestFooterProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-footer' }: Props = $props();

  // Footer children are partitioned by type into the three footer areas; each
  // area falls back to its legacy props array when it has no child blocks.
  const socialChildren = $derived((props.children ?? []).filter((b) => b.type === 'guest_footer_social'));
  const columnChildren = $derived((props.children ?? []).filter((b) => b.type === 'guest_footer_column'));
  const bottomChildren = $derived((props.children ?? []).filter((b) => b.type === 'guest_footer_link'));

  // Bundled defaults: emitted to /_app/immutable with content hashes, so they
  // get immutable cache headers — the multi-MB /logos PNGs did not.
  const logo = $derived(
    props.logo ?? {
      light: defaultLogoLight,
      dark: defaultLogoDark
    }
  );

  const logoSrc = $derived(resolveAssetUrl(logo.dark ?? logo.light ?? ''));

  // Emit intrinsic dims only when they're actually known — the bundled default
  // (794x384) or explicit width/height on a custom logo. Emitting the
  // default's dims for a different image misstates the aspect ratio.
  const logoDims = $derived(
    props.logo
      ? props.logo.width && props.logo.height
        ? { width: props.logo.width, height: props.logo.height }
        : {}
      : { width: 794, height: 384 }
  );

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
        { label: 'support@japa.to', url: 'mailto:support@japa.to', icon: 'Mail' },
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
    props.copyright ?? `© ${new Date().getFullYear()} ${siteName()}. All rights reserved.`
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
      <img src={logoSrc} alt={siteName()} {...logoDims} class="footer-logo footer-logo-dark" />
      <p class="footer-tagline">{tagline}</p>
      {#if socialChildren.length > 0}
        <div class="footer-socials">
          <GuestChildren blocks={socialChildren} {fetchCtx} {geoCountry} />
        </div>
      {:else if socials.length > 0}
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
      {#if columnChildren.length > 0}
        <GuestChildren blocks={columnChildren} {fetchCtx} {geoCountry} />
      {:else}
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
      {/if}
    </div>
  </div>

  <div class="guest-footer-bottom">
    <p class="footer-copyright">{copyright}</p>
    {#if bottomChildren.length > 0}
      <div class="footer-legal">
        <GuestChildren blocks={bottomChildren} {fetchCtx} {geoCountry} />
      </div>
    {:else if bottomLinks.length > 0}
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
