<script lang="ts">
  import type { GuestHeroCommunityMemberProps } from '../../../types';

  interface Props {
    props: GuestHeroCommunityMemberProps;
    onchange: (v: GuestHeroCommunityMemberProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroCommunityMemberProps>(key: K, value: GuestHeroCommunityMemberProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Name</span>
    <input class={inputClass} value={props.name ?? ''} oninput={(e) => update('name', e.currentTarget.value || undefined)} placeholder="Member name" />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Image URL</span>
    <input class={inputClass} value={props.image_url ?? ''} oninput={(e) => update('image_url', e.currentTarget.value || null)} placeholder="https://..." />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Link</span>
    <input class={inputClass} value={props.href ?? ''} oninput={(e) => update('href', e.currentTarget.value || undefined)} placeholder="/explore" />
  </label>
</div>
