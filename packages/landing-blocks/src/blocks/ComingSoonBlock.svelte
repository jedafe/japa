<script lang="ts">
  import type { ComingSoonProps } from '../types';
  import type { Component } from 'svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: ComingSoonProps;
  }
  let { props }: Props = $props();

  let email = $state('');
  let submitted = $state(false);
  let error = $state(false);

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (!props.waitlist_form?.enabled || !props.waitlist_form.endpoint) return;
    try {
      const res = await fetch(props.waitlist_form.endpoint, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email: email }),
      });
      if (!res.ok) throw new Error('Failed');
      submitted = true;
    } catch {
      error = true;
    }
  }

  function getIcon(name?: string): Component | null {
    if (!name) return null;
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-2xl rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-12 text-center">
    {#if props.icon}
      {@const Icon = getIcon(props.icon)}
      {#if Icon}
        <div class="mx-auto mb-6 inline-flex rounded-full bg-[var(--color-surface-subtle)] p-4 text-[var(--color-primary)]">
          <Icon size={32} />
        </div>
      {/if}
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
