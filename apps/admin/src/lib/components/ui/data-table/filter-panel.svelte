<script lang="ts">
  import { Accordion } from "bits-ui";
  import { Select } from "$lib/components/ui/select";
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
          <div class="w-44">
            <Select
              bind:value={value[filter.key]}
              options={filter.options}
              placeholder={filter.label}
            />
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
