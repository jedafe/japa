<script lang="ts">
  import type { GuestFooterProps } from '../../types';
  import { Globe } from 'lucide-svelte';
  import { getLucideIcon } from './guestIconMap';

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
      dark: '/logos/homepage-logo-dark.png',
    }
  );

  const tagline = $derived(
    props.tagline ??
      'Your trusted partner in navigating global opportunities and building a better future.'
  );

  const socials = $derived(props.socials ?? []);
  const columns = $derived(props.columns ?? []);
  const copyright = $derived(
    props.copyright ?? `© ${new Date().getFullYear()} JapaConnect. All rights reserved.`
  );
  const bottomLinks = $derived(props.bottom_links ?? []);
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<footer class="guest-footer-block" {id} aria-labelledby="guest-footer-heading-{id}">
  <h2 id="guest-footer-heading-{id}" class="sr-only">Footer</h2>
  <div class="guest-footer-inner">
    <div class="guest-footer-brand">
      <img src={logo.light} alt="JapaConnect" class="footer-logo footer-logo-light" />
      <p class="footer-tagline">{tagline}</p>
      {#if socials.length > 0}
        <div class="footer-socials">
          {#each socials as social (social.id ?? social.url)}
            {@const Icon = getLucideIcon(social.icon)}
            <a href={social.url} aria-label={social.label} target="_blank" rel="noopener noreferrer">
              {#if Icon}
                <Icon size={18} />
              {:else}
                <Globe size={18} />
              {/if}
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
            {#each col.links as link (link.url)}
              <li>
                <a
                  href={link.url}
                  target={link.target === '_blank' ? '_blank' : undefined}
                  rel={link.target === '_blank' ? 'noopener noreferrer' : undefined}
                >
                  {link.label}
                </a>
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
        {#each bottomLinks as link (link.url)}
          <a
            href={link.url}
            target={link.target === '_blank' ? '_blank' : undefined}
            rel={link.target === '_blank' ? 'noopener noreferrer' : undefined}
          >
            {link.label}
          </a>
        {/each}
      </div>
    {/if}
  </div>
</footer>
