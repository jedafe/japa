<script lang="ts">
  import type { GeoContext } from '../types';
  import { resolveString } from '../geo/resolveGeoCopy';
  import { DropdownMenu } from 'bits-ui';

  interface Props {
    value: string;
    onchange: (v: string) => void;
    geoCtx?: GeoContext;
    label?: string;
  }
  let { value, onchange, geoCtx, label }: Props = $props();

  let textareaEl = $state<HTMLTextAreaElement | null>(null);

  const placeholders = [
    { token: '{{country}}', label: 'Country name' },
    { token: '{{country_adjective}}', label: 'Country adjective' },
    { token: '{{continent}}', label: 'Continent' },
    { token: '{{diaspora_phrase}}', label: 'Diaspora phrase' },
    { token: '{{destination}}', label: 'Destination' },
  ];

  function insertPlaceholder(token: string) {
    if (!textareaEl) { value += token; onchange(value); return; }
    const start = textareaEl.selectionStart;
    const end = textareaEl.selectionEnd;
    value = value.substring(0, start) + token + value.substring(end);
    onchange(value);
  }

  const preview = $derived(geoCtx ? resolveString(value, geoCtx.tokens) : value);
</script>

<div>
  {#if label}
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{label}</label>
  {/if}
  <div class="flex gap-2">
    <textarea
      bind:this={textareaEl}
      bind:value
      oninput={() => onchange(value)}
      rows="2"
      class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
    ></textarea>
    <DropdownMenu.Root>
      <DropdownMenu.Trigger class="flex items-center rounded-[var(--radius)] border border-[var(--color-border)] px-2 text-xs text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">
        + Insert
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content class="z-50 min-w-[160px] rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
          {#each placeholders as p}
            <DropdownMenu.Item
              class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
              onSelect={() => insertPlaceholder(p.token)}
            >
              <span class="font-mono text-xs">{p.token}</span>
              <span class="ml-2 text-xs text-[var(--color-muted)]">{p.label}</span>
            </DropdownMenu.Item>
          {/each}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  </div>
  {#if geoCtx}
    <p class="mt-1 text-xs text-[var(--color-muted)]">Preview: {preview}</p>
  {/if}
</div>
