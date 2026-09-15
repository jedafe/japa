<script lang="ts">
  import type { GuestHeroImageProps, MediaRef } from '../../../types';
  import type { Component } from 'svelte';
  import MediaPicker from '../../MediaPicker.svelte';

  interface Props {
    props: GuestHeroImageProps;
    onchange: (v: GuestHeroImageProps) => void;
    mediaPickerComponent?: Component;
  }

  let { props, onchange, mediaPickerComponent }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroImageProps>(key: K, value: GuestHeroImageProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function onSlideChange(i: number, m: MediaRef | undefined) {
    const slides = [...(props.slides ?? [])];
    if (m) { slides[i] = m; } else { slides.splice(i, 1); }
    update('slides', slides.length > 0 ? slides : undefined);
  }
  function addSlide() {
    update('slides', [...(props.slides ?? []), { media_id: '', url: '', alt: '' }]);
  }
  function removeSlide(i: number) {
    const slides = [...(props.slides ?? [])];
    slides.splice(i, 1);
    update('slides', slides.length > 0 ? slides : undefined);
  }
</script>

<div class="space-y-4">
  <div class="space-y-2">
    <div class="text-xs font-medium text-[var(--color-muted)]">Slides</div>
    {#each props.slides ?? [] as slide, i (i)}
      <div class="flex items-center gap-2">
        <MediaPicker value={slide} onchange={(m) => onSlideChange(i, m ?? undefined)} mediaPickerComponent={mediaPickerComponent} />
        <button type="button" onclick={() => removeSlide(i)} class="text-[var(--color-danger)]">remove</button>
      </div>
    {/each}
    <button type="button" onclick={addSlide} class="rounded border border-dashed border-[var(--color-border)] px-2 py-1 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface)]">Add slide</button>
  </div>

  <div class="grid grid-cols-2 gap-2">
    <label class="flex items-center gap-2 text-sm"><input type="checkbox" checked={props.autoplay ?? true} onchange={(e) => update('autoplay', e.currentTarget.checked)} /> Autoplay</label>
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Interval (ms)</span><input type="number" class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.interval ?? 5000} oninput={(e) => update('interval', Number(e.currentTarget.value))} /></label>
  </div>
  <div class="grid grid-cols-2 gap-2">
    <label class="flex items-center gap-2 text-sm"><input type="checkbox" checked={props.dots ?? true} onchange={(e) => update('dots', e.currentTarget.checked)} /> Show dots</label>
    <label class="flex items-center gap-2 text-sm"><input type="checkbox" checked={props.arrows ?? true} onchange={(e) => update('arrows', e.currentTarget.checked)} /> Show arrows</label>
  </div>
</div>
