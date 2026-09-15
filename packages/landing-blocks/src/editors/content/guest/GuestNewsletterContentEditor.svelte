<script lang="ts">
  import type { GuestNewsletterProps } from '../../../types';

  interface Props {
    props: GuestNewsletterProps;
    onchange: (v: GuestNewsletterProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestNewsletterProps>(key: K, value: GuestNewsletterProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} placeholder="Get the Latest Global Opportunities" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subtitle</span><textarea class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' rows=2 value={props.subtitle ?? ''} oninput={(e) => update('subtitle', e.currentTarget.value || undefined)}></textarea></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">CTA</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.cta ?? ''} oninput={(e) => update('cta', e.currentTarget.value || undefined)} placeholder="Subscribe" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Placeholder</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.placeholder ?? ''} oninput={(e) => update('placeholder', e.currentTarget.value || undefined)} placeholder="Enter your email address" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Endpoint</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.endpoint ?? ''} oninput={(e) => update('endpoint', e.currentTarget.value || undefined)} placeholder="/newsletter/subscribe" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Success message</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.success_message ?? ''} oninput={(e) => update('success_message', e.currentTarget.value || undefined)} /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Error message</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.error_message ?? ''} oninput={(e) => update('error_message', e.currentTarget.value || undefined)} /></label>
</div>
