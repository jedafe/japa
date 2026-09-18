<script lang="ts">
  import type { GuestHeroCtaItemProps } from '../../../types';

  interface Props {
    props: GuestHeroCtaItemProps;
    onchange: (v: GuestHeroCtaItemProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroCtaItemProps>(key: K, value: GuestHeroCtaItemProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</span>
    <input class={inputClass} value={props.label ?? ''} oninput={(e) => update('label', e.currentTarget.value)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Link</span>
    <input class={inputClass} value={props.href ?? ''} oninput={(e) => update('href', e.currentTarget.value)} placeholder="/signup" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Variant</span>
    <select class={inputClass} value={props.variant ?? 'primary'} onchange={(e) => update('variant', e.currentTarget.value as GuestHeroCtaItemProps['variant'])}>
      <option value="primary">Primary</option>
      <option value="ghost">Ghost</option>
      <option value="outline">Outline</option>
      <option value="link">Link</option>
    </select>
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</span>
    <input class={inputClass} value={props.icon ?? ''} oninput={(e) => update('icon', e.currentTarget.value || undefined)} placeholder="ArrowRight" />
  </label>
</div>
