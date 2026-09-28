<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { t } from "$lib/stores/i18n.svelte";
  import type { BulkAction } from "./types";

  let {
    selectedIds,
    selectedRows = [],
    actions = [],
    onclear,
  }: {
    selectedIds: Set<string>;
    selectedRows?: any[];
    actions?: BulkAction[];
    onclear?: () => void;
  } = $props();
</script>

{#if selectedIds.size > 0}
  <div
    data-bulk-bar
    class="mt-3 flex flex-wrap items-center gap-3 rounded-lg p-3"
    style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)"
  >
    <span class="text-sm font-medium" style="color: var(--color-foreground)">
      {t("common.selected", "{count} selected").replace(
        "{count}",
        String(selectedIds.size),
      )}
    </span>
    {#each actions as action, i (i)}
      <Button
        size="sm"
        variant={action.variant === "destructive" ? "destructive" : "default"}
        disabled={action.disabled?.(selectedIds, selectedRows) ?? false}
        onclick={() => action.onclick(selectedIds)}
      >
        {#if action.icon}<action.icon class="mr-1 h-3.5 w-3.5" />{/if}
        {action.label}
      </Button>
    {/each}
    <Button size="sm" variant="ghost" style="margin-left: auto" onclick={onclear}>
      {t("common.clear", "Clear")}
    </Button>
  </div>
{/if}
