<script lang="ts">
  import type { GuestHeroDestinationItemProps } from '../../../types';

  interface Props {
    props: GuestHeroDestinationItemProps;
    onchange: (v: GuestHeroDestinationItemProps) => void;
  }

  let { props, onchange }: Props = $props();

  function update<K extends keyof GuestHeroDestinationItemProps>(key: K, value: GuestHeroDestinationItemProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Name</span>
    <input
      class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
      value={props.name ?? ''}
      oninput={(e) => update('name', e.currentTarget.value || undefined)}
      placeholder="Country name"
    />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Country code</span>
    <input
      class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
      value={props.code ?? ''}
      oninput={(e) => update('code', e.currentTarget.value || undefined)}
      placeholder="us, ca, fr..."
    />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon URL</span>
    <input
      class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
      value={props.icon_url ?? ''}
      oninput={(e) => update('icon_url', e.currentTarget.value || null)}
      placeholder="https://..."
    />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Link</span>
    <input
      class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
      value={props.href ?? ''}
      oninput={(e) => update('href', e.currentTarget.value || undefined)}
      placeholder="/explore"
    />
  </label>
</div>
