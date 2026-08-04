<script lang="ts">
  import type { TimelineProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import GeoText from '../GeoText.svelte';
  import IconPicker from '../IconPicker.svelte';
  import MediaPicker from '../MediaPicker.svelte';

  interface Props {
    props: TimelineProps;
    onchange: (v: TimelineProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  type Step = TimelineProps['steps'][number];

  function update<K extends keyof TimelineProps>(field: K, v: TimelineProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addStep() {
    update('steps', [...props.steps, { title: '', description: '' }]);
  }

  function removeStep(idx: number) {
    update('steps', props.steps.filter((_, i) => i !== idx));
  }

  function updateStep(idx: number, patch: Partial<Step>) {
    update('steps', props.steps.map((s, i) => (i === idx ? { ...s, ...patch } : s)));
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</label>
    <select value={props.layout} onchange={(e) => update('layout', e.currentTarget.value as TimelineProps['layout'])} class={inputClass}>
      <option value="horizontal">Horizontal</option>
      <option value="vertical">Vertical</option>
      <option value="zigzag">Zigzag</option>
    </select>
  </div>

  <div class="space-y-2">
    <label class="block text-xs font-medium text-[var(--color-muted)]">Steps</label>
    {#each props.steps as step, i (i)}
      <div class="space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">Step {i + 1}</span>
          <button type="button" onclick={() => removeStep(i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        <div>
          <label class="mb-1 block text-xs text-[var(--color-muted)]">Icon</label>
          <IconPicker value={step.icon} onchange={(v) => updateStep(i, { icon: v })} />
        </div>
        <div>
          <label class="mb-1 block text-xs text-[var(--color-muted)]">Title</label>
          <GeoText value={step.title} onchange={(v) => updateStep(i, { title: v })} {geoCtx} />
        </div>
        <div>
          <label class="mb-1 block text-xs text-[var(--color-muted)]">Description</label>
          <GeoText value={step.description ?? ''} onchange={(v) => updateStep(i, { description: v })} {geoCtx} />
        </div>
        <div>
          <label class="mb-1 block text-xs text-[var(--color-muted)]">Media</label>
          <MediaPicker value={step.media} onchange={(v) => updateStep(i, { media: v })} {mediaPickerComponent} />
        </div>
      </div>
    {/each}
    <button type="button" onclick={addStep} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add step</button>
  </div>
</div>
