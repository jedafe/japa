<script lang="ts">
  import type { TabsProps, GeoContext, Block } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import IconPicker from '../IconPicker.svelte';
  import NestedBlocksEditor from '../NestedBlocksEditor.svelte';

  interface Props {
    props: TabsProps;
    onchange: (v: TabsProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  type Tab = TabsProps['tabs'][number];

  function update<K extends keyof TabsProps>(field: K, v: TabsProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addTab() {
    update('tabs', [...props.tabs, { label: '' }]);
  }

  function removeTab(idx: number) {
    update('tabs', props.tabs.filter((_, i) => i !== idx));
  }

  function updateTab(idx: number, patch: Partial<Tab>) {
    update('tabs', props.tabs.map((t, i) => (i === idx ? { ...t, ...patch } : t)));
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</label>
    <select value={props.layout} onchange={(e) => update('layout', e.currentTarget.value as TabsProps['layout'])} class={inputClass}>
      <option value="underline">Underline</option>
      <option value="pills">Pills</option>
      <option value="boxed">Boxed</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Orientation</label>
    <select value={props.orientation} onchange={(e) => update('orientation', e.currentTarget.value as TabsProps['orientation'])} class={inputClass}>
      <option value="horizontal">Horizontal</option>
      <option value="vertical">Vertical</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Initial tab index</label>
    <input type="number" min="0" value={props.initial_tab} oninput={(e) => update('initial_tab', Number(e.currentTarget.value))} class={inputClass} />
  </div>

  <div class="space-y-2">
    <label class="block text-xs font-medium text-[var(--color-muted)]">Tabs</label>
    {#each props.tabs as tab, i (i)}
      <div class="space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">Tab {i + 1}</span>
          <button type="button" onclick={() => removeTab(i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        <div>
          <label class="mb-1 block text-xs text-[var(--color-muted)]">Label</label>
          <input type="text" value={tab.label} oninput={(e) => updateTab(i, { label: e.currentTarget.value })} class={inputClass} />
        </div>
        <div>
          <label class="mb-1 block text-xs text-[var(--color-muted)]">Icon</label>
          <IconPicker value={tab.icon} onchange={(v) => updateTab(i, { icon: v })} />
        </div>
        <div>
          <label class="mb-1 block text-xs text-[var(--color-muted)]">Content blocks</label>
          <NestedBlocksEditor
            blocks={tab.content_blocks ?? []}
            onchange={(v: Block[]) => updateTab(i, { content_blocks: v })}
            allowedTypes={['content', 'media', 'card_grid', 'icon', 'quote', 'video']}
            {mediaPickerComponent}
            {geoCtx}
          />
        </div>
      </div>
    {/each}
    <button type="button" onclick={addTab} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add tab</button>
  </div>
</div>
