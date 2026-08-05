<script lang="ts">
  import type { HeroProps, CTA, GeoContext } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';
  import GeoText from '../GeoText.svelte';
  import CTAEditor from '../CTAEditor.svelte';

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
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Eyebrow</span>
    <GeoText value={props.eyebrow ?? ''} onchange={(v) => update('eyebrow', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Headline</span>
    <GeoText value={props.headline} onchange={(v) => update('headline', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subhead</span>
    <GeoText value={props.subhead ?? ''} onchange={(v) => update('subhead', v)} {geoCtx} />
  </label>

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
            class="absolute right-1 top-1 z-10 text-xs text-[var(--color-danger)] hover:underline"
          >Remove</button>
          <CTAEditor value={cta} onchange={(v) => updateCta(i, v)} />
        </div>
      {/each}
    </div>
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
