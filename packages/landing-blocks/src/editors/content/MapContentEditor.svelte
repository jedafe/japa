<script lang="ts">
  import type { MapProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';

  interface Props {
    props: MapProps;
    onchange: (v: MapProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  type Pin = MapProps['pins'][number];

  function update<K extends keyof MapProps>(field: K, v: MapProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addPin() {
    update('pins', [...props.pins, { country: '', label: '' }]);
  }

  function removePin(idx: number) {
    update('pins', props.pins.filter((_, i) => i !== idx));
  }

  function updatePin(idx: number, patch: Partial<Pin>) {
    update('pins', props.pins.map((p, i) => (i === idx ? { ...p, ...patch } : p)));
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Mode</span>
    <select value={props.mode} onchange={(e) => update('mode', e.currentTarget.value as MapProps['mode'])} class={inputClass}>
      <option value="static">Static</option>
      <option value="dynamic">Dynamic</option>
    </select>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Geo source</span>
    <select value={props.geo_source} onchange={(e) => update('geo_source', e.currentTarget.value as MapProps['geo_source'])} class={inputClass}>
      <option value="manual">Manual</option>
      <option value="trending_spaces">Trending spaces</option>
    </select>
  </label>

  <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
    <input type="checkbox" checked={props.highlight_visitor_country} onchange={(e) => update('highlight_visitor_country', e.currentTarget.checked)} /> Highlight visitor country
  </label>

  <label class="block space-y-2">
    <span class="block text-xs font-medium text-[var(--color-muted)]">Pins</span>
    {#each props.pins as pin, i (i)}
      <div class="space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">Pin {i + 1}</span>
          <button type="button" onclick={() => removePin(i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        <input type="text" value={pin.country} placeholder="country" oninput={(e) => updatePin(i, { country: e.currentTarget.value })} class={inputClass} />
        <input type="text" value={pin.label} placeholder="label" oninput={(e) => updatePin(i, { label: e.currentTarget.value })} class={inputClass} />
        <input type="text" value={pin.href ?? ''} placeholder="href" oninput={(e) => updatePin(i, { href: e.currentTarget.value })} class={inputClass} />
      </div>
    {/each}
    <button type="button" onclick={addPin} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add pin</button>
  </label>
</div>
