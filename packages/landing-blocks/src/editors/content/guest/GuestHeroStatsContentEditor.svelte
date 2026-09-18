<script lang="ts">
  import type { GuestHeroStatsProps, GuestHomeStat, Block } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';
  import NestedBlocksEditor from '../../NestedBlocksEditor.svelte';

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

  function statToBlock(stat: GuestHomeStat): Block {
    return {
      id: crypto.randomUUID(),
      type: 'guest_hero_stat_item',
      visible: true,
      props: {
        value: stat.value,
        label: stat.label,
        icon: stat.icon,
      },
    } as Block;
  }

  function convertToNested() {
    const children = (props.items ?? []).map(statToBlock);
    onchange({ ...props, items: undefined, children });
  }

  // If children exist without an explicit source, assume manual so the sidebar
  // blocks actually drive the preview. When there are no children, default to context.
  const dataSource = $derived(
    props.data_source ?? (props.children && props.children.length > 0 ? 'manual' : 'context')
  );
  const hasChildren = $derived((props.children?.length ?? 0) > 0);
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span><input class={inputClass} value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} /></label>

  <div class="flex items-center gap-2">
    <input type="checkbox" checked={dataSource === 'manual'} onchange={(e) => update('data_source', e.currentTarget.checked ? 'manual' : 'context')} id="stats-manual" />
    <label for="stats-manual" class="text-sm text-[var(--color-text)]">Use manual items</label>
  </div>

  {#if dataSource === 'manual'}
    {#if hasChildren}
      <div class="space-y-2">
        <p class="text-xs text-[var(--color-muted)]">Each stat is a nested block. Select one in the sidebar to edit its content and style.</p>
        <NestedBlocksEditor blocks={props.children ?? []} onchange={(children) => update('children', children)} allowedTypes={['guest_hero_stat_item']} />
      </div>
    {:else}
      <p class="text-xs text-[var(--color-muted)]">Add stat blocks from the sidebar, or use the legacy list below.</p>
      <GuestCardListEditor items={props.items ?? []} onchange={(items) => update('items', items)} add={defaultStat} id="hero-stats">
        {#snippet card(stat: GuestHomeStat, updateStat, removeStat)}
          <div class="space-y-2">
            <input class={inputClass} value={stat.label} oninput={(e) => updateStat({ ...stat, label: e.currentTarget.value })} placeholder="Label" />
            <input class={inputClass} value={stat.value} oninput={(e) => updateStat({ ...stat, value: e.currentTarget.value })} placeholder="Value" />
            <input class={inputClass} value={stat.icon} oninput={(e) => updateStat({ ...stat, icon: e.currentTarget.value })} placeholder="Icon" />
          </div>
        {/snippet}
      </GuestCardListEditor>
      {#if (props.items?.length ?? 0) > 0}
        <button
          type="button"
          onclick={convertToNested}
          class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface)]"
        >
          Convert to block items
        </button>
      {/if}
    {/if}
  {:else}
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Max items</span><input type="number" min={1} max={50} class={inputClass} value={props.max_items ?? 4} oninput={(e) => update('max_items', Number(e.currentTarget.value))} /></label>

    <div class="space-y-2">
      <p class="text-xs text-[var(--color-muted)]">
        Optional: add a <b>Stat Item</b> block below as a template. It will be cloned for each stat from the geo context, with value, label, and icon injected.
        <span class="block text-[var(--color-danger)]" class:hidden={!hasChildren}>Only the first child is used as the template.</span>
      </p>
      <NestedBlocksEditor blocks={props.children ?? []} onchange={(children) => update('children', children)} allowedTypes={['guest_hero_stat_item']} />
    </div>
  {/if}
</div>
