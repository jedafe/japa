<script lang="ts">
  import type { CTA } from '../types';
  import IconPicker from './IconPicker.svelte';

  interface Props {
    value: CTA;
    onchange: (v: CTA) => void;
  }
  let { value, onchange }: Props = $props();

  function update(field: keyof CTA, v: unknown) {
    onchange({ ...value, [field]: v });
  }
</script>

<div class="space-y-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</label>
    <input
      type="text"
      value={value.label}
      oninput={(e) => update('label', e.currentTarget.value)}
      class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
    />
  </div>
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">URL</label>
    <input
      type="text"
      value={value.href}
      oninput={(e) => update('href', e.currentTarget.value)}
      class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
    />
  </div>
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Variant</label>
    <select
      value={value.variant}
      onchange={(e) => update('variant', e.currentTarget.value)}
      class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
    >
      <option value="primary">Primary</option>
      <option value="outline">Outline</option>
      <option value="ghost">Ghost</option>
      <option value="link">Link</option>
    </select>
  </div>
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</label>
    <IconPicker value={value.icon} onchange={(v) => update('icon', v)} />
  </div>
</div>
