<script lang="ts">
  import type { WidgetBlockProps } from '../../types';
  import type { Component } from 'svelte';

  interface Props {
    props: WidgetBlockProps;
    onchange: (v: WidgetBlockProps) => void;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mediaPickerComponent?: any;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    geoCtx?: any;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    fetchCtx?: any;
    /** Optional widget picker component (e.g. Autocomplete-based WidgetPicker).
     *  When provided, replaces the plain UUID text input with a live search.
     *  Expected props: selectedId, selectedLabel, onSelect, onClear. */
    widgetPickerComponent?: Component;
  }
  let { props, onchange, widgetPickerComponent }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof WidgetBlockProps>(field: K, v: WidgetBlockProps[K]) {
    onchange({ ...props, [field]: v });
  }
</script>

<div class="space-y-4">
  {#if widgetPickerComponent}
    {@const WidgetPicker = widgetPickerComponent}
    <div>
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Widget</span>
      <WidgetPicker
        selectedId={props.widget_id || null}
        selectedLabel={props.heading || props.widget_id || null}
        onSelect={(item: { id: string; label: string }) => {
          update('widget_id', item.id);
          // Auto-fill heading if empty so the block has a sensible title.
          if (!props.heading) update('heading', item.label);
        }}
        onClear={() => update('widget_id', '')}
      />
      <span class="mt-1 block text-[10px] text-[var(--color-muted)]">Search for a widget by name. Create new widgets in the Widgets admin page.</span>
    </div>
  {:else}
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Widget ID</span>
      <input
        type="text"
        value={props.widget_id ?? ''}
        placeholder="UUID from /admin/widgets"
        onchange={(e) => update('widget_id', e.currentTarget.value)}
        class={inputClass}
      />
      <span class="mt-1 block text-[10px] text-[var(--color-muted)]">Create a widget in the Widgets admin page, then paste its ID here.</span>
    </label>
  {/if}

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Columns</span>
    <select
      value={props.columns ?? 3}
      onchange={(e) => update('columns', Number(e.currentTarget.value))}
      class={inputClass}
    >
      <option value={1}>1</option>
      <option value={2}>2</option>
      <option value={3}>3</option>
      <option value={4}>4</option>
    </select>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Heading (optional)</span>
    <input
      type="text"
      value={props.heading ?? ''}
      placeholder="e.g. Latest Blog Posts"
      onchange={(e) => update('heading', e.currentTarget.value)}
      class={inputClass}
    />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">View All link (optional)</span>
    <input
      type="text"
      value={props.view_all_href ?? ''}
      placeholder="e.g. /blog"
      onchange={(e) => update('view_all_href', e.currentTarget.value)}
      class={inputClass}
    />
  </label>
</div>
