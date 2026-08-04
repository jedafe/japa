<script lang="ts">
  import type { SpacerDividerProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';

  interface Props {
    props: SpacerDividerProps;
    onchange: (v: SpacerDividerProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof SpacerDividerProps>(field: K, v: SpacerDividerProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Height: {props.height}px</label>
    <input type="range" min="0" max="200" value={props.height} oninput={(e) => update('height', Number(e.currentTarget.value))} class="w-full" />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Divider style</label>
    <select value={props.divider_style} onchange={(e) => update('divider_style', e.currentTarget.value as SpacerDividerProps['divider_style'])} class={inputClass}>
      <option value="none">None</option>
      <option value="line">Line</option>
      <option value="gradient">Gradient</option>
      <option value="dots">Dots</option>
    </select>
  </div>

  {#if props.divider_style !== 'none'}
    <div>
      <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Divider color</label>
      <div class="flex items-center gap-2">
        <input type="color" value={props.divider_color ?? '#cccccc'} oninput={(e) => update('divider_color', e.currentTarget.value)} class="h-8 w-12 rounded border border-[var(--color-border)]" />
        <input type="text" value={props.divider_color ?? ''} oninput={(e) => update('divider_color', e.currentTarget.value)} placeholder="#cccccc" class={inputClass} />
      </div>
    </div>
  {/if}
</div>
