<script lang="ts">
  import type { GuestFooterLinkProps } from '../../../types';

  interface Props {
    props: GuestFooterLinkProps;
    onchange: (v: GuestFooterLinkProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestFooterLinkProps>(key: K, value: GuestFooterLinkProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</span>
    <input class={inputClass} value={props.label ?? ''} oninput={(e) => update('label', e.currentTarget.value)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">URL</span>
    <input class={inputClass} value={props.url ?? ''} oninput={(e) => update('url', e.currentTarget.value || undefined)} placeholder="/help" />
  </label>
  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Target</span>
      <select class={inputClass} value={props.target ?? ''} onchange={(e) => update('target', e.currentTarget.value || undefined)}>
        <option value="">Same tab</option>
        <option value="_blank">New tab</option>
      </select>
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</span>
      <input class={inputClass} value={props.icon ?? ''} oninput={(e) => update('icon', e.currentTarget.value || undefined)} placeholder="Globe" />
    </label>
  </div>
</div>
