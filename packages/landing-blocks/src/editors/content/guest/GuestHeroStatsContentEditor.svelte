<script lang="ts">
  import type { GuestHeroStatsProps, GuestHomeStat } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';

  interface Props {
    props: GuestHeroStatsProps;
    onchange: (v: GuestHeroStatsProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroStatsProps>(key: K, value: GuestHeroStatsProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function defaultStat(): GuestHomeStat {
    return { id: crypto.randomUUID(), label: '', value: '', icon: 'Users' };
  }

  const dataSource = $derived(props.data_source ?? 'context');
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} /></label>

  <div class="flex items-center gap-2">
    <input type="checkbox" checked={dataSource === 'manual'} onchange={(e) => update('data_source', e.currentTarget.checked ? 'manual' : 'context')} id="stats-manual" />
    <label for="stats-manual" class="text-sm text-[var(--color-text)]">Use manual items</label>
  </div>

  {#if dataSource === 'manual'}
    <GuestCardListEditor items={props.items ?? []} onchange={(items) => update('items', items)} add={defaultStat} id="hero-stats">
      {#snippet card(stat: GuestHomeStat, updateStat, removeStat)}
        <div class="space-y-2">
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={stat.label} oninput={(e) => updateStat({ ...stat, label: e.currentTarget.value })} placeholder="Label" />
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={stat.value} oninput={(e) => updateStat({ ...stat, value: e.currentTarget.value })} placeholder="Value" />
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={stat.icon} oninput={(e) => updateStat({ ...stat, icon: e.currentTarget.value })} placeholder="Icon" />
        </div>
      {/snippet}
    </GuestCardListEditor>
  {:else}
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Max items</span><input type="number" min={1} max={50} class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.max_items ?? 4} oninput={(e) => update('max_items', Number(e.currentTarget.value))} /></label>
  {/if}
</div>
