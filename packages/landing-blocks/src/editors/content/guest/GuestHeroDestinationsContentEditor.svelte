<script lang="ts">
  import type { GuestHeroDestinationsProps, GuestHomeDestination, Block } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';
  import NestedBlocksEditor from '../../NestedBlocksEditor.svelte';

  interface Props {
    props: GuestHeroDestinationsProps;
    onchange: (v: GuestHeroDestinationsProps) => void;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    widgetPickerComponent?: any;
  }

  let { props, onchange, widgetPickerComponent }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroDestinationsProps>(key: K, value: GuestHeroDestinationsProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function defaultDestination(): GuestHomeDestination {
    return { id: crypto.randomUUID(), name: '', icon_url: null, href: '/explore' };
  }

  function destinationToBlock(dest: GuestHomeDestination): Block {
    return {
      id: crypto.randomUUID(),
      type: 'guest_hero_destination_item',
      visible: true,
      props: {
        name: dest.name,
        icon_url: dest.icon_url,
        href: dest.href,
        code: dest.id,
      },
    } as Block;
  }

  function convertToNested() {
    const fromItems = props.items ?? [];
    const children = fromItems.map(destinationToBlock);
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
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span><input class={inputClass} value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} placeholder="Top Destinations" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">See all link</span><input class={inputClass} value={props.see_all_href ?? ''} oninput={(e) => update('see_all_href', e.currentTarget.value || undefined)} placeholder="/explore" /></label>

  <div class="flex items-center gap-2">
    <input type="checkbox" checked={dataSource === 'manual'} onchange={(e) => update('data_source', e.currentTarget.checked ? 'manual' : 'context')} id="destinations-manual" />
    <label for="destinations-manual" class="text-sm text-[var(--color-text)]">Use manual items</label>
  </div>

  {#if dataSource === 'manual'}
    {#if hasChildren}
      <div class="space-y-2">
        <p class="text-xs text-[var(--color-muted)]">Each destination is a nested block. Select one in the sidebar to edit its content and style.</p>
        <NestedBlocksEditor blocks={props.children ?? []} onchange={(children) => update('children', children)} allowedTypes={['guest_hero_destination_item']} />
      </div>
    {:else}
      <p class="text-xs text-[var(--color-muted)]">Add destination blocks from the sidebar, or use the legacy list below.</p>
      <GuestCardListEditor items={props.items ?? []} onchange={(items) => update('items', items)} add={defaultDestination} id="hero-destinations">
        {#snippet card(dest: GuestHomeDestination, updateDest, removeDest)}
          <div class="space-y-2">
            <input class={inputClass} value={dest.name} oninput={(e) => updateDest({ ...dest, name: e.currentTarget.value })} placeholder="Name" />
            <input class={inputClass} value={dest.icon_url ?? ''} oninput={(e) => updateDest({ ...dest, icon_url: e.currentTarget.value || null })} placeholder="Icon URL" />
            <input class={inputClass} value={dest.href ?? ''} oninput={(e) => updateDest({ ...dest, href: e.currentTarget.value })} placeholder="Link" />
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
        Optional: add a <b>Destination Item</b> block below as a template. It will be cloned for each top destination from the geo context, with name, icon, and country code injected.
        <span class="block text-[var(--color-danger)]" class:hidden={!hasChildren}>Only the first child is used as the template.</span>
      </p>
      <NestedBlocksEditor blocks={props.children ?? []} onchange={(children) => update('children', children)} allowedTypes={['guest_hero_destination_item']} />
    </div>
  {/if}
</div>
