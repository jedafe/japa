<script lang="ts">
  import type { IconProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import IconPicker from '../IconPicker.svelte';

  interface Props {
    props: IconProps;
    onchange: (v: IconProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof IconProps>(field: K, v: IconProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</label>
    <IconPicker value={props.icon} onchange={(v) => update('icon', v ?? '')} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Size</label>
    <select value={props.size} onchange={(e) => update('size', e.currentTarget.value as IconProps['size'])} class={inputClass}>
      <option value="xs">xs</option>
      <option value="sm">sm</option>
      <option value="md">md</option>
      <option value="lg">lg</option>
      <option value="xl">xl</option>
      <option value="2xl">2xl</option>
    </select>
  </div>

  <div class="grid grid-cols-2 gap-2">
    <div>
      <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Color</label>
      <input type="color" value={props.color ?? '#000000'} oninput={(e) => update('color', e.currentTarget.value)} class="h-8 w-full rounded border border-[var(--color-border)]" />
    </div>
    <div>
      <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Background</label>
      <input type="color" value={props.background_color ?? '#ffffff'} oninput={(e) => update('background_color', e.currentTarget.value)} class="h-8 w-full rounded border border-[var(--color-border)]" />
    </div>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Shape</label>
    <select value={props.shape} onchange={(e) => update('shape', e.currentTarget.value as IconProps['shape'])} class={inputClass}>
      <option value="none">None</option>
      <option value="circle">Circle</option>
      <option value="square">Square</option>
      <option value="rounded">Rounded</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</label>
    <input type="text" value={props.label ?? ''} oninput={(e) => update('label', e.currentTarget.value)} class={inputClass} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Href</label>
    <input type="text" value={props.href ?? ''} oninput={(e) => update('href', e.currentTarget.value)} class={inputClass} />
  </div>
</div>
