<script lang="ts">
  import type { CtaBannerProps, CTA, GeoContext } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';
  import GeoText from '../GeoText.svelte';
  import CTAEditor from '../CTAEditor.svelte';

  interface Props {
    props: CtaBannerProps;
    onchange: (v: CtaBannerProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof CtaBannerProps>(field: K, v: CtaBannerProps[K]) {
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
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Headline</span>
    <GeoText value={props.headline} onchange={(v) => update('headline', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subhead</span>
    <GeoText value={props.subhead ?? ''} onchange={(v) => update('subhead', v)} {geoCtx} />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Alignment</span>
    <select
      value={props.alignment ?? 'center'}
      onchange={(e) => update('alignment', e.currentTarget.value as CtaBannerProps['alignment'])}
      class={inputClass}
    >
      <option value="left">Left</option>
      <option value="center">Center</option>
      <option value="right">Right</option>
    </select>
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
</div>
