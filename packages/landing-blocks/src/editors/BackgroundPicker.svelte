<script lang="ts">
  import type { BackgroundConfig, MediaRef } from '../types';
  import type { Component } from 'svelte';
  import MediaPicker from './MediaPicker.svelte';

  interface Props {
    value?: BackgroundConfig;
    onchange: (v: BackgroundConfig) => void;
    mediaPickerComponent?: Component;
  }
  let { value, onchange, mediaPickerComponent }: Props = $props();

  const currentType = $derived(value?.type ?? 'none');

  function update(patch: Partial<BackgroundConfig>) {
    onchange({ ...(value ?? { type: 'none' }), ...patch });
  }

  function setType(type: BackgroundConfig['type']) {
    onchange({ type });
  }

  const types: Array<{ value: BackgroundConfig['type']; label: string }> = [
    { value: 'none', label: 'None' },
    { value: 'color', label: 'Color' },
    { value: 'gradient', label: 'Gradient' },
    { value: 'image', label: 'Image' },
    { value: 'video', label: 'Video' },
  ];
</script>

<div class="space-y-3">
  <div class="flex gap-1">
    {#each types as t}
      <button
        type="button"
        onclick={() => setType(t.value)}
        class="flex-1 rounded-[var(--radius)] px-2 py-1 text-xs font-medium {currentType === t.value ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]'}"
      >
        {t.label}
      </button>
    {/each}
  </div>

  {#if currentType === 'color'}
    <div class="flex items-center gap-2">
      <input
        type="color"
        value={value?.color ?? '#000000'}
        oninput={(e) => update({ color: e.currentTarget.value })}
        class="h-8 w-12 rounded border border-[var(--color-border)]"
      />
      <input
        type="text"
        value={value?.color ?? ''}
        oninput={(e) => update({ color: e.currentTarget.value })}
        placeholder="#000000 or var(--color)"
        class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]"
      />
    </div>
  {/if}

  {#if currentType === 'gradient'}
    <div class="space-y-2">
      <div class="flex items-center gap-2">
        <label class="text-xs text-[var(--color-muted)]">From</label>
        <input type="color" value={value?.gradient?.from ?? '#000000'} oninput={(e) => update({ gradient: { ...(value?.gradient ?? { from: '#000000', to: '#ffffff', angle: 90 }), from: e.currentTarget.value } })} class="h-8 w-12 rounded border border-[var(--color-border)]" />
        <label class="text-xs text-[var(--color-muted)]">To</label>
        <input type="color" value={value?.gradient?.to ?? '#ffffff'} oninput={(e) => update({ gradient: { ...(value?.gradient ?? { from: '#000000', to: '#ffffff', angle: 90 }), to: e.currentTarget.value } })} class="h-8 w-12 rounded border border-[var(--color-border)]" />
      </div>
      <div class="flex items-center gap-2">
        <label class="text-xs text-[var(--color-muted)]">Angle</label>
        <input type="range" min="0" max="360" value={value?.gradient?.angle ?? 90} oninput={(e) => update({ gradient: { ...(value?.gradient ?? { from: '#000000', to: '#ffffff', angle: 90 }), angle: Number(e.currentTarget.value) } })} class="flex-1" />
        <span class="text-xs text-[var(--color-muted)]">{value?.gradient?.angle ?? 90}°</span>
      </div>
    </div>
  {/if}

  {#if currentType === 'image'}
    <div class="space-y-2">
      <MediaPicker
        value={value?.image ? { media_id: value.image.media_id, url: value.image.url, alt: value.image.alt } : undefined}
        onchange={(m) => update({ image: { ...(value?.image ?? { fit: 'cover' }), ...(m ? { media_id: m.media_id, url: m.url, alt: m.alt ?? '' } : {}) } })}
        {mediaPickerComponent}
      />
      <div>
        <label class="mb-1 block text-xs text-[var(--color-muted)]">Fit</label>
        <select value={value?.image?.fit ?? 'cover'} onchange={(e) => update({ image: { ...(value?.image ?? { media_id: '', url: '', alt: '' }), fit: e.currentTarget.value as 'cover' | 'contain' | 'repeat' | 'fixed' } })} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]">
          <option value="cover">Cover</option>
          <option value="contain">Contain</option>
          <option value="repeat">Repeat</option>
          <option value="fixed">Fixed</option>
        </select>
      </div>
      <div class="flex items-center gap-2">
        <label class="text-xs text-[var(--color-muted)]">Overlay</label>
        <input type="color" value={value?.image?.overlay?.color ?? '#000000'} oninput={(e) => update({ image: { ...(value?.image ?? { media_id: '', url: '', alt: '', fit: 'cover' }), overlay: { color: e.currentTarget.value, opacity: value?.image?.overlay?.opacity ?? 50 } } })} class="h-8 w-12 rounded border border-[var(--color-border)]" />
        <input type="range" min="0" max="100" value={value?.image?.overlay?.opacity ?? 50} oninput={(e) => update({ image: { ...(value?.image ?? { media_id: '', url: '', alt: '', fit: 'cover' }), overlay: { color: value?.image?.overlay?.color ?? '#000000', opacity: Number(e.currentTarget.value) } } })} class="flex-1" />
        <span class="text-xs text-[var(--color-muted)]">{value?.image?.overlay?.opacity ?? 50}%</span>
      </div>
    </div>
  {/if}

  {#if currentType === 'video'}
    <div class="space-y-2">
      <MediaPicker
        value={value?.video ? { media_id: value.video.media_id, url: value.video.url, alt: '' } : undefined}
        onchange={(m) => update({ video: { ...(value?.video ?? { loop: false, muted: true, autoplay: false }), ...(m ? { media_id: m.media_id, url: m.url } : {}) } })}
        {mediaPickerComponent}
      />
      <div class="flex gap-4">
        <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
          <input type="checkbox" checked={value?.video?.loop ?? false} onchange={(e) => update({ video: { ...(value?.video ?? { media_id: '', url: '' }), loop: e.currentTarget.checked } })} /> Loop
        </label>
        <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
          <input type="checkbox" checked={value?.video?.muted ?? true} onchange={(e) => update({ video: { ...(value?.video ?? { media_id: '', url: '' }), muted: e.currentTarget.checked } })} /> Muted
        </label>
        <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
          <input type="checkbox" checked={value?.video?.autoplay ?? false} onchange={(e) => update({ video: { ...(value?.video ?? { media_id: '', url: '' }), autoplay: e.currentTarget.checked } })} /> Autoplay
        </label>
      </div>
    </div>
  {/if}
</div>
