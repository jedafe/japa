<script lang="ts">
  import { t } from '$lib/stores/i18n.svelte';
  import { Select } from "bits-ui";
  import { Check, ChevronDown } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";

  let {
    options,
    value = $bindable(),
    placeholder,
    disabled = false,
    class: className,
    id,
    onchange
  }: {
    options: Array<{ value: string; label: string; disabled?: boolean }>;
    value?: string;
    placeholder?: string;
    disabled?: boolean;
    class?: ClassValue;
    id?: string;
    onchange?: (val: string) => void;
  } = $props();

  let uniqueOptions = $derived(
    Array.from(new Map(options.map(o => [o.value, o])).values())
  );

  let selectedLabel = $derived(
    uniqueOptions.find(o => o.value === value)?.label ?? (placeholder ?? t('common.select', 'Select...'))
  );

  let open = $state(false);

  function handleValueChange(val: string | undefined) {
    value = val;
    onchange?.(val ?? '');
  }
</script>

<Select.Root type="single" bind:value bind:open {disabled} items={uniqueOptions} onValueChange={handleValueChange}>
  <Select.Trigger
    {id}
    class={clsx(
      "flex h-10 w-full items-center justify-between rounded-lg border px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)] focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
      className
    )}
    style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
  >
    <Select.Value>
      <span class={clsx(!value && "text-[var(--color-muted)]")}>{selectedLabel}</span>
    </Select.Value>
    <ChevronDown class="h-4 w-4 opacity-50" />
  </Select.Trigger>

  <!-- Gate the portaled content on "open": bits-ui's presence layer can leave
       a closed-but-mounted popover behind (ghost overlay intercepting clicks)
       when a selection triggers a parent re-render; unmounting via "{#if}"
       guarantees cleanup. -->
  {#if open}
    <Select.Portal>
      <Select.Content
        class="relative z-50 max-h-96 min-w-[8rem] overflow-hidden rounded-lg border shadow-md data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
        style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
        sideOffset={4}
      >
        <Select.Viewport class="p-1">
          {#each uniqueOptions as option (option.value)}
            <Select.Item
              value={option.value}
              disabled={option.disabled}
              label={option.label}
              class="relative flex w-full cursor-default select-none items-center rounded-md py-1.5 ps-8 pe-2 text-sm outline-none focus:bg-[var(--color-accent)] focus:text-[var(--color-accent-foreground)] data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
            >
              {#snippet children({ selected })}
                <span class="absolute start-2 flex h-3.5 w-3.5 items-center justify-center">
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
  {/if}
</Select.Root>
