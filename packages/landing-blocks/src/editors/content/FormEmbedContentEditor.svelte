<script lang="ts">
  import type { FormEmbedProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import GeoText from '../GeoText.svelte';

  interface Props {
    props: FormEmbedProps;
    onchange: (v: FormEmbedProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  type Field = FormEmbedProps['fields'][number];

  function update<K extends keyof FormEmbedProps>(field: K, v: FormEmbedProps[K]) {
    onchange({ ...props, [field]: v });
  }

  function addField() {
    update('fields', [...props.fields, { name: '', label: '', type: 'text', required: false, placeholder: '', options: [] }]);
  }

  function removeField(idx: number) {
    update('fields', props.fields.filter((_, i) => i !== idx));
  }

  function updateField(idx: number, patch: Partial<Field>) {
    update('fields', props.fields.map((f, i) => (i === idx ? { ...f, ...patch } : f)));
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Preset</label>
    <select value={props.preset} onchange={(e) => update('preset', e.currentTarget.value as FormEmbedProps['preset'])} class={inputClass}>
      <option value="newsletter">Newsletter</option>
      <option value="waitlist">Waitlist</option>
      <option value="contact">Contact</option>
      <option value="custom">Custom</option>
    </select>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</label>
    <GeoText value={props.title ?? ''} onchange={(v) => update('title', v)} {geoCtx} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subtitle</label>
    <GeoText value={props.subtitle ?? ''} onchange={(v) => update('subtitle', v)} {geoCtx} />
  </div>

  <div class="space-y-2">
    <label class="block text-xs font-medium text-[var(--color-muted)]">Fields</label>
    {#each props.fields as field, i (i)}
      <div class="space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">Field {i + 1}</span>
          <button type="button" onclick={() => removeField(i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <input type="text" value={field.name} placeholder="name" oninput={(e) => updateField(i, { name: e.currentTarget.value })} class={inputClass} />
          <input type="text" value={field.label} placeholder="label" oninput={(e) => updateField(i, { label: e.currentTarget.value })} class={inputClass} />
        </div>
        <div class="grid grid-cols-2 gap-2">
          <select value={field.type} onchange={(e) => updateField(i, { type: e.currentTarget.value as Field['type'] })} class={inputClass}>
            <option value="text">text</option>
            <option value="email">email</option>
            <option value="textarea">textarea</option>
            <option value="select">select</option>
          </select>
          <label class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
            <input type="checkbox" checked={field.required} onchange={(e) => updateField(i, { required: e.currentTarget.checked })} /> Required
          </label>
        </div>
        <input type="text" value={field.placeholder ?? ''} placeholder="placeholder" oninput={(e) => updateField(i, { placeholder: e.currentTarget.value })} class={inputClass} />
        {#if field.type === 'select'}
          <input type="text" value={(field.options ?? []).join(', ')} placeholder="options (comma separated)" oninput={(e) => updateField(i, { options: e.currentTarget.value.split(',').map((s) => s.trim()).filter(Boolean) })} class={inputClass} />
        {/if}
      </div>
    {/each}
    <button type="button" onclick={addField} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add field</button>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Submit endpoint</label>
    <input type="text" value={props.submit_endpoint} oninput={(e) => update('submit_endpoint', e.currentTarget.value)} class={inputClass} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Submit label</label>
    <input type="text" value={props.submit_label} oninput={(e) => update('submit_label', e.currentTarget.value)} class={inputClass} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Success message</label>
    <GeoText value={props.success_message} onchange={(v) => update('success_message', v)} {geoCtx} />
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Error message</label>
    <GeoText value={props.error_message} onchange={(v) => update('error_message', v)} {geoCtx} />
  </div>
</div>
