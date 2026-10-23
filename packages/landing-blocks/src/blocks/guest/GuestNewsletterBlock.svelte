<script lang="ts">
  import type { GuestNewsletterProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { Plane, ArrowRight } from 'lucide-svelte';
  import GuestChildren from './GuestChildren.svelte';

  interface Props {
    props: GuestNewsletterProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-newsletter' }: Props = $props();

  let email = $state('');
  let newsletterStatus = $state<'idle' | 'loading' | 'success' | 'error'>('idle');
  let newsletterMessage = $state('');

  const title = $derived(props.title ?? 'Stay Ahead, Always');
  const subtitle = $derived(
    props.subtitle ??
      'Get the latest opportunities, tips, and updates straight to your inbox.'
  );
  const placeholder = $derived(props.placeholder ?? 'Enter your email address');
  const cta = $derived(props.cta ?? 'Subscribe Now');
  const endpoint = $derived(props.endpoint ?? '/newsletter/subscribe');
  const successMessage = $derived(
    props.success_message ?? "You're subscribed! Check your inbox for the next update."
  );
  const errorMessage = $derived(props.error_message ?? 'Something went wrong. Please try again.');
  // Arbitrary child blocks rendered inside the newsletter card.
  const children = $derived(props.children ?? []);

  async function subscribe(e: Event) {
    e.preventDefault();
    if (!email) return;
    newsletterStatus = 'loading';
    const url = (fetchCtx?.apiBase ?? '') + endpoint;
    try {
      const headers: Record<string, string> = { 'content-type': 'application/json' };
      if (fetchCtx?.csrfToken) headers['x-csrf-token'] = fetchCtx.csrfToken;
      if (fetchCtx?.surface) headers['x-jc-surface'] = fetchCtx.surface;
      const res = await (fetchCtx?.fetch ?? fetch)(url, {
        method: 'POST',
        headers,
        body: JSON.stringify({ email }),
        credentials: 'include',
      });
      if (res.ok) {
        const data = await res.json().catch(() => ({}) as { message?: string });
        newsletterStatus = 'success';
        newsletterMessage = data.message ?? successMessage;
        email = '';
      } else {
        const err = await res.json().catch(() => ({ error: { message: '' } }));
        newsletterStatus = 'error';
        newsletterMessage = err.error?.message ?? errorMessage;
      }
    } catch {
      newsletterStatus = 'error';
      newsletterMessage = errorMessage;
    }
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<section class="guest-section newsletter-section" {id} aria-labelledby="newsletter-heading-{id}">
  <div class="newsletter-inner">
    <div class="newsletter-content">
      <div class="newsletter-icon">
        <Plane size={28} />
      </div>
      <div>
        <h2 id="newsletter-heading-{id}" class="newsletter-title">{title}</h2>
        <p class="newsletter-subtitle">{subtitle}</p>
      </div>
    </div>

    <form class="newsletter-form" onsubmit={subscribe}>
      <input
        type="email"
        bind:value={email}
        placeholder={placeholder}
        required
        class="newsletter-input"
        aria-label="Email address"
      />
      <button type="submit" class="newsletter-btn" disabled={newsletterStatus === 'loading'}>
        {cta}
        <ArrowRight class="rtl:-scale-x-100" size={18} />
      </button>
    </form>

    {#if newsletterStatus === 'success'}
      <p class="newsletter-status newsletter-status-success">{newsletterMessage}</p>
    {/if}
    {#if newsletterStatus === 'error'}
      <p class="newsletter-status newsletter-status-error">{newsletterMessage}</p>
    {/if}

    {#if children.length > 0}
      <div class="newsletter-children">
        <GuestChildren blocks={children} {fetchCtx} {geoCountry} />
      </div>
    {/if}
  </div>
</section>
