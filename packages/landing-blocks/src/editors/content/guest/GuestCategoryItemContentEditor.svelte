<script lang="ts">
  import type { GuestCategoryItemProps } from '../../../types';

  interface Props {
    props: GuestCategoryItemProps;
    onchange: (v: GuestCategoryItemProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestCategoryItemProps>(key: K, value: GuestCategoryItemProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span>
    <input class={inputClass} value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Description</span>
    <input class={inputClass} value={props.description ?? ''} oninput={(e) => update('description', e.currentTarget.value)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Link</span>
    <input class={inputClass} value={props.href ?? ''} oninput={(e) => update('href', e.currentTarget.value)} placeholder="/jobs" />
  </label>
  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</span>
      <input class={inputClass} value={props.icon ?? ''} oninput={(e) => update('icon', e.currentTarget.value)} placeholder="Briefcase" />
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Color</span>
      <select class={inputClass} value={props.color ?? 'blue'} onchange={(e) => update('color', e.currentTarget.value)}>
        <option value="blue">Blue</option>
        <option value="green">Green</option>
        <option value="purple">Purple</option>
        <option value="orange">Orange</option>
        <option value="teal">Teal</option>
      </select>
    </label>
  </div>
</div>
