<script lang="ts">
  import type { GuestHeroDestinationsProps, GuestHomeDestination } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';

  interface Props {
    props: GuestHeroDestinationsProps;
    onchange: (v: GuestHeroDestinationsProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroDestinationsProps>(key: K, value: GuestHeroDestinationsProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function defaultDestination(): GuestHomeDestination {
    return { id: crypto.randomUUID(), name: '', icon_url: null, href: '/explore' };
  }

  const dataSource = $derived(props.data_source ?? 'context');
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} placeholder="Top Destinations" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">See all link</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.see_all_href ?? ''} oninput={(e) => update('see_all_href', e.currentTarget.value || undefined)} placeholder="/explore" /></label>

  <div class="flex items-center gap-2">
    <input type="checkbox" checked={dataSource === 'manual'} onchange={(e) => update('data_source', e.currentTarget.checked ? 'manual' : 'context')} id="destinations-manual" />
    <label for="destinations-manual" class="text-sm text-[var(--color-text)]">Use manual items</label>
  </div>

  {#if dataSource === 'manual'}
    <GuestCardListEditor items={props.items ?? []} onchange={(items) => update('items', items)} add={defaultDestination} id="hero-destinations">
      {#snippet card(dest: GuestHomeDestination, updateDest, removeDest)}
        <div class="space-y-2">
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={dest.name} oninput={(e) => updateDest({ ...dest, name: e.currentTarget.value })} placeholder="Name" />
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={dest.icon_url ?? ''} oninput={(e) => updateDest({ ...dest, icon_url: e.currentTarget.value || null })} placeholder="Icon URL" />
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={dest.href ?? ''} oninput={(e) => updateDest({ ...dest, href: e.currentTarget.value })} placeholder="Link" />
        </div>
      {/snippet}
    </GuestCardListEditor>
  {:else}
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Max items</span><input type="number" min={1} max={50} class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.max_items ?? 4} oninput={(e) => update('max_items', Number(e.currentTarget.value))} /></label>
  {/if}
</div>
