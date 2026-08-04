<script lang="ts">
  import type { EntityFeatureProps, EntityType, GeoContext } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';
  import EntityRefPicker from '../EntityRefPicker.svelte';
  import MediaPicker from '../MediaPicker.svelte';

  interface Props {
    props: EntityFeatureProps;
    onchange: (v: EntityFeatureProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  const entityTypes: EntityType[] = [
    'post', 'blog', 'thread', 'space', 'organization', 'university',
    'job', 'scholarship', 'tool', 'remittance_rate',
  ];

  function update<K extends keyof EntityFeatureProps>(field: K, v: EntityFeatureProps[K]) {
    onchange({ ...props, [field]: v });
  }
</script>

<div class="space-y-4">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Entity type</label>
    <select
      value={props.entity_type}
      onchange={(e) => update('entity_type', e.currentTarget.value as EntityType)}
      class={inputClass}
    >
      {#each entityTypes as et}
        <option value={et}>{et}</option>
      {/each}
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</label>
    <select
      value={props.layout}
      onchange={(e) => update('layout', e.currentTarget.value as EntityFeatureProps['layout'])}
      class={inputClass}
    >
      <option value="hero">Hero</option>
      <option value="card">Card</option>
      <option value="split">Split</option>
      <option value="banner">Banner</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Reference</label>
    <EntityRefPicker
      entityType={props.entity_type}
      value={props.ref}
      onchange={(v) => update('ref', v)}
      {fetchCtx}
    />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Media override</label>
    <MediaPicker
      value={props.media_override}
      onchange={(v) => update('media_override', v)}
      {mediaPickerComponent}
    />
  </div>
</div>
