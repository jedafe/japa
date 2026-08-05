<script lang="ts">
  import type { CarouselProps, Block, GeoContext } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';
  import NestedBlocksEditor from '../NestedBlocksEditor.svelte';

  interface Props {
    props: CarouselProps;
    onchange: (v: CarouselProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  const allBlockTypes: Block['type'][] = [
    'hero', 'cta_banner', 'card_grid', 'media_grid', 'accordion', 'carousel',
    'entity_collection', 'entity_feature', 'video', 'icon', 'content', 'quote',
    'spacer_divider', 'countdown', 'coming_soon', 'form_embed', 'timeline',
    'map', 'table', 'advertisement', 'live_rate_ticker', 'tabs', 'media',
  ];

  function update<K extends keyof CarouselProps>(field: K, v: CarouselProps[K]) {
    onchange({ ...props, [field]: v });
  }
</script>

<div class="space-y-4">
  <div class="flex items-center gap-2">
    <input
      type="checkbox"
      id="car-autoplay"
      checked={props.autoplay}
      onchange={(e) => update('autoplay', e.currentTarget.checked)}
      class="rounded"
    />
    <label for="car-autoplay" class="text-xs font-medium text-[var(--color-muted)]">Autoplay</label>
  </div>

  <div class="flex items-center gap-2">
    <input
      type="checkbox"
      id="car-dots"
      checked={props.dots}
      onchange={(e) => update('dots', e.currentTarget.checked)}
      class="rounded"
    />
    <label for="car-dots" class="text-xs font-medium text-[var(--color-muted)]">Dots</label>
  </div>

  <div class="flex items-center gap-2">
    <input
      type="checkbox"
      id="car-arrows"
      checked={props.arrows}
      onchange={(e) => update('arrows', e.currentTarget.checked)}
      class="rounded"
    />
    <label for="car-arrows" class="text-xs font-medium text-[var(--color-muted)]">Arrows</label>
  </div>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Transition</span>
    <select
      value={props.transition}
      onchange={(e) => update('transition', e.currentTarget.value as CarouselProps['transition'])}
      class={inputClass}
    >
      <option value="slide">Slide</option>
      <option value="fade">Fade</option>
    </select>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Interval (ms)</span>
    <input
      type="number"
      min="0"
      step="100"
      value={props.interval}
      oninput={(e) => update('interval', Number(e.currentTarget.value))}
      class={inputClass}
    />
  </label>

  <div class="flex items-center gap-2">
    <input
      type="checkbox"
      id="car-pause"
      checked={props.pause_on_hover}
      onchange={(e) => update('pause_on_hover', e.currentTarget.checked)}
      class="rounded"
    />
    <label for="car-pause" class="text-xs font-medium text-[var(--color-muted)]">Pause on hover</label>
  </div>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Slides</span>
    <NestedBlocksEditor
      blocks={props.slides ?? []}
      onchange={(v) => update('slides', v)}
      allowedTypes={allBlockTypes}
      {mediaPickerComponent}
      {geoCtx}
    />
  </label>
</div>
