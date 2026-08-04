<script lang="ts">
  import type { EntityType } from '../types';
  import type { FetchContext } from '../data/types';
  import { fetchEntityCollection } from '../data/entityFetchers';

  interface Props {
    entityType: EntityType;
    value?: { id?: string; slug?: string };
    onchange: (v: { id?: string; slug?: string } | undefined) => void;
    fetchCtx?: FetchContext;
  }
  let { entityType, value, onchange, fetchCtx }: Props = $props();

  let search = $state('');
  let results = $state<Record<string, unknown>[]>([]);
  let loading = $state(false);
  let showSearch = $state(false);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  $effect(() => {
    if (!showSearch || !fetchCtx) return;
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(async () => {
      loading = true;
      try {
        const items = await fetchEntityCollection(
          entityType,
          { filters: search ? { q: search } : undefined, limit: 10 },
          fetchCtx
        );
        results = items as Record<string, unknown>[];
      } catch {
        results = [];
      } finally {
        loading = false;
      }
    }, 300);
    return () => { if (debounceTimer) clearTimeout(debounceTimer); };
  });

  const currentLabel = $derived(value?.id ?? value?.slug ?? '');
</script>

<div>
  {#if value && !showSearch}
    <div class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5">
      <span class="text-sm text-[var(--color-text)]">{currentLabel}</span>
      <div class="flex gap-2">
        <button type="button" onclick={() => { showSearch = true; }} class="text-xs text-[var(--color-primary)] hover:underline">Change</button>
        <button type="button" onclick={() => onchange(undefined)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
      </div>
    </div>
  {:else}
    <div class="space-y-2">
      <input
        type="search"
        bind:value={search}
        placeholder="Search {entityType}..."
        class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
      />
      {#if loading}
        <p class="text-xs text-[var(--color-muted)]">Searching...</p>
      {:else}
        <div class="max-h-48 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)]">
          {#each results as result}
            <button
              type="button"
              onclick={() => { onchange({ id: result.id as string, slug: result.slug as string | undefined }); showSearch = false; }}
              class="block w-full px-3 py-1.5 text-left text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
            >
              {(result.title ?? result.name ?? result.id) as string}
            </button>
          {/each}
          {#if results.length === 0}
            <p class="px-3 py-2 text-xs text-[var(--color-muted)]">No results</p>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>
