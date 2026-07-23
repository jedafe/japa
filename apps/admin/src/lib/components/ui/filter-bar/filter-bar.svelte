<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Select } from "$lib/components/ui/select";
  import { Search, X } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";
  import { t } from "$lib/stores/i18n.svelte";
  
  let {
    filters = [],
    value = $bindable({}),
    searchPlaceholder,
    onsearch,
    onclear,
    class: className
  }: {
    filters?: Array<{
      key: string;
      label: string;
      options: Array<{ value: string; label: string }>;
    }>;
    value?: Record<string, string>;
    searchPlaceholder?: string;
    onsearch?: (filters: Record<string, string>) => void;
    onclear?: () => void;
    class?: ClassValue;
  } = $props();
  
  let searchQuery = $state("");
  
  function handleSearch() {
    onsearch?.({ search: searchQuery, ...value });
  }
  
  function handleClear() {
    searchQuery = "";
    value = {};
    onclear?.();
  }
  
  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Enter") {
      handleSearch();
    }
  }
</script>

<div class={clsx("flex flex-wrap items-center gap-3", className)}>
  <div class="relative flex-1 min-w-[200px]">
    <Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted" />
    <input
      type="text"
      bind:value={searchQuery}
      placeholder={searchPlaceholder ?? t("common.search_placeholder", "Search...")}
      class="h-10 w-full rounded-lg border border bg-surface pl-10 pr-4 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
      onkeydown={handleKeydown}
    />
  </div>
  
  {#each filters as filter}
    <div class="w-40 shrink-0">
      <Select
        bind:value={value[filter.key]}
        options={filter.options}
        placeholder={filter.label}
      />
    </div>
  {/each}
  
  <Button variant="default" onclick={handleSearch}>
    <Search class="h-4 w-4 mr-2" />
    {t("common.search", "Search")}
  </Button>
  
  <Button variant="ghost" onclick={handleClear}>
    <X class="h-4 w-4 mr-2" />
    {t("common.clear", "Clear")}
  </Button>
</div>
