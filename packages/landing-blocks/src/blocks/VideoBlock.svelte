<script lang="ts">
  import type { VideoProps } from '../types';

  interface Props {
    props: VideoProps;
  }
  let { props }: Props = $props();

  let showVideo = $state(false);

  const aspectClass = $derived(
    props.aspect_ratio === '16:9' ? 'aspect-video' :
    props.aspect_ratio === '4:3' ? 'aspect-[4/3]' :
    props.aspect_ratio === '1:1' ? 'aspect-square' :
    'aspect-[21/9]'
  );

  const embedUrl = $derived(() => {
    if (props.provider === 'youtube') return `https://www.youtube.com/embed/${props.id}?autoplay=${props.autoplay ? 1 : 0}&mute=${props.mute ? 1 : 0}&loop=${props.loop ? 1 : 0}&cc_load_policy=${props.captions ? 1 : 0}`;
    if (props.provider === 'vimeo') return `https://player.vimeo.com/video/${props.id}?autoplay=${props.autoplay ? 1 : 0}&muted=${props.mute ? 1 : 0}&loop=${props.loop ? 1 : 0}`;
    if (props.provider === 'facebook') return `https://www.facebook.com/plugins/video.php?href=${encodeURIComponent(props.url ?? '')}&show_text=false`;
    return props.url ?? '';
  });
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-4xl">
    <div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
      <div class="relative {aspectClass} w-full">
        {#if props.thumbnail && !showVideo}
          <button type="button" onclick={() => (showVideo = true)} class="absolute inset-0 flex items-center justify-center" aria-label="Play video">
            <img src={props.thumbnail.url} alt={props.thumbnail.alt} class="h-full w-full object-cover" loading="lazy" />
            <span class="absolute inset-0 flex items-center justify-center bg-black/30">
              <span class="flex h-16 w-16 items-center justify-center rounded-full bg-white/90 text-2xl text-[var(--color-primary)]">&#9658;</span>
            </span>
          </button>
        {:else if props.provider === 'file'}
          <video
            src={props.url}
            controls
            autoplay={props.autoplay}
            muted={props.mute}
            loop={props.loop}
            class="h-full w-full object-cover"
          >
            <track kind="captions" />
          </video>
        {:else}
          <iframe
            src={embedUrl()}
            class="h-full w-full"
            frameborder="0"
            allow="autoplay; encrypted-media; fullscreen"
            allowfullscreen
            title={props.title ?? 'Video'}
          ></iframe>
        {/if}
      </div>
    </div>
    {#if props.title}
      <h3 class="mt-4 text-lg font-semibold text-[var(--color-text)]">{props.title}</h3>
    {/if}
    {#if props.description}
      <p class="mt-1 text-sm text-[var(--color-text-subtle)]">{props.description}</p>
    {/if}
  </div>
</section>
