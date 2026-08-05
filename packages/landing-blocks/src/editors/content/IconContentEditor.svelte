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
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</span>
    <IconPicker value={props.icon} onchange={(v) => update('icon', v ?? '')} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Size</span>
    <select value={props.size} onchange={(e) => update('size', e.currentTarget.value as IconProps['size'])} class={inputClass}>
      <option value="xs">xs</option>
      <option value="sm">sm</option>
      <option value="md">md</option>
      <option value="lg">lg</option>
      <option value="xl">xl</option>
      <option value="2xl">2xl</option>
    </select>
  </label>

  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Color</span>
      <input type="color" value={props.color ?? '#000000'} oninput={(e) => update('color', e.currentTarget.value)} class="h-8 w-full rounded border border-[var(--color-border)]" />
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Background</span>
      <input type="color" value={props.background_color ?? '#ffffff'} oninput={(e) => update('background_color', e.currentTarget.value)} class="h-8 w-full rounded border border-[var(--color-border)]" />
    </label>
  </div>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Shape</span>
    <select value={props.shape} onchange={(e) => update('shape', e.currentTarget.value as IconProps['shape'])} class={inputClass}>
      <option value="none">None</option>
      <option value="circle">Circle</option>
      <option value="square">Square</option>
      <option value="rounded">Rounded</option>
    </select>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</span>
    <input type="text" value={props.label ?? ''} oninput={(e) => update('label', e.currentTarget.value)} class={inputClass} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Href</span>
    <input type="text" value={props.href ?? ''} oninput={(e) => update('href', e.currentTarget.value)} class={inputClass} />
  </label>
</div>
