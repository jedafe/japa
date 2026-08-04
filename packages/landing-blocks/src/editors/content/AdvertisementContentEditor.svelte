<script lang="ts">
  import type { AdvertisementProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';

  interface Props {
    props: AdvertisementProps;
    onchange: (v: AdvertisementProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof AdvertisementProps>(field: K, v: AdvertisementProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Slot key</label>
    <input type="text" value={props.slot_key} oninput={(e) => update('slot_key', e.currentTarget.value)} placeholder="e.g. sidebar_leaderboard" class={inputClass} />
  </div>
</div>
