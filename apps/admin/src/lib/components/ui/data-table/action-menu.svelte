<script lang="ts">
  import { DropdownMenu } from 'bits-ui';
  import { MoreVertical } from 'lucide-svelte';
  import { clsx } from 'clsx';
  import type { ActionItem } from './types';
  import { t } from '$lib/stores/i18n.svelte';

  let { actions, row }: { actions: ActionItem[]; row: any } = $props();

  const visibleActions = $derived(actions.filter((a) => !a.show || a.show(row)));
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger
    data-action-menu-trigger
    class="inline-flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 dark:hover:bg-white/10"
    style="color: var(--color-muted)"
    aria-label={t('dataTable.rowActions', 'Row actions')}
  >
    <MoreVertical class="h-4 w-4" />
  </DropdownMenu.Trigger>
  <DropdownMenu.Portal>
    <DropdownMenu.Content
      class="z-50 min-w-[160px] rounded-md border p-1 shadow-lg"
      style="border-color: var(--color-border); background-color: var(--color-surface)"
      sideOffset={4}
    >
      {#each visibleActions as action (action.label)}
        <DropdownMenu.Item
          class={clsx(
            'flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm outline-none transition-colors hover:bg-black/5 dark:hover:bg-white/10',
            action.variant === 'destructive' && 'text-danger hover:bg-danger/10'
          )}
          style={action.variant === 'destructive' ? 'color: var(--color-danger)' : 'color: var(--color-foreground)'}
          onSelect={() => action.onclick(row)}
        >
          {#if action.icon}
            <action.icon class="h-4 w-4" />
          {/if}
          {action.label}
        </DropdownMenu.Item>
      {/each}
    </DropdownMenu.Content>
  </DropdownMenu.Portal>
</DropdownMenu.Root>
