<script lang="ts">
  import type { AccordionProps, Block, GeoContext } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';
  import GeoText from '../GeoText.svelte';
  import NestedBlocksEditor from '../NestedBlocksEditor.svelte';

  interface Props {
    props: AccordionProps;
    onchange: (v: AccordionProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  const allBlockTypes: Block['type'][] = [
    'hero', 'cta_banner', 'card_grid', 'media_grid', 'accordion', 'carousel',
    'entity_collection', 'entity_feature', 'video', 'icon', 'content', 'quote',
    'spacer_divider', 'countdown', 'coming_soon', 'form_embed', 'timeline',
    'map', 'table', 'advertisement', 'live_rate_ticker', 'tabs', 'media',
  ];

  function update<K extends keyof AccordionProps>(field: K, v: AccordionProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addItem() {
    update('items', [...(props.items ?? []), { title: '' }]);
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
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Preset</label>
    <select
      value={props.preset}
      onchange={(e) => update('preset', e.currentTarget.value as AccordionProps['preset'])}
      class={inputClass}
    >
      <option value="faq">FAQ</option>
      <option value="generic">Generic</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</label>
    <select
      value={props.layout}
      onchange={(e) => update('layout', e.currentTarget.value as AccordionProps['layout'])}
      class={inputClass}
    >
      <option value="accordion">Accordion</option>
      <option value="expanded">Expanded</option>
    </select>
  </div>

  <div class="flex items-center gap-2">
    <input
      type="checkbox"
      id="acc-searchable"
      checked={props.searchable}
      onchange={(e) => update('searchable', e.currentTarget.checked)}
      class="rounded"
    />
    <label for="acc-searchable" class="text-xs font-medium text-[var(--color-muted)]">Searchable</label>
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
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Title</label>
            <GeoText value={item.title} onchange={(v) => updateItem(i, 'title', v)} {geoCtx} />
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Content blocks</label>
            <NestedBlocksEditor
              blocks={item.content_blocks ?? []}
              onchange={(v) => updateItem(i, 'content_blocks', v)}
              allowedTypes={allBlockTypes}
              {mediaPickerComponent}
              {geoCtx}
            />
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>
