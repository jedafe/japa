<script lang="ts">
  import type { MediaBlockProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import GeoText from '../GeoText.svelte';
  import MediaPicker from '../MediaPicker.svelte';

  interface Props {
    props: MediaBlockProps;
    onchange: (v: MediaBlockProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof MediaBlockProps>(field: K, v: MediaBlockProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Media</label>
    <MediaPicker value={props.media} onchange={(v) => update('media', v as MediaBlockProps['media'])} {mediaPickerComponent} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</label>
    <select value={props.layout} onchange={(e) => update('layout', e.currentTarget.value as MediaBlockProps['layout'])} class={inputClass}>
      <option value="full">Full</option>
      <option value="contained">Contained</option>
      <option value="split">Split</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Caption</label>
    <GeoText value={props.caption ?? ''} onchange={(v) => update('caption', v)} {geoCtx} />
  </div>

  <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
    <input type="checkbox" checked={props.lightbox} onchange={(e) => update('lightbox', e.currentTarget.checked)} /> Lightbox
  </label>
</div>
