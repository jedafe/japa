<script lang="ts">
  import type { TabsProps } from '../types';
  import type { FetchContext } from '../data/types';
  import type { Component } from 'svelte';
  import { Tabs } from 'bits-ui';
  import * as LucideIcons from 'lucide-svelte';
  import { blockComponentFor } from '../render/blockComponent';
  import { isBlockVisible, isBlockEmpty } from '../render/blockVisibility';
  import BlockShell from '../shell/BlockShell.svelte';
  import ScopedCss from '../shell/ScopedCss.svelte';
  import { responsiveVisibilityCss } from '../shell/styleToCss';

  interface Props {
    props: TabsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props, fetchCtx, geoCountry }: Props = $props();

  const allNested = $derived(props.tabs.flatMap((tab) => tab.content_blocks ?? []));
  const visibilityCss = $derived(responsiveVisibilityCss(allNested));

  function getIcon(name?: string): Component | null {
    if (!name) return null;
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }
</script>

<ScopedCss css={visibilityCss} />

<section class="px-6 py-16">
  <div class="mx-auto max-w-5xl">
    <Tabs.Root value={String(props.initial_tab ?? 0)} orientation={props.orientation}>
      <!-- Mobile: the trigger row scrolls horizontally instead of overflowing the page. -->
      <Tabs.List class={props.orientation === 'vertical' ? 'flex flex-col gap-1 w-full sm:w-48' : 'flex gap-1 overflow-x-auto ' + (props.layout === 'underline' ? 'border-b border-[var(--color-border)]' : '')}>
        {#each props.tabs as tab, i}
          <Tabs.Trigger
            value={String(i)}
            class="flex shrink-0 items-center gap-2 whitespace-nowrap px-4 py-2 text-sm font-medium transition-colors
              {props.layout === 'underline'
                ? 'border-b-2 border-transparent text-[var(--color-muted)] hover:text-[var(--color-text)] data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-primary)]'
                : props.layout === 'pills'
                  ? 'rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)] hover:text-[var(--color-text)] data-[state=active]:bg-[var(--color-primary)] data-[state=active]:text-[var(--color-primary-contrast)]'
                  : 'rounded-t-[var(--radius)] border border-[var(--color-border)] border-b-0 text-[var(--color-muted)] hover:text-[var(--color-text)] data-[state=active]:bg-[var(--color-surface)] data-[state=active]:text-[var(--color-text)]'
              }"
          >
            {#if tab.icon}
              {@const Icon = getIcon(tab.icon)}
              {#if Icon}
                <Icon size={16} />
              {/if}
            {/if}
            {tab.label}
          </Tabs.Trigger>
        {/each}
      </Tabs.List>
      {#each props.tabs as tab, i}
        <Tabs.Content value={String(i)} class={props.orientation === 'vertical' ? 'flex-1 p-4' : 'pt-4'}>
          {#if tab.content_blocks}
            {#each tab.content_blocks as block (block.id)}
              {#if isBlockVisible(block, geoCountry)}
                {@const meta = blockComponentFor(block.type)}
                {#if meta}
                  {@const Component = meta.component}
                  <BlockShell
                    style={block.style}
                    motion={block.motion}
                    background={block.style?.background}
                    hideIfEmpty={block.visibility_rules?.hide_if_empty}
                    initialHasContent={!isBlockEmpty(block)}
                    scopeKey={block.id}
                  >
                    {#if block.type === 'custom'}
                      <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
                    {:else}
                      <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
                    {/if}
                  </BlockShell>
                {/if}
              {/if}
            {/each}
          {/if}
        </Tabs.Content>
      {/each}
    </Tabs.Root>
  </div>
</section>
