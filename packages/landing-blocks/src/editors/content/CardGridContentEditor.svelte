<script lang="ts">
  import type { CardGridProps, CTA, GeoContext } from '../../types';
  import type { Component } from 'svelte';
  import type { FetchContext } from '../../data/types';
  import GeoText from '../GeoText.svelte';
  import CTAEditor from '../CTAEditor.svelte';
  import IconPicker from '../IconPicker.svelte';
  import MediaPicker from '../MediaPicker.svelte';

  interface Props {
    props: CardGridProps;
    onchange: (v: CardGridProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof CardGridProps>(field: K, v: CardGridProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addCard() {
    update('cards', [...(props.cards ?? []), { title: '' }]);
  }
  function removeCard(i: number) {
    update('cards', (props.cards ?? []).filter((_, idx) => idx !== i));
  }
  function updateCard(i: number, field: string, v: unknown) {
    const arr = [...(props.cards ?? [])];
    arr[i] = { ...arr[i], [field]: v };
    update('cards', arr);
  }
  function updateCardCta(i: number, v: CTA) {
    updateCard(i, 'cta', v);
  }
</script>

<div class="space-y-4">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Card variant</label>
    <select
      value={props.card_variant}
      onchange={(e) => update('card_variant', e.currentTarget.value as CardGridProps['card_variant'])}
      class={inputClass}
    >
      <option value="feature">Feature</option>
      <option value="testimonial">Testimonial</option>
      <option value="stat">Stat</option>
      <option value="custom">Custom</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</label>
    <select
      value={props.layout}
      onchange={(e) => update('layout', e.currentTarget.value as CardGridProps['layout'])}
      class={inputClass}
    >
      <option value="grid">Grid</option>
      <option value="alternating">Alternating</option>
      <option value="masonry">Masonry</option>
      <option value="carousel">Carousel</option>
      <option value="marquee">Marquee</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Columns: {props.columns}</label>
    <input
      type="range"
      min="1"
      max="6"
      value={props.columns}
      oninput={(e) => update('columns', Number(e.currentTarget.value))}
      class="w-full"
    />
  </div>

  <div>
    <div class="mb-2 flex items-center justify-between">
      <label class="text-xs font-medium text-[var(--color-muted)]">Cards</label>
      <button type="button" onclick={addCard} class="text-xs text-[var(--color-primary)] hover:underline">+ Add Card</button>
    </div>
    <div class="space-y-4">
      {#each props.cards ?? [] as card, i}
        <div class="space-y-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium text-[var(--color-muted)]">Card {i + 1}</span>
            <button type="button" onclick={() => removeCard(i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Icon</label>
            <IconPicker value={card.icon} onchange={(v) => updateCard(i, 'icon', v)} />
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Title</label>
            <GeoText value={card.title} onchange={(v) => updateCard(i, 'title', v)} {geoCtx} />
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Description</label>
            <GeoText value={card.description ?? ''} onchange={(v) => updateCard(i, 'description', v)} {geoCtx} />
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Media</label>
            <MediaPicker value={card.media} onchange={(v) => updateCard(i, 'media', v)} {mediaPickerComponent} />
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">CTA</label>
            <CTAEditor
              value={card.cta ?? { label: '', href: '', variant: 'primary' }}
              onchange={updateCardCta.bind(null, i)}
            />
          </div>

          <div>
            <label class="mb-1 block text-xs text-[var(--color-muted)]">Rating</label>
            <input
              type="number"
              min="0"
              max="5"
              step="0.1"
              value={card.rating ?? 0}
              oninput={(e) => updateCard(i, 'rating', Number(e.currentTarget.value))}
              class={inputClass}
            />
          </div>

          <div class="grid grid-cols-2 gap-2">
            <div>
              <label class="mb-1 block text-xs text-[var(--color-muted)]">Stat value</label>
              <input
                type="text"
                value={card.stat_value ?? ''}
                oninput={(e) => updateCard(i, 'stat_value', e.currentTarget.value)}
                class={inputClass}
              />
            </div>
            <div>
              <label class="mb-1 block text-xs text-[var(--color-muted)]">Stat label</label>
              <input
                type="text"
                value={card.stat_label ?? ''}
                oninput={(e) => updateCard(i, 'stat_label', e.currentTarget.value)}
                class={inputClass}
              />
            </div>
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>
