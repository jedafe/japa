<script lang="ts">
  import type { GuestStoryItemProps } from '../../../types';

  interface Props {
    props: GuestStoryItemProps;
    onchange: (v: GuestStoryItemProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestStoryItemProps>(key: K, value: GuestStoryItemProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span>
    <input class={inputClass} value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Quote</span>
    <textarea class={inputClass} rows={3} value={props.quote ?? ''} oninput={(e) => update('quote', e.currentTarget.value)}></textarea>
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Author name</span>
    <input class={inputClass} value={props.author_name ?? ''} oninput={(e) => update('author_name', e.currentTarget.value)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Author role</span>
    <input class={inputClass} value={props.author_role ?? ''} oninput={(e) => update('author_role', e.currentTarget.value || undefined)} placeholder="Moved to Canada" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Avatar URL</span>
    <input class={inputClass} value={props.author_avatar_url ?? ''} oninput={(e) => update('author_avatar_url', e.currentTarget.value || null)} placeholder="https://..." />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Link</span>
    <input class={inputClass} value={props.href ?? ''} oninput={(e) => update('href', e.currentTarget.value || undefined)} placeholder="/stories/amara" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Destination</span>
    <input class={inputClass} value={props.destination ?? ''} oninput={(e) => update('destination', e.currentTarget.value || undefined)} placeholder="Canada" />
  </label>
</div>
