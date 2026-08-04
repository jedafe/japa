<script lang="ts">
  import type { MediaBlockProps } from '../types';
  import { Dialog } from 'bits-ui';

  interface Props {
    props: MediaBlockProps;
  }
  let { props }: Props = $props();

  let lightboxOpen = $state(false);

  const layoutClass = $derived(
    props.layout === 'full' ? 'w-full' :
    props.layout === 'split' ? 'mx-auto max-w-2xl' :
    'mx-auto max-w-4xl'
  );
</script>

<section class="px-6 py-8">
  <div class={layoutClass}>
    {#if props.lightbox}
      <button type="button" onclick={() => (lightboxOpen = true)} class="block w-full" aria-label="Open image in lightbox">
        <img src={props.media.url} alt={props.media.alt} class="w-full rounded-[var(--radius-card)] object-cover" loading="lazy" />
      </button>
      <Dialog.Root bind:open={lightboxOpen}>
        <Dialog.Portal>
          <Dialog.Overlay class="fixed inset-0 bg-black/80 z-50" />
          <Dialog.Content class="fixed inset-0 z-50 flex items-center justify-center p-6">
            <img src={props.media.url} alt={props.media.alt} class="max-h-full max-w-full rounded-[var(--radius)] object-contain" />
            <Dialog.Close class="absolute top-4 right-4 rounded-full bg-white/20 p-2 text-white hover:bg-white/30" aria-label="Close">
              &#10005;
            </Dialog.Close>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    {:else}
      <img src={props.media.url} alt={props.media.alt} class="w-full rounded-[var(--radius-card)] object-cover" loading="lazy" />
    {/if}
    {#if props.caption}
      <p class="mt-3 text-center text-sm text-[var(--color-muted)]">{props.caption}</p>
    {/if}
  </div>
</section>
