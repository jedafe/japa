<script lang="ts">
  import type { LiveRateTickerProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';

  interface Props {
    props: LiveRateTickerProps;
    onchange: (v: LiveRateTickerProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof LiveRateTickerProps>(field: K, v: LiveRateTickerProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Corridors (comma separated)</label>
    <input
      type="text"
      value={(props.corridors ?? []).join(', ')}
      oninput={(e) => update('corridors', e.currentTarget.value.split(',').map((s) => s.trim()).filter(Boolean))}
      placeholder="US-NG, US-KE"
      class={inputClass}
    />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Limit</label>
    <input type="number" min="1" value={props.limit} oninput={(e) => update('limit', Number(e.currentTarget.value))} class={inputClass} />
  </div>

  <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
    <input type="checkbox" checked={props.geo_aware} onchange={(e) => update('geo_aware', e.currentTarget.checked)} /> Geo-aware
  </label>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</label>
    <select value={props.layout} onchange={(e) => update('layout', e.currentTarget.value as LiveRateTickerProps['layout'])} class={inputClass}>
      <option value="horizontal">Horizontal</option>
      <option value="vertical">Vertical</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Speed</label>
    <input type="number" min="1" value={props.speed ?? 30} oninput={(e) => update('speed', Number(e.currentTarget.value))} class={inputClass} />
  </div>
</div>
