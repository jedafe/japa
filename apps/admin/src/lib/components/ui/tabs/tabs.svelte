<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
  import { Tabs } from "bits-ui";
  import { clsx, type ClassValue } from "clsx";
  
  let {
    tabs,
    value = $bindable(),
    class: className,
    children
  }: {
    tabs: Array<{
      value: string;
      label: string;
      icon?: any;
      count?: number;
    }>;
    value?: string;
    class?: ClassValue;
    children?: any;
  } = $props();
  
  // Set initial value if not provided
  $effect(() => {
    if (!value && tabs.length > 0) {
      value = tabs[0].value;
    }
  });
</script>

<Tabs.Root bind:value class={clsx("w-full", className)}>
  <Tabs.List class="inline-flex h-11 items-center justify-center rounded-lg bg-gray-100 p-1 text-gray-500">
    {#each tabs as tab}
      <Tabs.Trigger
        value={tab.value}
        class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium ring-offset-white transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-white data-[state=active]:text-gray-950 data-[state=active]:shadow-sm"
      >
        {#if tab.icon}
          {@const Icon = tab.icon}
          <Icon class="mr-2 h-4 w-4" />
        {/if}
        {tab.label}
        {#if tab.count !== undefined}
          <span class="ml-2 rounded-full bg-gray-200 px-2 py-0.5 text-xs font-medium text-gray-600">
            {tab.count}
          </span>
        {/if}
      </Tabs.Trigger>
    {/each}
  </Tabs.List>
  
  {@render children?.()}
</Tabs.Root>
