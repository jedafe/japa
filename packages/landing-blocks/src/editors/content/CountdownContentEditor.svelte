<script lang="ts">
  import type { CountdownProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import GeoText from '../GeoText.svelte';

  interface Props {
    props: CountdownProps;
    onchange: (v: CountdownProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof CountdownProps>(field: K, v: CountdownProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Target date</label>
    <input type="datetime-local" value={props.target_date} oninput={(e) => update('target_date', e.currentTarget.value)} class={inputClass} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</label>
    <GeoText value={props.label ?? ''} onchange={(v) => update('label', v)} {geoCtx} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</label>
    <select value={props.layout} onchange={(e) => update('layout', e.currentTarget.value as CountdownProps['layout'])} class={inputClass}>
      <option value="inline">Inline</option>
      <option value="stacked">Stacked</option>
      <option value="card">Card</option>
    </select>
  </div>

  <div class="flex flex-wrap gap-4">
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.show_days} onchange={(e) => update('show_days', e.currentTarget.checked)} /> Days
    </label>
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.show_hours} onchange={(e) => update('show_hours', e.currentTarget.checked)} /> Hours
    </label>
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.show_minutes} onchange={(e) => update('show_minutes', e.currentTarget.checked)} /> Minutes
    </label>
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.show_seconds} onchange={(e) => update('show_seconds', e.currentTarget.checked)} /> Seconds
    </label>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">On expire</label>
    <select value={props.on_expire} onchange={(e) => update('on_expire', e.currentTarget.value as CountdownProps['on_expire'])} class={inputClass}>
      <option value="hide">Hide</option>
      <option value="show_message">Show message</option>
    </select>
  </div>

  {#if props.on_expire === 'show_message'}
    <div>
      <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Expired message</label>
      <GeoText value={props.expired_message ?? ''} onchange={(v) => update('expired_message', v)} {geoCtx} />
    </div>
  {/if}
</div>
