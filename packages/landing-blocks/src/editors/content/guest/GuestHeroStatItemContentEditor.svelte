<script lang="ts">
  import type { GuestHeroStatItemProps } from '../../../types';

  interface Props {
    props: GuestHeroStatItemProps;
    onchange: (v: GuestHeroStatItemProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroStatItemProps>(key: K, value: GuestHeroStatItemProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Value</span>
    <input class={inputClass} value={props.value ?? ''} oninput={(e) => update('value', e.currentTarget.value)} placeholder="500K+" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</span>
    <input class={inputClass} value={props.label ?? ''} oninput={(e) => update('label', e.currentTarget.value)} placeholder="members worldwide" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</span>
    <input class={inputClass} value={props.icon ?? ''} oninput={(e) => update('icon', e.currentTarget.value || undefined)} placeholder="Users" />
  </label>
</div>
