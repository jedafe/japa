<script lang="ts">
  import type { MediaRef } from '../types';
  import type { Component } from 'svelte';

  interface Props {
    value?: MediaRef;
    onchange: (v: MediaRef | undefined) => void;
    mediaPickerComponent?: Component;
  }
  let { value, onchange, mediaPickerComponent }: Props = $props();

  let open = $state(false);
</script>

<div>
  {#if value}
    <div class="flex items-center gap-3">
      <img src={value.url} alt={value.alt ?? ''} class="h-16 w-16 rounded-[var(--radius)] border border-[var(--color-border)] object-cover" />
      <div class="flex-1">
        <p class="text-sm text-[var(--color-text)]">{value.alt ?? 'Selected media'}</p>
        <div class="mt-1 flex gap-2">
          <button type="button" onclick={() => (open = true)} class="text-xs text-[var(--color-primary)] hover:underline">Change</button>
          <button type="button" onclick={() => onchange(undefined)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
      </div>
    </div>
  {:else}
    <button
      type="button"
      onclick={() => (open = true)}
      class="flex h-20 w-full items-center justify-center rounded-[var(--radius)] border border-dashed border-[var(--color-border)] text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
    >
      Select image
    </button>
  {/if}

  {#if mediaPickerComponent && open}
    {@const Picker = mediaPickerComponent}
    <Picker bind:open onselect={(m: { id: string; url: string; filename: string }) => { onchange({ media_id: m.id, url: m.url, alt: m.filename }); open = false; }} />
  {/if}
</div>
