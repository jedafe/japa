<script lang="ts">
  import type { VideoProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import GeoText from '../GeoText.svelte';
  import MediaPicker from '../MediaPicker.svelte';

  interface Props {
    props: VideoProps;
    onchange: (v: VideoProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof VideoProps>(field: K, v: VideoProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Provider</span>
    <select value={props.provider} onchange={(e) => update('provider', e.currentTarget.value as VideoProps['provider'])} class={inputClass}>
      <option value="youtube">YouTube</option>
      <option value="facebook">Facebook</option>
      <option value="vimeo">Vimeo</option>
      <option value="file">File</option>
    </select>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">URL</span>
    <input type="text" value={props.url ?? ''} oninput={(e) => update('url', e.currentTarget.value)} class={inputClass} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Video ID</span>
    <input type="text" value={props.id ?? ''} oninput={(e) => update('id', e.currentTarget.value)} class={inputClass} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Aspect ratio</span>
    <select value={props.aspect_ratio} onchange={(e) => update('aspect_ratio', e.currentTarget.value as VideoProps['aspect_ratio'])} class={inputClass}>
      <option value="16:9">16:9</option>
      <option value="4:3">4:3</option>
      <option value="1:1">1:1</option>
      <option value="21:9">21:9</option>
    </select>
  </label>

  <div class="flex flex-wrap gap-4">
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.autoplay} onchange={(e) => update('autoplay', e.currentTarget.checked)} /> Autoplay
    </label>
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.mute} onchange={(e) => update('mute', e.currentTarget.checked)} /> Mute
    </label>
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.captions} onchange={(e) => update('captions', e.currentTarget.checked)} /> Captions
    </label>
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.loop} onchange={(e) => update('loop', e.currentTarget.checked)} /> Loop
    </label>
  </div>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Thumbnail</span>
    <MediaPicker value={props.thumbnail} onchange={(v) => update('thumbnail', v)} {mediaPickerComponent} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span>
    <GeoText value={props.title ?? ''} onchange={(v) => update('title', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Description</span>
    <GeoText value={props.description ?? ''} onchange={(v) => update('description', v)} {geoCtx} />
  </label>
</div>
