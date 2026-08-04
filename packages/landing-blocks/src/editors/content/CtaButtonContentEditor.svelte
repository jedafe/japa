<script lang="ts">
  import type { CtaButtonProps } from '../../types';
  import type { Component } from 'svelte';
  import type { GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import GeoText from '../GeoText.svelte';
  import IconPicker from '../IconPicker.svelte';

  interface Props {
    props: CtaButtonProps;
    onchange: (v: CtaButtonProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof CtaButtonProps>(field: K, v: CtaButtonProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</label>
    <GeoText value={props.label} onchange={(v) => update('label', v)} {geoCtx} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">URL</label>
    <input type="text" value={props.href} oninput={(e) => update('href', e.currentTarget.value)} placeholder="/explore" class={inputClass} />
  </div>

  <div class="grid grid-cols-2 gap-2">
    <div>
      <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Variant</label>
      <select class={inputClass} value={props.variant ?? 'primary'} onchange={(e) => update('variant', e.currentTarget.value as CtaButtonProps['variant'])}>
        <option value="primary">Primary</option>
        <option value="outline">Outline</option>
        <option value="ghost">Ghost</option>
        <option value="link">Link</option>
        <option value="gradient">Gradient</option>
      </select>
    </div>
    <div>
      <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Size</label>
      <select class={inputClass} value={props.size ?? 'lg'} onchange={(e) => update('size', e.currentTarget.value as CtaButtonProps['size'])}>
        <option value="sm">Small</option>
        <option value="md">Medium</option>
        <option value="lg">Large</option>
        <option value="xl">Extra Large</option>
      </select>
    </div>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Alignment</label>
    <select class={inputClass} value={props.align ?? 'center'} onchange={(e) => update('align', e.currentTarget.value as CtaButtonProps['align'])}>
      <option value="left">Left</option>
      <option value="center">Center</option>
      <option value="right">Right</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon (left, optional)</label>
    <IconPicker value={props.icon} onchange={(v) => update('icon', v)} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon (right, optional)</label>
    <IconPicker value={props.icon_right} onchange={(v) => update('icon_right', v)} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subtext (optional)</label>
    <GeoText value={props.subtext ?? ''} onchange={(v) => update('subtext', v)} {geoCtx} />
  </div>

  {#if props.variant === 'gradient'}
    <div class="grid grid-cols-2 gap-2">
      <div>
        <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Gradient from</label>
        <div class="flex items-center gap-2">
          <input type="color" value={props.gradient_from ?? '#f97316'} oninput={(e) => update('gradient_from', e.currentTarget.value)} class="h-8 w-12 rounded border border-[var(--color-border)]" />
          <input type="text" value={props.gradient_from ?? ''} oninput={(e) => update('gradient_from', e.currentTarget.value)} placeholder="#f97316" class={inputClass} />
        </div>
      </div>
      <div>
        <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Gradient to</label>
        <div class="flex items-center gap-2">
          <input type="color" value={props.gradient_to ?? '#a855f7'} oninput={(e) => update('gradient_to', e.currentTarget.value)} class="h-8 w-12 rounded border border-[var(--color-border)]" />
          <input type="text" value={props.gradient_to ?? ''} oninput={(e) => update('gradient_to', e.currentTarget.value)} placeholder="#a855f7" class={inputClass} />
        </div>
      </div>
    </div>
  {/if}

  <div class="flex items-center gap-4">
    <label class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.full_width ?? false} onchange={(e) => update('full_width', e.currentTarget.checked)} /> Full width
    </label>
    <label class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
      <input type="checkbox" checked={props.new_tab ?? false} onchange={(e) => update('new_tab', e.currentTarget.checked)} /> Open in new tab
    </label>
  </div>
</div>
