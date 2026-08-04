<script lang="ts">
  import type { Component } from 'svelte';
  import { Popover } from 'bits-ui';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    value?: string;
    onchange: (v: string | undefined) => void;
  }
  let { value, onchange }: Props = $props();

  let search = $state('');
  let open = $state(false);

  // Get all icon names that are valid components
  const allIconNames = Object.keys(LucideIcons).filter(
    (name) => typeof (LucideIcons as Record<string, unknown>)[name] === 'object' || typeof (LucideIcons as Record<string, unknown>)[name] === 'function'
  );

  const filteredIcons = $derived(
    search
      ? allIconNames.filter((name) => name.toLowerCase().includes(search.toLowerCase())).slice(0, 100)
      : allIconNames.slice(0, 100)
  );

  function getIcon(name: string): Component | null {
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }

  const CurrentIcon = $derived(value ? getIcon(value) : null);
</script>

<Popover.Root bind:open>
  <Popover.Trigger class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
    {#if CurrentIcon}
      <CurrentIcon size={16} />
    {/if}
    <span>{value ?? 'Select icon'}</span>
  </Popover.Trigger>
  <Popover.Portal>
    <Popover.Content class="z-50 w-64 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg">
      <input
        type="search"
        bind:value={search}
        placeholder="Search icons..."
        class="mb-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
      />
      <div class="grid max-h-64 grid-cols-6 gap-1 overflow-y-auto">
        {#each filteredIcons as name}
          {@const Icon = getIcon(name)}
          {#if Icon}
            <button
              type="button"
              onclick={() => { onchange(name); open = false; }}
              class="flex h-8 w-8 items-center justify-center rounded-[var(--radius)] hover:bg-[var(--color-surface-subtle)] {value === name ? 'bg-[var(--color-surface-subtle)] ring-1 ring-[var(--color-primary)]' : ''}"
              title={name}
            >
              <Icon size={16} />
            </button>
          {/if}
        {/each}
      </div>
      {#if value}
        <button type="button" onclick={() => { onchange(undefined); open = false; }} class="mt-2 w-full text-xs text-[var(--color-danger)] hover:underline">
          Clear icon
        </button>
      {/if}
    </Popover.Content>
  </Popover.Portal>
</Popover.Root>
