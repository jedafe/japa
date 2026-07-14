<script lang="ts">
  import { Select } from "bits-ui";
  import { Check, ChevronDown } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";
  
  let {
    options,
    value = $bindable(),
    placeholder = "Select...",
    disabled = false,
    class: className
  }: {
    options: Array<{ value: string; label: string; disabled?: boolean }>;
    value?: string;
    placeholder?: string;
    disabled?: boolean;
    class?: ClassValue;
  } = $props();
  
  let selectedLabel = $derived(
    options.find(o => o.value === value)?.label ?? placeholder
  );
</script>

<Select.Root type="single" bind:value {disabled} items={options}>
  <Select.Trigger
    class={clsx(
      "flex h-10 w-full items-center justify-between rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm ring-offset-white placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
      className
    )}
  >
    <Select.Value>
      <span class={clsx(!value && "text-gray-500")}>{selectedLabel}</span>
    </Select.Value>
    <ChevronDown class="h-4 w-4 opacity-50" />
  </Select.Trigger>
  
  <Select.Portal>
    <Select.Content
      class="relative z-50 max-h-96 min-w-[8rem] overflow-hidden rounded-lg border border-gray-200 bg-white text-gray-950 shadow-md data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
      sideOffset={4}
    >
      <Select.Viewport class="p-1">
        {#each options as option}
          <Select.Item
            value={option.value}
            disabled={option.disabled}
            label={option.label}
            class="relative flex w-full cursor-default select-none items-center rounded-md py-1.5 pl-8 pr-2 text-sm outline-none focus:bg-gray-100 focus:text-gray-900 data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
          >
            {#snippet children({ selected })}
              <span class="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
                {#if selected}
                  <Check class="h-4 w-4" />
                {/if}
              </span>
              {option.label}
            {/snippet}
          </Select.Item>
        {/each}
      </Select.Viewport>
    </Select.Content>
  </Select.Portal>
</Select.Root>
