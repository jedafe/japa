<script lang="ts">
  import type { ContainerProps, Block, GeoContext } from '../../types';
  import type { FetchContext } from '../../data/types';
  import type { Component } from 'svelte';
  import { blockRegistry } from '../../render/blockRegistry';
  import NestedBlocksEditor from '../NestedBlocksEditor.svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: ContainerProps;
    onchange: (v: ContainerProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  function update<K extends keyof ContainerProps>(field: K, v: ContainerProps[K]) {
    onchange({ ...props, [field]: v });
  }

  // All block types are allowed as children — containers can hold anything,
  // including other containers (arbitrary nesting depth).
  const allowedChildTypes = Object.keys(blockRegistry) as Block['type'][];

  function getIcon(name: string): Component | null {
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
  const labelClass = 'mb-1 block text-xs font-medium text-[var(--color-muted)]';
</script>

<div class="space-y-4">
  <!-- Layout mode -->
  <div>
    <span class={labelClass}>Layout mode</span>
    <div class="grid grid-cols-3 gap-1.5">
      {#each ['stack', 'grid', 'flex'] as mode}
        <button
          type="button"
          onclick={() => update('layout', mode as ContainerProps['layout'])}
          class="rounded-[var(--radius)] border px-2 py-1.5 text-xs font-medium transition-colors"
          style={props.layout === mode
            ? 'border-color: var(--color-primary); background: var(--color-primary); color: white'
            : 'border-color: var(--color-border); color: var(--color-text)'}
        >
          {mode.charAt(0).toUpperCase() + mode.slice(1)}
        </button>
      {/each}
    </div>
    <p class="mt-1 text-[10px] text-[var(--color-muted)]">
      {#if props.layout === 'stack'}Vertical stack — children flow top to bottom
      {:else if props.layout === 'grid'}12-column grid — set per-child span for side-by-side
      {:else}Horizontal flex — children flow left to right with wrap
      {/if}
    </p>
  </div>

  <!-- Columns (grid mode only) — desktop + per-device overrides -->
  {#if props.layout === 'grid'}
    <div class="space-y-3">
      <!-- Desktop columns -->
      <div>
        <span class={labelClass}>Desktop columns: {props.columns}</span>
        <input
          type="range"
          min="1"
          max="12"
          value={props.columns}
          oninput={(e) => update('columns', Number(e.currentTarget.value))}
          class="w-full"
        />
        <p class="mt-1 text-[10px] text-[var(--color-muted)]">
          Children without an explicit span will each get 12÷{props.columns} = {Math.floor(12 / props.columns)} columns.
        </p>
      </div>

      <!-- Responsive column overrides -->
      <div class="grid grid-cols-2 gap-3">
        <div>
          <span class={labelClass}>
            Tablet columns
            {#if props.responsive_columns?.tablet !== undefined}
              <button type="button" class="ms-1 text-[10px] text-[var(--color-danger)] hover:underline" onclick={() => { const rc = { ...props.responsive_columns }; delete rc.tablet; update('responsive_columns', Object.keys(rc).length > 0 ? rc : undefined); }}>reset</button>
            {/if}
          </span>
          <input
            type="range"
            min="1"
            max="12"
            value={props.responsive_columns?.tablet ?? props.columns}
            oninput={(e) => update('responsive_columns', { ...props.responsive_columns, tablet: Number(e.currentTarget.value) })}
            class="w-full"
          />
          <p class="mt-0.5 text-[10px] text-[var(--color-muted)]">{props.responsive_columns?.tablet ?? props.columns} cols ≤ 1024px</p>
        </div>
        <div>
          <span class={labelClass}>
            Mobile columns
            {#if props.responsive_columns?.mobile !== undefined}
              <button type="button" class="ms-1 text-[10px] text-[var(--color-danger)] hover:underline" onclick={() => { const rc = { ...props.responsive_columns }; delete rc.mobile; update('responsive_columns', Object.keys(rc).length > 0 ? rc : undefined); }}>reset</button>
            {/if}
          </span>
          <input
            type="range"
            min="1"
            max="12"
            value={props.responsive_columns?.mobile ?? props.columns}
            oninput={(e) => update('responsive_columns', { ...props.responsive_columns, mobile: Number(e.currentTarget.value) })}
            class="w-full"
          />
          <p class="mt-0.5 text-[10px] text-[var(--color-muted)]">{props.responsive_columns?.mobile ?? props.columns} cols ≤ 768px</p>
        </div>
      </div>
    </div>
  {/if}

  <!-- Gap -->
  <div>
    <span class={labelClass}>Gap</span>
    <select value={props.gap} onchange={(e) => update('gap', e.currentTarget.value as ContainerProps['gap'])} class={inputClass}>
      {#each ['none', 'xs', 'sm', 'md', 'lg', 'xl', '2xl'] as g}
        <option value={g}>{g === 'none' ? 'None' : g.toUpperCase()}</option>
      {/each}
    </select>
  </div>

  <!-- Flex-only options -->
  {#if props.layout === 'flex'}
    <div class="grid grid-cols-2 gap-2">
      <div>
        <span class={labelClass}>Align</span>
        <select value={props.align_items ?? 'start'} onchange={(e) => update('align_items', e.currentTarget.value as ContainerProps['align_items'])} class={inputClass}>
          <option value="start">Start</option>
          <option value="center">Center</option>
          <option value="end">End</option>
          <option value="stretch">Stretch</option>
        </select>
      </div>
      <div>
        <span class={labelClass}>Justify</span>
        <select value={props.justify_content ?? 'start'} onchange={(e) => update('justify_content', e.currentTarget.value as ContainerProps['justify_content'])} class={inputClass}>
          <option value="start">Start</option>
          <option value="center">Center</option>
          <option value="end">End</option>
          <option value="between">Between</option>
          <option value="around">Around</option>
          <option value="evenly">Evenly</option>
        </select>
      </div>
    </div>
    <label class="flex items-center justify-between text-sm">
      <span style="color: var(--color-foreground)">Wrap</span>
      <input type="checkbox" checked={props.wrap ?? false} onchange={(e) => update('wrap', e.currentTarget.checked)} />
    </label>
  {/if}

  {#if props.layout === 'grid'}
    <div>
      <span class={labelClass}>Align items</span>
      <select value={props.align_items ?? 'stretch'} onchange={(e) => update('align_items', e.currentTarget.value as ContainerProps['align_items'])} class={inputClass}>
        <option value="stretch">Stretch (default)</option>
        <option value="start">Start</option>
        <option value="center">Center</option>
        <option value="end">End</option>
      </select>
    </div>
  {/if}

  <!-- Max width -->
  <div>
    <span class={labelClass}>Max width</span>
    <select value={props.max_width ?? 'full'} onchange={(e) => update('max_width', e.currentTarget.value as ContainerProps['max_width'])} class={inputClass}>
      <option value="full">Full width</option>
      <option value="xl">Extra Large</option>
      <option value="lg">Large</option>
      <option value="md">Medium</option>
      <option value="sm">Small</option>
    </select>
  </div>

  <!-- Min height -->
  <div>
    <span class={labelClass}>Min height (px, 0 = auto)</span>
    <input
      type="number"
      min="0"
      value={props.min_height === 'auto' ? 0 : props.min_height ?? 0}
      oninput={(e) => update('min_height', Number(e.currentTarget.value) || 'auto')}
      class={inputClass}
    />
  </div>

  <!-- Per-child span controls (grid mode only) -->
  {#if props.layout === 'grid' && props.children.length > 0}
    <div class="rounded-[var(--radius)] border border-[var(--color-border)] p-3">
      <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--color-muted)]">Column spans (1–12)</h4>
      <div class="space-y-2">
        {#each props.children as child, i (child.id)}
          {@const meta = blockRegistry[child.type]}
          <div class="flex items-center gap-2">
            {#if meta}
              {@const Icon = getIcon(meta.icon)}
              {#if Icon}<Icon size={12} />{/if}
            {/if}
            <span class="flex-1 truncate text-xs text-[var(--color-text)]">{meta?.label ?? child.type}</span>
            <input
              type="number"
              min="1"
              max="12"
              value={child.span ?? Math.floor(12 / props.columns)}
              oninput={(e) => {
                const span = Math.max(1, Math.min(12, Number(e.currentTarget.value)));
                const next = [...props.children];
                next[i] = { ...child, span };
                update('children', next);
              }}
              class="w-14 rounded border px-1 py-0.5 text-xs text-center"
              style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-text)"
              aria-label="Column span"
            />
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Child blocks -->
  <div>
    <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--color-muted)]">Child blocks</h4>
    <NestedBlocksEditor
      blocks={props.children ?? []}
      onchange={(v) => update('children', v)}
      allowedTypes={allowedChildTypes}
      {mediaPickerComponent}
      {geoCtx}
    />
  </div>
</div>
