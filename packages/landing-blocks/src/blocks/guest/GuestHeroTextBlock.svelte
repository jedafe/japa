<script lang="ts">
  import { siteName } from '../../site';
  import type { GuestHeroTextProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { ArrowRight } from 'lucide-svelte';
  import GuestChildren from './GuestChildren.svelte';

  interface Props {
    props: GuestHeroTextProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-hero-text' }: Props = $props();

  const eyebrow = $derived(props.eyebrow ?? 'Your Bridge to Global Opportunities');
  const headlinePrefix = $derived(props.headline_prefix ?? 'Work. Study. Thrive.');
  const headlineAccent = $derived(props.headline_accent ?? 'Anywhere in the World.');
  const subhead = $derived(
    props.subhead ??
      `${siteName()} is your all-in-one platform for migration guidance, job opportunities, scholarships, business insights, and everything you need to succeed globally.`
  );
  // CTA children (guest_hero_cta_item blocks) take precedence over the legacy
  // ctas[] prop so each action is editable in the page builder tree.
  const ctaChildren = $derived(props.children && props.children.length > 0 ? props.children : undefined);
  const ctas = $derived(
    props.ctas && props.ctas.length > 0
      ? props.ctas
      : [
          { label: 'Get Started Free', href: '/signup', variant: 'primary' as const, icon: 'ArrowRight' },
          { label: 'Explore Opportunities', href: '/explore', variant: 'ghost' as const },
        ]
  );
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="hero-text" {id}>
  <span class="hero-eyebrow">{eyebrow}</span>
  <h1 class="hero-headline">
    {headlinePrefix}
    <span class="hero-headline-accent">{headlineAccent}</span>
  </h1>
  <p class="hero-subhead">{subhead}</p>
  <div class="hero-actions">
    {#if ctaChildren}
      <GuestChildren blocks={ctaChildren} {fetchCtx} {geoCountry} tag="div" class="hero-action-item" />
    {:else}
      {#each ctas as cta, i (cta.href + cta.label)}
        <a
          href={cta.href}
          class="hero-btn"
          class:hero-btn-primary={cta.variant === 'primary'}
          class:hero-btn-secondary={cta.variant !== 'primary'}
        >
          {cta.label}
          {#if cta.icon}
            {#if cta.icon === 'ArrowRight'}
              <ArrowRight size={18} />
            {:else}
              <span class="hero-btn-icon-label">{cta.icon}</span>
            {/if}
          {:else if i === 0}
            <ArrowRight size={18} />
          {/if}
        </a>
      {/each}
    {/if}
  </div>
</div>

<style>
  .hero-text {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }
</style>
