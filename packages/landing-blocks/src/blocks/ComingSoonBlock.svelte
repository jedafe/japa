<script lang="ts">
  import type { ComingSoonProps } from '../types';
  import type { FetchContext } from '../data/types';
  import LazyIcon from '../lib/LazyIcon.svelte';

  interface Props {
    props: ComingSoonProps;
    fetchCtx?: FetchContext;
  }
  let { props, fetchCtx }: Props = $props();

  let email = $state('');
  let submitted = $state(false);
  let error = $state(false);

  // Relative stored endpoints resolve against the page origin — on
  // split-origin deploys the frontend host does not proxy /api, so a
  // content-stored "/api/v1/..." (or bare "/newsletter/...") 404s there.
  // Rebase relative endpoints onto the injected API base when present.
  const endpoint = $derived.by(() => {
    const ep = props.waitlist_form?.endpoint;
    if (!ep) return '';
    if (ep.startsWith('/') && fetchCtx?.apiBase) return `${fetchCtx.apiBase}${ep}`;
    return ep;
  });

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (!props.waitlist_form?.enabled || !endpoint) return;
    try {
      const headers: Record<string, string> = { 'content-type': 'application/json' };
      if (fetchCtx?.csrfToken) headers['x-csrf-token'] = fetchCtx.csrfToken;
      if (fetchCtx?.surface) headers['x-jc-surface'] = fetchCtx.surface;
      const res = await (fetchCtx?.fetch ?? fetch)(endpoint, {
        method: 'POST',
        headers,
        body: JSON.stringify({ email: email }),
        credentials: 'include',
      });
      if (!res.ok) throw new Error('Failed');
      submitted = true;
    } catch {
      error = true;
    }
  }


</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-2xl rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-12 text-center">
    {#if props.icon}
      <div class="mx-auto mb-6 inline-flex rounded-full bg-[var(--color-surface-subtle)] p-4 text-[var(--color-primary)]">
        <LazyIcon name={props.icon} size={32} />
      </div>
    {/if}
    {#if props.title}
      <h2 class="text-2xl font-bold text-[var(--color-text)]">{props.title}</h2>
    {/if}
    {#if props.description}
      <p class="mt-3 text-[var(--color-text-subtle)]">{props.description}</p>
    {/if}
    {#if props.estimated_date}
      <p class="mt-2 text-sm text-[var(--color-muted)]">Expected: {props.estimated_date}</p>
    {/if}
    {#if props.waitlist_form?.enabled}
      {#if submitted}
        <p class="mt-6 text-sm font-medium text-[var(--color-primary)]">You're on the list! We'll notify you when it's ready.</p>
      {:else}
        <form onsubmit={handleSubmit} class="mt-6 flex flex-col gap-3 sm:flex-row sm:justify-center">
          <input
            type="email"
            bind:value={email}
            placeholder="Enter your email"
            required
            class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
          />
          <button type="submit" class="rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]">
            {props.waitlist_form.cta_label ?? 'Notify me'}
          </button>
        </form>
        {#if error}
          <p class="mt-2 text-sm text-[var(--color-error)]">Something went wrong. Please try again.</p>
        {/if}
      {/if}
    {/if}
  </div>
</section>
