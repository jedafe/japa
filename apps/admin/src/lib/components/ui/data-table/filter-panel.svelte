<script lang="ts">
  import { Accordion } from "bits-ui";
  import Autocomplete from "$lib/components/ui/Autocomplete.svelte";
  import type { AutocompleteItem } from "$lib/components/ui/Autocomplete.svelte";
  import { Button } from "$lib/components/ui/button";
  import { Filter, X } from "lucide-svelte";
  import { t } from "$lib/stores/i18n.svelte";
  import type { FilterDef } from "./types";

  let {
    filters = [],
    value = $bindable({}),
    onapply,
    onclear,
  }: {
    filters?: FilterDef[];
    value?: Record<string, string>;
    onapply?: () => void;
    onclear?: () => void;
  } = $props();

  let accordionValue = $state("");
</script>

<Accordion.Root type="single" bind:value={accordionValue}>
  <Accordion.Item value="filters">
    <Accordion.Trigger
      class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium"
      style="color: var(--color-foreground)"
    >
      <Filter class="h-4 w-4" style="color: var(--color-muted)" />
      {t("common.filters", "Filters")}
      {#if Object.values(value).some((v) => v)}
        <span
          class="rounded-full px-2 py-0.5 text-xs"
          style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)"
        >
          {Object.values(value).filter(Boolean).length}
        </span>
      {/if}
    </Accordion.Trigger>
    <Accordion.Content class="space-y-3 px-3 pb-3">
      <div class="flex flex-wrap gap-3">
        {#each filters as filter (filter.key)}
          <div class={filter.type === 'autocomplete' ? 'w-64' : 'w-44'}>
            {#if filter.type === 'text'}
              <label for={`filter-${filter.key}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">
                {filter.label}
              </label>
              <input
                id={`filter-${filter.key}`}
                type="text"
                bind:value={value[filter.key]}
                placeholder={filter.placeholder ?? filter.label}
                class="w-full rounded-md border px-3 py-2 text-sm"
                style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
              />
            {:else if filter.type === 'autocomplete' && filter.fetcher}
              <Autocomplete
                id={`filter-${filter.key}`}
                placeholder={filter.placeholder ?? filter.label}
                emptyMessage={filter.emptyMessage ?? t('common.no_results', 'No results found')}
                fetcher={filter.fetcher}
                initialFetcher={filter.initialFetcher}
                selectedId={value[filter.key] || null}
                selectedLabel={value[filter.key] ? (filter.getSelectedLabel?.(value[filter.key]) ?? value[filter.key]) : null}
                onSelect={(item: AutocompleteItem) => { value[filter.key] = item.id; }}
                onClear={() => { value[filter.key] = ''; }}
              />
            {:else}
              <!-- Option filters render through Autocomplete (static options
                   filtered client-side) — every dropdown uses the shared
                   Autocomplete component. -->
              {@const opts = filter.options ?? []}
              {@const selVal = value[filter.key] ?? ''}
              <Autocomplete
                id={`filter-${filter.key}`}
                placeholder={filter.placeholder ?? filter.label}
                emptyMessage={filter.emptyMessage ?? t('common.no_results', 'No results found')}
                fetcher={async (q: string) => {
                  const needle = q.trim().toLowerCase();
                  return opts
                    .filter((o) => !needle || o.label.toLowerCase().includes(needle) || o.value.toLowerCase().includes(needle))
                    .map((o) => ({ id: o.value, label: o.label }));
                }}
                initialFetcher={async () => opts.map((o) => ({ id: o.value, label: o.label }))}
                selectedId={selVal || null}
                selectedLabel={selVal ? (opts.find((o) => o.value === selVal)?.label ?? selVal) : null}
                onSelect={(item: AutocompleteItem) => { value[filter.key] = item.id; }}
                onClear={() => { value[filter.key] = ''; }}
              />
            {/if}
          </div>
        {/each}
      </div>
      <div class="flex gap-2">
        <Button size="sm" onclick={onapply}>{t("common.apply", "Apply")}</Button>
        <Button size="sm" variant="ghost" onclick={onclear}>
          <X class="mr-1 h-3.5 w-3.5" />{t("common.clear", "Clear")}
        </Button>
      </div>
    </Accordion.Content>
  </Accordion.Item>
</Accordion.Root>
