<script lang="ts">
  import type { TabsProps } from '../types';
  import type { FetchContext } from '../data/types';
  import type { Component } from 'svelte';
  import { Tabs } from 'bits-ui';
  import LazyIcon from '../lib/LazyIcon.svelte';
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

  // Vertical orientation renders as a sidenav: trigger column pinned left at
  // every breakpoint (sticky on sm+ so long tab content can scroll past it).
  const vertical = $derived(props.orientation === 'vertical');

  const triggerLayoutClass = $derived(
    vertical
      ? props.layout === 'underline'
        ? 'rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)] data-[state=active]:bg-[var(--color-surface-subtle)] data-[state=active]:text-[var(--color-text)]'
        : props.layout === 'pills'
          ? 'rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)] hover:text-[var(--color-text)] data-[state=active]:bg-[var(--color-primary)] data-[state=active]:text-[var(--color-primary-contrast)]'
          : 'rounded-[var(--radius)] border border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)] data-[state=active]:border-[var(--color-primary)] data-[state=active]:bg-[var(--color-surface)] data-[state=active]:text-[var(--color-text)]'
      : props.layout === 'underline'
        ? 'border-b-2 border-transparent text-[var(--color-muted)] hover:text-[var(--color-text)] data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-primary)]'
        : props.layout === 'pills'
          ? 'rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)] hover:text-[var(--color-text)] data-[state=active]:bg-[var(--color-primary)] data-[state=active]:text-[var(--color-primary-contrast)]'
          : 'rounded-t-[var(--radius)] border border-[var(--color-border)] border-b-0 text-[var(--color-muted)] hover:text-[var(--color-text)] data-[state=active]:bg-[var(--color-surface)] data-[state=active]:text-[var(--color-text)]'
  );
</script>

<ScopedCss css={visibilityCss} />

<section class="px-6 py-16">
  <div class="mx-auto max-w-5xl">
    <Tabs.Root value={String(props.initial_tab ?? 0)} orientation={props.orientation}>
      <div class={vertical ? 'flex flex-row items-start gap-4 sm:gap-6' : 'contents'}>
        <Tabs.List class={vertical ? 'flex w-36 shrink-0 flex-col gap-2 self-start sm:sticky sm:top-24 sm:w-56' : 'flex gap-1 overflow-x-auto ' + (props.layout === 'underline' ? 'border-b border-[var(--color-border)]' : '')}>
          {#each props.tabs as tab, i}
            <Tabs.Trigger
              value={String(i)}
              class="flex shrink-0 items-center gap-2 px-4 py-2 text-sm font-medium transition-colors
                {vertical ? 'w-full justify-start whitespace-normal text-start ' : 'whitespace-nowrap '}{triggerLayoutClass}"
            >
              {#if tab.icon}
                <LazyIcon name={tab.icon} size={16} />
              {/if}
              {tab.label}
            </Tabs.Trigger>
          {/each}
        </Tabs.List>
        {#each props.tabs as tab, i}
          <!-- Vertical: strip the first block section's top padding so the
               question column starts on the same row as the sidenav. -->
          <Tabs.Content value={String(i)} class={vertical ? 'min-w-0 flex-1 [&>*:first-child_section:first-child]:pt-0' : 'pt-4'}>
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
      </div>
    </Tabs.Root>
  </div>
</section>
