<script lang="ts">
  import type { QuoteProps } from '../types';

  interface Props {
    props: QuoteProps;
  }
  let { props }: Props = $props();

  const layoutClass = $derived(
    props.layout === 'centered' ? 'text-center mx-auto max-w-3xl' :
    props.layout === 'large' ? 'text-center mx-auto max-w-5xl' :
    'max-w-4xl'
  );
</script>

<section class="px-6 py-16">
  <div class={layoutClass}>
    <span class="block text-6xl text-[var(--color-primary)] font-serif leading-none mb-4">&ldquo;</span>
    <blockquote class="text-xl text-[var(--color-text)] font-serif italic {props.layout === 'large' ? 'sm:text-2xl' : ''}">
      {props.quote}
    </blockquote>
    <div class="mt-6 flex items-center gap-3 {props.layout === 'centered' || props.layout === 'large' ? 'justify-center' : ''}">
      {#if props.avatar}
        <img src={props.avatar.url} alt={props.avatar.alt} class="h-12 w-12 rounded-full object-cover" loading="lazy" />
      {/if}
      <div>
        <p class="font-semibold text-[var(--color-text)]">{props.author}</p>
        {#if props.role}
          <p class="text-sm text-[var(--color-muted)]">{props.role}</p>
        {/if}
      </div>
    </div>
  </div>
</section>
