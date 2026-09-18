<script lang="ts">
  import type { GuestOpportunityItemProps } from '../../../types';

  interface Props {
    props: GuestOpportunityItemProps;
    onchange: (v: GuestOpportunityItemProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestOpportunityItemProps>(key: K, value: GuestOpportunityItemProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Type</span>
    <select class={inputClass} value={props.type ?? 'job'} onchange={(e) => update('type', e.currentTarget.value)}>
      <option value="job">Job</option>
      <option value="scholarship">Scholarship</option>
      <option value="visa">Visa</option>
      <option value="business">Business</option>
      <option value="internship">Internship</option>
    </select>
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span>
    <input class={inputClass} value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Organization</span>
    <input class={inputClass} value={props.organization ?? ''} oninput={(e) => update('organization', e.currentTarget.value)} />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Location</span>
    <input class={inputClass} value={props.location ?? ''} oninput={(e) => update('location', e.currentTarget.value)} placeholder="Toronto, Canada" />
  </label>
  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Tag</span>
      <input class={inputClass} value={props.tag ?? ''} oninput={(e) => update('tag', e.currentTarget.value)} placeholder="Job" />
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Badge</span>
      <input class={inputClass} value={props.badge ?? ''} oninput={(e) => update('badge', e.currentTarget.value || undefined)} placeholder="New" />
    </label>
  </div>
  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Value</span>
      <input class={inputClass} value={props.value ?? ''} oninput={(e) => update('value', e.currentTarget.value || undefined)} placeholder="$80k – $110k" />
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Published at</span>
      <input type="date" class={inputClass} value={props.published_at ?? ''} oninput={(e) => update('published_at', e.currentTarget.value || undefined)} />
    </label>
  </div>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Logo URL</span>
    <input class={inputClass} value={props.logo_url ?? ''} oninput={(e) => update('logo_url', e.currentTarget.value || null)} placeholder="https://..." />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Link</span>
    <input class={inputClass} value={props.href ?? ''} oninput={(e) => update('href', e.currentTarget.value)} placeholder="/jobs/software-engineer" />
  </label>
</div>
