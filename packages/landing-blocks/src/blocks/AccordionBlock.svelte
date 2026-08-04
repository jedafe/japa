<script lang="ts">
  import type { AccordionProps } from '../types';
  import { Accordion } from 'bits-ui';

  interface Props {
    props: AccordionProps;
  }
  let { props }: Props = $props();

  let searchQuery = $state('');

  const filteredItems = $derived(
    !props.searchable || !searchQuery
      ? props.items
      : props.items.filter((item) =>
          item.title.toLowerCase().includes(searchQuery.toLowerCase())
        )
  );
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-3xl">
    {#if props.searchable}
      <input
        type="search"
        bind:value={searchQuery}
        placeholder="Search..."
        class="mb-6 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
      />
    {/if}

    {#if props.layout === 'expanded'}
      <div class="space-y-6">
        {#each filteredItems as item, i}
          <div class="border-b border-[var(--color-border)] pb-6">
            <h3 class="text-lg font-semibold text-[var(--color-text)]">{item.title}</h3>
            <div class="mt-2 text-sm text-[var(--color-text-subtle)]">
              {#if item.content_blocks}
                <!-- Nested blocks would render here via BlockRenderer -->
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {:else}
      <Accordion.Root type="single" class="space-y-2">
        {#each filteredItems as item, i}
          <Accordion.Item value={String(i)} class="overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
            <Accordion.Header>
              <Accordion.Trigger class="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-semibold text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
                {item.title}
                <span class="transition-transform accordion-chevron">&#9662;</span>
              </Accordion.Trigger>
            </Accordion.Header>
            <Accordion.Content class="px-4 pb-3 text-sm text-[var(--color-text-subtle)]">
              {#if item.content_blocks}
                <!-- Nested blocks render here -->
              {/if}
            </Accordion.Content>
          </Accordion.Item>
        {/each}
      </Accordion.Root>
    {/if}
  </div>
</section>

<style>
  :global([data-state="open"]) .accordion-chevron {
    transform: rotate(180deg);
  }
</style>
