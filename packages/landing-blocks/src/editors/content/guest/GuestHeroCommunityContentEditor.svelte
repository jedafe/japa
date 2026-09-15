<script lang="ts">
  import type { GuestHeroCommunityProps } from '../../../types';

  interface Props {
    props: GuestHeroCommunityProps;
    onchange: (v: GuestHeroCommunityProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroCommunityProps>(key: K, value: GuestHeroCommunityProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title template</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} placeholder="Join {value} {label}" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subtitle</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.subtitle ?? ''} oninput={(e) => update('subtitle', e.currentTarget.value || undefined)} placeholder="Your future starts with the right information." /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Value</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.value ?? ''} oninput={(e) => update('value', e.currentTarget.value || undefined)} placeholder="500K+" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.label ?? ''} oninput={(e) => update('label', e.currentTarget.value || undefined)} placeholder="members worldwide" /></label>

  <div class="space-y-2">
    <span class="text-xs font-medium text-[var(--color-muted)]">CTA</span>
    <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.cta?.label ?? ''} oninput={(e) => update('cta', { ...props.cta, label: e.currentTarget.value })} placeholder="Label" />
    <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.cta?.href ?? ''} oninput={(e) => update('cta', { ...props.cta, href: e.currentTarget.value })} placeholder="Href" />
    <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.cta?.icon ?? ''} oninput={(e) => update('cta', { ...props.cta, icon: e.currentTarget.value })} placeholder="Icon" />
  </div>
</div>
