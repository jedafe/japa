<script lang="ts">
  import type { GuestToolItemProps } from '../../../types';

  interface Props {
    props: GuestToolItemProps;
    onchange: (v: GuestToolItemProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestToolItemProps>(key: K, value: GuestToolItemProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Name</span>
    <input class={inputClass} value={props.name ?? ''} oninput={(e) => update('name', e.currentTarget.value)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Slug</span>
    <input class={inputClass} value={props.slug ?? ''} oninput={(e) => update('slug', e.currentTarget.value || undefined)} placeholder="visa-checker" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Description</span>
    <input class={inputClass} value={props.description ?? ''} oninput={(e) => update('description', e.currentTarget.value || undefined)} />
  </label>
  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</span>
      <input class={inputClass} value={props.icon ?? ''} oninput={(e) => update('icon', e.currentTarget.value || undefined)} placeholder="Calculator" />
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Badge</span>
      <input class={inputClass} value={props.badge ?? ''} oninput={(e) => update('badge', e.currentTarget.value || undefined)} />
    </label>
  </div>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Accent color</span>
    <input class={inputClass} value={props.accent ?? ''} oninput={(e) => update('accent', e.currentTarget.value || undefined)} placeholder="#0ea5e9" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Link</span>
    <input class={inputClass} value={props.href ?? ''} oninput={(e) => update('href', e.currentTarget.value || undefined)} placeholder="/tools/visa-checker" />
  </label>
</div>
