<script lang="ts">
  import type { TableProps } from '../types';

  interface Props {
    props: TableProps;
  }
  let { props }: Props = $props();

  let sortKey = $state<string | null>(null);
  let sortDir = $state<'asc' | 'desc'>('asc');

  const sortedRows = $derived(() => {
    if (!sortKey) return props.rows;
    const sorted = [...props.rows].sort((a, b) => {
      const av = a[sortKey] ?? '';
      const bv = b[sortKey] ?? '';
      return sortDir === 'asc' ? av.localeCompare(bv) : bv.localeCompare(av);
    });
    return sorted;
  });

  function toggleSort(key: string) {
    if (sortKey === key) {
      sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    } else {
      sortKey = key;
      sortDir = 'asc';
    }
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-4xl overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)]">
    <table class="w-full border-collapse">
      <thead>
        <tr class="bg-[var(--color-surface-subtle)]">
          {#each props.columns as col}
            <th
              class="px-4 py-3 text-sm font-semibold text-[var(--color-text)] {col.align === 'center' ? 'text-center' : col.align === 'right' ? 'text-right' : 'text-left'} {props.sortable ? 'cursor-pointer select-none hover:bg-[var(--color-surface)]' : ''}"
              onclick={() => props.sortable && toggleSort(col.key)}
            >
              {col.label}
              {#if sortKey === col.key}
                <span>{sortDir === 'asc' ? ' &#9650;' : ' &#9660;'}</span>
              {/if}
            </th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each sortedRows() as row, i}
          <tr class="{props.striped && i % 2 === 1 ? 'bg-[var(--color-surface-subtle)]' : ''} border-b border-[var(--color-border)]">
            {#each props.columns as col}
              <td class="px-4 py-3 text-sm text-[var(--color-text-subtle)] {col.align === 'center' ? 'text-center' : col.align === 'right' ? 'text-right' : 'text-left'}">
                {row[col.key] ?? ''}
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</section>
