<script lang="ts">
  import type { GuestHeroTextProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { ArrowRight } from 'lucide-svelte';

  interface Props {
    props: GuestHeroTextProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-hero-text' }: Props = $props();

  const eyebrow = $derived(props.eyebrow ?? 'Your Bridge to Global Opportunities');
  const headlinePrefix = $derived(props.headline_prefix ?? 'Work. Study. Live.');
  const headlineAccent = $derived(props.headline_accent ?? 'Anywhere in the World.');
  const subhead = $derived(
    props.subhead ??
      'JapaConnect is your all-in-one platform for migration guidance, job opportunities, scholarships, business insights, and everything you need to succeed globally.'
  );
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
  </div>
</div>
