<script lang="ts">
  import type { ComingSoonProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import GeoText from '../GeoText.svelte';
  import IconPicker from '../IconPicker.svelte';
  import BackgroundPicker from '../BackgroundPicker.svelte';

  interface Props {
    props: ComingSoonProps;
    onchange: (v: ComingSoonProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof ComingSoonProps>(field: K, v: ComingSoonProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function updateWaitlist(patch: Partial<NonNullable<ComingSoonProps['waitlist_form']>>) {
    const current = props.waitlist_form ?? { enabled: false, cta_label: '', endpoint: '' };
    update('waitlist_form', { ...current, ...patch });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span>
    <GeoText value={props.title} onchange={(v) => update('title', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Description</span>
    <GeoText value={props.description ?? ''} onchange={(v) => update('description', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon</span>
    <IconPicker value={props.icon} onchange={(v) => update('icon', v)} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Estimated date</span>
    <input type="date" value={props.estimated_date ?? ''} oninput={(e) => update('estimated_date', e.currentTarget.value)} class={inputClass} />
  </label>

  <div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
    <label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
      <input type="checkbox" checked={props.waitlist_form?.enabled ?? false} onchange={(e) => updateWaitlist({ enabled: e.currentTarget.checked })} />
      Waitlist form
    </label>

    {#if props.waitlist_form?.enabled}
      <div class="mt-3 space-y-2">
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">CTA label</span>
          <input type="text" value={props.waitlist_form.cta_label} oninput={(e) => updateWaitlist({ cta_label: e.currentTarget.value })} class={inputClass} />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Endpoint</span>
          <input type="text" value={props.waitlist_form.endpoint} oninput={(e) => updateWaitlist({ endpoint: e.currentTarget.value })} class={inputClass} />
        </label>
      </div>
    {/if}
  </div>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Background</span>
    <BackgroundPicker value={props.background} onchange={(v) => update('background', v)} {mediaPickerComponent} />
  </label>
</div>
