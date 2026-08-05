<script lang="ts">
  import type { QuoteProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import GeoText from '../GeoText.svelte';
  import MediaPicker from '../MediaPicker.svelte';

  interface Props {
    props: QuoteProps;
    onchange: (v: QuoteProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof QuoteProps>(field: K, v: QuoteProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Quote</span>
    <GeoText value={props.quote} onchange={(v) => update('quote', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Author</span>
    <input type="text" value={props.author} oninput={(e) => update('author', e.currentTarget.value)} class={inputClass} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Role</span>
    <input type="text" value={props.role ?? ''} oninput={(e) => update('role', e.currentTarget.value)} class={inputClass} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Avatar</span>
    <MediaPicker value={props.avatar} onchange={(v) => update('avatar', v)} {mediaPickerComponent} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</span>
    <select value={props.layout} onchange={(e) => update('layout', e.currentTarget.value as QuoteProps['layout'])} class={inputClass}>
      <option value="centered">Centered</option>
      <option value="inline">Inline</option>
      <option value="large">Large</option>
    </select>
  </label>
</div>
