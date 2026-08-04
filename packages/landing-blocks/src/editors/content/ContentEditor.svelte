<script lang="ts">
  import type { ContentProps, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';

  interface Props {
    props: ContentProps;
    onchange: (v: ContentProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof ContentProps>(field: K, v: ContentProps[K]) {
    onchange({ ...props, [field]: v });
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
</script>

<div class="space-y-3">
  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">HTML content</label>
    <textarea
      value={props.html}
      oninput={(e) => update('html', e.currentTarget.value)}
      rows="8"
      class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 font-mono text-sm text-[var(--color-text)]"
    ></textarea>
  </div>

  <div>
    <label class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Prose style</label>
    <select value={props.prose_style} onchange={(e) => update('prose_style', e.currentTarget.value as ContentProps['prose_style'])} class={inputClass}>
      <option value="default">Default</option>
      <option value="minimal">Minimal</option>
      <option value="article">Article</option>
      <option value="documentation">Documentation</option>
    </select>
  </div>
</div>
