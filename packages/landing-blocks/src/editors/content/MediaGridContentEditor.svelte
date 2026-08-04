<script lang="ts">
  import type { MediaGridProps, GeoContext } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';
  import MediaPicker from '../MediaPicker.svelte';

  interface Props {
    props: MediaGridProps;
    onchange: (v: MediaGridProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof MediaGridProps>(field: K, v: MediaGridProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addItem() {
    update('items', [...(props.items ?? []), { media: { media_id: '', url: '', alt: '' } }]);
  }
  function removeItem(i: number) {
    update('items', (props.items ?? []).filter((_, idx) => idx !== i));
  }
  function updateItem(i: number, field: string, v: unknown) {
    const arr = [...(props.items ?? [])];
    arr[i] = { ...arr[i], [field]: v };
    update('items', arr);
  }
</script>

<div class="space-y-4">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Card style</label>
    <select
      value={props.card_style}
      onchange={(e) => update('card_style', e.currentTarget.value as MediaGridProps['card_style'])}
      class={inputClass}
    >
      <option value="default">Default</option>
      <option value="logo">Logo</option>
      <option value="overlay">Overlay</option>
    </select>
  </div>

  <div class="flex items-center gap-2">
    <input
      type="checkbox"
      id="mg-lightbox"
      checked={props.lightbox}
      onchange={(e) => update('lightbox', e.currentTarget.checked)}
      class="rounded"
    />
    <label for="mg-lightbox" class="text-xs font-medium text-[var(--color-muted)]">Lightbox</label>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Columns: {props.columns}</label>
    <input
      type="range"
      min="1"
      max="8"
      value={props.columns}
      oninput={(e) => update('columns', Number(e.currentTarget.value))}
      class="w-full"
    />
  </div>

  <div>
    <div class="mb-2 flex items-center justify-between">
      <label class="text-xs font-medium text-[var(--color-muted)]">Items</label>
      <button type="button" onclick={addItem} class="text-xs text-[var(--color-primary)] hover:underline">+ Add Item</button>
    </div>
    <div class="space-y-4">
      {#each props.items ?? [] as item, i}
        <div class="space-y-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium text-[var(--color-muted)]">Item {i + 1}</span>
            <button type="button" onclick={() => removeItem(i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Media</label>
            <MediaPicker value={item.media} onchange={(v) => updateItem(i, 'media', v)} {mediaPickerComponent} />
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Caption</label>
            <input
              type="text"
              value={item.caption ?? ''}
              oninput={(e) => updateItem(i, 'caption', e.currentTarget.value)}
              class={inputClass}
            />
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Href</label>
            <input
              type="text"
              value={item.href ?? ''}
              oninput={(e) => updateItem(i, 'href', e.currentTarget.value)}
              class={inputClass}
            />
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>
