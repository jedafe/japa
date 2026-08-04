<script lang="ts">
  import type { EntityCollectionProps, EntityType, GeoContext } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';

  interface Props {
    props: EntityCollectionProps;
    onchange: (v: EntityCollectionProps) => void;
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

  function update<K extends keyof EntityCollectionProps>(field: K, v: EntityCollectionProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function updateQuery<K extends keyof EntityCollectionProps['query']>(field: K, v: EntityCollectionProps['query'][K]) {
    update('query', { ...props.query, [field]: v });
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
      onchange={(e) => update('layout', e.currentTarget.value as EntityCollectionProps['layout'])}
      class={inputClass}
    >
      <option value="grid">Grid</option>
      <option value="list">List</option>
      <option value="slider">Slider</option>
      <option value="marquee">Marquee</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Card variant</label>
    <select
      value={props.card_variant}
      onchange={(e) => update('card_variant', e.currentTarget.value as EntityCollectionProps['card_variant'])}
      class={inputClass}
    >
      <option value="feature">Feature</option>
      <option value="compact">Compact</option>
      <option value="minimal">Minimal</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Columns: {props.columns}</label>
    <input
      type="range"
      min="1"
      max="6"
      value={props.columns}
      oninput={(e) => update('columns', Number(e.currentTarget.value))}
      class="w-full"
    />
  </div>

  <div class="space-y-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
    <p class="text-xs font-medium text-[var(--color-muted)]">Query</p>

    <div>
      <label class="mb-1 block text-xs text-[var(--color-muted)]">Limit</label>
      <input
        type="number"
        min="1"
        value={props.query?.limit ?? 10}
        oninput={(e) => updateQuery('limit', Number(e.currentTarget.value))}
        class={inputClass}
      />
    </div>

    <div>
      <label class="mb-1 block text-xs text-[var(--color-muted)]">Sort</label>
      <select
        value={props.query?.sort ?? ''}
        onchange={(e) => updateQuery('sort', e.currentTarget.value)}
        class={inputClass}
      >
        <option value="">Default</option>
        <option value="created_at">Created date</option>
        <option value="updated_at">Updated date</option>
        <option value="title">Title</option>
        <option value="popularity">Popularity</option>
      </select>
    </div>

    <div class="flex items-center gap-2">
      <input
        type="checkbox"
        id="ec-geo"
        checked={props.query?.geo ?? false}
        onchange={(e) => updateQuery('geo', e.currentTarget.checked)}
        class="rounded"
      />
      <label for="ec-geo" class="text-xs font-medium text-[var(--color-muted)]">Geo-aware query</label>
    </div>
  </div>
</div>
