<script lang="ts">
  import type { HeadingProps } from '../../types';
  import type { Component } from 'svelte';
  import type { GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import GeoText from '../GeoText.svelte';
  import IconPicker from '../IconPicker.svelte';

  interface Props {
    props: HeadingProps;
    onchange: (v: HeadingProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof HeadingProps>(field: K, v: HeadingProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Heading text</span>
    <GeoText value={props.text} onchange={(v) => update('text', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Eyebrow (optional)</span>
    <GeoText value={props.eyebrow ?? ''} onchange={(v) => update('eyebrow', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subheading (optional)</span>
    <GeoText value={props.subheading ?? ''} onchange={(v) => update('subheading', v)} {geoCtx} />
  </label>

  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Heading level</span>
      <select class={inputClass} value={props.level ?? 'h2'} onchange={(e) => update('level', e.currentTarget.value as HeadingProps['level'])}>
        <option value="h1">H1</option>
        <option value="h2">H2</option>
        <option value="h3">H3</option>
        <option value="h4">H4</option>
        <option value="h5">H5</option>
        <option value="h6">H6</option>
      </select>
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Visual size</span>
      <select class={inputClass} value={props.size ?? 'xl'} onchange={(e) => update('size', e.currentTarget.value as HeadingProps['size'])}>
        <option value="xs">XS</option>
        <option value="sm">SM</option>
        <option value="md">MD</option>
        <option value="lg">LG</option>
        <option value="xl">XL</option>
        <option value="2xl">2XL</option>
        <option value="3xl">3XL</option>
        <option value="4xl">4XL</option>
      </select>
    </label>
  </div>

  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Alignment</span>
      <select class={inputClass} value={props.align ?? 'left'} onchange={(e) => update('align', e.currentTarget.value as HeadingProps['align'])}>
        <option value="left">Left</option>
        <option value="center">Center</option>
        <option value="right">Right</option>
      </select>
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Max width</span>
      <select class={inputClass} value={props.max_width ?? 'xl'} onchange={(e) => update('max_width', e.currentTarget.value as HeadingProps['max_width'])}>
        <option value="sm">Small</option>
        <option value="md">Medium</option>
        <option value="lg">Large</option>
        <option value="xl">Extra Large</option>
        <option value="full">Full</option>
      </select>
    </label>
  </div>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Accent style</span>
    <select class={inputClass} value={props.accent ?? 'none'} onchange={(e) => update('accent', e.currentTarget.value as HeadingProps['accent'])}>
      <option value="none">None</option>
      <option value="underline">Underline</option>
      <option value="highlight">Highlight</option>
      <option value="gradient">Gradient</option>
    </select>
  </label>

  {#if props.accent && props.accent !== 'none'}
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Accent color</span>
      <div class="flex items-center gap-2">
        <input type="color" value={props.accent_color ?? '#f97316'} oninput={(e) => update('accent_color', e.currentTarget.value)} class="h-8 w-12 rounded border border-[var(--color-border)]" />
        <input type="text" value={props.accent_color ?? ''} oninput={(e) => update('accent_color', e.currentTarget.value)} placeholder="var(--color-primary)" class={inputClass} />
      </div>
    </label>
  {/if}

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Icon (optional)</span>
    <IconPicker value={props.icon} onchange={(v) => update('icon', v)} />
  </label>

  <label class="block rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
    <span class="mb-2 block text-xs font-medium text-[var(--color-muted)]">CTA link (optional)</span>
    <div class="space-y-2">
      <input type="text" value={props.cta?.label ?? ''} oninput={(e) => update('cta', { ...(props.cta ?? { href: '' }), label: e.currentTarget.value })} placeholder="Label" class={inputClass} />
      <input type="text" value={props.cta?.href ?? ''} oninput={(e) => update('cta', { ...(props.cta ?? { label: '' }), href: e.currentTarget.value })} placeholder="/explore" class={inputClass} />
    </div>
  </label>
</div>
