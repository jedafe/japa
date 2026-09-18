<script lang="ts">
  import type { GuestFooterSocialProps } from '../../../types';

  interface Props {
    props: GuestFooterSocialProps;
    onchange: (v: GuestFooterSocialProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestFooterSocialProps>(key: K, value: GuestFooterSocialProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</span>
    <input class={inputClass} value={props.icon ?? ''} oninput={(e) => update('icon', e.currentTarget.value)} placeholder="Facebook, Twitter, Instagram, Youtube, Linkedin, or a Lucide name" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">URL</span>
    <input class={inputClass} value={props.url ?? ''} oninput={(e) => update('url', e.currentTarget.value)} placeholder="https://twitter.com/japaconnect" />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</span>
    <input class={inputClass} value={props.label ?? ''} oninput={(e) => update('label', e.currentTarget.value)} placeholder="Twitter" />
  </label>
</div>
