<script lang="ts">
  import type { TabsProps } from '../types';
  import type { Component } from 'svelte';
  import { Tabs } from 'bits-ui';
  import * as LucideIcons from 'lucide-svelte';
  import { blockRegistry } from '../render/blockRegistry';
  import BlockShell from '../shell/BlockShell.svelte';

  interface Props {
    props: TabsProps;
  }
  let { props }: Props = $props();

  function getIcon(name?: string): Component | null {
    if (!name) return null;
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-5xl">
    <Tabs.Root value={String(props.initial_tab ?? 0)} orientation={props.orientation}>
      <Tabs.List class={props.orientation === 'vertical' ? 'flex flex-col gap-1 w-48' : 'flex gap-1 ' + (props.layout === 'underline' ? 'border-b border-[var(--color-border)]' : '')}>
        {#each props.tabs as tab, i}
          <Tabs.Trigger
            value={String(i)}
            class="flex items-center gap-2 px-4 py-2 text-sm font-medium transition-colors
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
              {#if block.visible}
                {@const meta = blockRegistry[block.type]}
                {#if meta}
                  <BlockShell style={block.style} motion={block.motion} background={block.style?.background}>
                    {@const Component = meta.component}
                    <Component props={block.props} />
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
