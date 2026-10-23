<script lang="ts">
  import type { HeroProps, CTA, GeoContext, MediaRef } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';
  import GeoText from '../GeoText.svelte';
  import CTAEditor from '../CTAEditor.svelte';
  import MediaPicker from '../MediaPicker.svelte';

  interface Props {
    props: HeroProps;
    onchange: (v: HeroProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof HeroProps>(field: K, v: HeroProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addCta() {
    update('ctas', [...(props.ctas ?? []), { label: '', href: '', variant: 'primary' }]);
  }
  function removeCta(i: number) {
    update('ctas', (props.ctas ?? []).filter((_, idx) => idx !== i));
  }
  function updateCta(i: number, v: CTA) {
    const arr = [...(props.ctas ?? [])];
    arr[i] = v;
    update('ctas', arr);
  }

  function addStat() {
    update('stats', [...(props.stats ?? []), { label: '', value: '' }]);
  }
  function removeStat(i: number) {
    update('stats', (props.stats ?? []).filter((_, idx) => idx !== i));
  }
  function updateStat(i: number, field: 'label' | 'value', v: string) {
    const arr = [...(props.stats ?? [])];
    arr[i] = { ...arr[i], [field]: v };
    update('stats', arr);
  }

  function onMediaChange(m: MediaRef | undefined) {
    update('media', m);
  }

  function onSlideChange(i: number, m: MediaRef | undefined) {
    const slides = [...(props.slides ?? [])];
    if (m) {
      slides[i] = m;
    } else {
      slides.splice(i, 1);
    }
    update('slides', slides.length > 0 ? slides : undefined);
  }
  function addSlide() {
    const slides = [...(props.slides ?? []), { media_id: '', url: '', alt: '' }];
    update('slides', slides);
  }
  function removeSlide(i: number) {
    const slides = [...(props.slides ?? [])];
    slides.splice(i, 1);
    update('slides', slides.length > 0 ? slides : undefined);
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</span>
    <select
      value={props.layout ?? 'center'}
      onchange={(e) => update('layout', e.currentTarget.value as HeroProps['layout'])}
      class={inputClass}
    >
      <option value="center">Centered</option>
      <option value="split">Split (text + illustration)</option>
    </select>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Eyebrow</span>
    <GeoText value={props.eyebrow ?? ''} onchange={(v) => update('eyebrow', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Headline</span>
    <GeoText value={props.headline} onchange={(v) => update('headline', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Highlight word/phrase</span>
    <input
      type="text"
      value={props.highlight ?? ''}
      placeholder="Simplified."
      oninput={(e) => update('highlight', e.currentTarget.value)}
      class={inputClass}
    />
    <p class="mt-1 text-[10px] text-[var(--color-muted)]">The part of the headline to accent (e.g. in green).</p>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Tagline</span>
    <GeoText value={props.tagline ?? ''} onchange={(v) => update('tagline', v)} {geoCtx} />
    <p class="mt-1 text-[10px] text-[var(--color-muted)]">Short punchy line under the headline, e.g. "Study. Work. Live. Thrive Anywhere."</p>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subhead</span>
    <GeoText value={props.subhead ?? ''} onchange={(v) => update('subhead', v)} {geoCtx} />
  </label>

  {#if props.layout === 'split'}
    <label class="block">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-xs font-medium text-[var(--color-muted)]">Slides</span>
        <button type="button" onclick={addSlide} class="text-xs text-[var(--color-primary)] hover:underline">+ Add slide</button>
      </div>

      {#if (props.slides?.length ?? 0) > 0}
        <div class="space-y-3">
          {#each props.slides ?? [] as slide, i (i)}
            <div class="relative rounded-[var(--radius)] border border-[var(--color-border)] p-2">
              <button
                type="button"
                onclick={() => removeSlide(i)}
                class="absolute end-1 top-1 z-10 text-xs text-[var(--color-danger)] hover:underline"
              >Remove</button>
              <MediaPicker value={slide} onchange={(m) => onSlideChange(i, m)} {mediaPickerComponent} />
            </div>
          {/each}
        </div>
        <p class="mt-1 text-[10px] text-[var(--color-muted)]">Add two or more slides to enable the hero slider with arrows and dots.</p>
      {:else}
        <MediaPicker value={props.media} onchange={onMediaChange} {mediaPickerComponent} />
        <p class="mt-1 text-[10px] text-[var(--color-muted)]">Add a single hero image, or use the + Add slide button to create a slider. If empty, a generated illustration is used.</p>
      {/if}
    </label>
  {/if}

  <label class="block">
    <div class="mb-2 flex items-center justify-between">
      <span class="text-xs font-medium text-[var(--color-muted)]">CTAs</span>
      <button type="button" onclick={addCta} class="text-xs text-[var(--color-primary)] hover:underline">+ Add CTA</button>
    </div>
    <div class="space-y-3">
      {#each props.ctas ?? [] as cta, i}
        <div class="relative">
          <button
            type="button"
            onclick={() => removeCta(i)}
            class="absolute end-1 top-1 z-10 text-xs text-[var(--color-danger)] hover:underline"
          >Remove</button>
          <CTAEditor value={cta} onchange={(v) => updateCta(i, v)} />
        </div>
      {/each}
    </div>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Trust text</span>
    <GeoText value={props.trust_text ?? ''} onchange={(v) => update('trust_text', v)} {geoCtx} />
  </label>

  <label class="block">
    <div class="mb-2 flex items-center justify-between">
      <span class="text-xs font-medium text-[var(--color-muted)]">Stats</span>
      <button type="button" onclick={addStat} class="text-xs text-[var(--color-primary)] hover:underline">+ Add Stat</button>
    </div>
    <div class="space-y-2">
      {#each props.stats ?? [] as stat, i}
        <div class="flex gap-2">
          <input
            type="text"
            value={stat.value}
            placeholder="Value"
            oninput={(e) => updateStat(i, 'value', e.currentTarget.value)}
            class={inputClass}
          />
          <input
            type="text"
            value={stat.label}
            placeholder="Label"
            oninput={(e) => updateStat(i, 'label', e.currentTarget.value)}
            class={inputClass}
          />
          <button type="button" onclick={() => removeStat(i)} class="text-xs text-[var(--color-danger)]">✕</button>
        </div>
      {/each}
    </div>
  </label>
</div>
