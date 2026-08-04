<script lang="ts">
  import type { TableProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';

  interface Props {
    props: TableProps;
    onchange: (v: TableProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  type Column = TableProps['columns'][number];

  function update<K extends keyof TableProps>(field: K, v: TableProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addColumn() {
    update('columns', [...props.columns, { key: '', label: '', align: 'left' }]);
  }

  function removeColumn(idx: number) {
    update('columns', props.columns.filter((_, i) => i !== idx));
  }

  function updateColumn(idx: number, patch: Partial<Column>) {
    update('columns', props.columns.map((c, i) => (i === idx ? { ...c, ...patch } : c)));
  }

  function addRow() {
    const empty: Record<string, string> = {};
    for (const col of props.columns) empty[col.key] = '';
    update('rows', [...props.rows, empty]);
  }

  function removeRow(idx: number) {
    update('rows', props.rows.filter((_, i) => i !== idx));
  }

  function updateRow(idx: number, key: string, value: string) {
    update('rows', props.rows.map((r, i) => (i === idx ? { ...r, [key]: value } : r)));
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div class="space-y-2">
    <label class="block text-xs font-medium text-[var(--color-muted)]">Columns</label>
    {#each props.columns as col, i (i)}
      <div class="space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">Column {i + 1}</span>
          <button type="button" onclick={() => removeColumn(i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <input type="text" value={col.key} placeholder="key" oninput={(e) => updateColumn(i, { key: e.currentTarget.value })} class={inputClass} />
          <input type="text" value={col.label} placeholder="label" oninput={(e) => updateColumn(i, { label: e.currentTarget.value })} class={inputClass} />
        </div>
        <select value={col.align ?? 'left'} onchange={(e) => updateColumn(i, { align: e.currentTarget.value as Column['align'] })} class={inputClass}>
          <option value="left">Left</option>
          <option value="center">Center</option>
          <option value="right">Right</option>
        </select>
      </div>
    {/each}
    <button type="button" onclick={addColumn} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add column</button>
  </div>

  <div class="flex gap-4">
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.sortable} onchange={(e) => update('sortable', e.currentTarget.checked)} /> Sortable
    </label>
    <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.striped} onchange={(e) => update('striped', e.currentTarget.checked)} /> Striped
    </label>
  </div>

  <div class="space-y-2">
    <label class="block text-xs font-medium text-[var(--color-muted)]">Rows</label>
    {#each props.rows as row, i (i)}
      <div class="space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">Row {i + 1}</span>
          <button type="button" onclick={() => removeRow(i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        {#each props.columns as col (col.key)}
          <input type="text" value={row[col.key] ?? ''} placeholder={col.label} oninput={(e) => updateRow(i, col.key, e.currentTarget.value)} class={inputClass} />
        {/each}
      </div>
    {/each}
    <button type="button" onclick={addRow} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add row</button>
  </div>
</div>
