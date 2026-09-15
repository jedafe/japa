<script lang="ts">
  import type { GuestCategoriesProps, GuestHomeCategory } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';

  interface Props {
    props: GuestCategoriesProps;
    onchange: (v: GuestCategoriesProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestCategoriesProps>(key: K, value: GuestCategoriesProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function defaultCategory(): GuestHomeCategory {
    return { id: crypto.randomUUID(), title: '', description: '', href: '#', icon: 'Globe', color: 'blue' };
  }

  const dataSource = $derived(props.data_source ?? 'context');
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span>
    <input
      class={inputClass}
      value={props.title ?? ''}
      oninput={(e) => update('title', e.currentTarget.value || undefined)}
      placeholder="Explore What Matters to You"
    />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subtitle</span>
    <input
      class={inputClass}
      value={props.subtitle ?? ''}
      oninput={(e) => update('subtitle', e.currentTarget.value || undefined)}
      placeholder="Everything you need to achieve your global dreams."
    />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">View all link</span>
    <input
      class={inputClass}
      value={props.view_all_href ?? ''}
      oninput={(e) => update('view_all_href', e.currentTarget.value || undefined)}
      placeholder="/explore"
    />
  </label>

  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</span>
      <select
        class={inputClass}
        value={props.layout ?? 'grid'}
        onchange={(e) => update('layout', e.currentTarget.value as 'grid' | 'slider')}
      >
        <option value="grid">Grid</option>
        <option value="slider">Slider</option>
      </select>
    </label>

    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Columns</span>
      <input
        type="number"
        min={1}
        max={6}
        class={inputClass}
        value={props.columns ?? 4}
        oninput={(e) => update('columns', Number(e.currentTarget.value))}
      />
    </label>
  </div>

  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Tablet columns</span>
      <input
        type="number"
        min={1}
        max={6}
        class={inputClass}
        value={props.responsive_columns?.tablet ?? ''}
        oninput={(e) =>
          update('responsive_columns', {
            ...props.responsive_columns,
            tablet: e.currentTarget.value ? Number(e.currentTarget.value) : undefined,
          })}
      />
    </label>

    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Mobile columns</span>
      <input
        type="number"
        min={1}
        max={6}
        class={inputClass}
        value={props.responsive_columns?.mobile ?? ''}
        oninput={(e) =>
          update('responsive_columns', {
            ...props.responsive_columns,
            mobile: e.currentTarget.value ? Number(e.currentTarget.value) : undefined,
          })}
      />
    </label>
  </div>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Gap</span>
    <select
      class={inputClass}
      value={props.gap ?? 'md'}
      onchange={(e) => update('gap', e.currentTarget.value as GuestCategoriesProps['gap'])}
    >
      <option value="none">none</option>
      <option value="xs">xs</option>
      <option value="sm">sm</option>
      <option value="md">md</option>
      <option value="lg">lg</option>
      <option value="xl">xl</option>
      <option value="2xl">2xl</option>
    </select>
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Card style</span>
    <select
      class={inputClass}
      value={props.card_style ?? 'default'}
      onchange={(e) => update('card_style', e.currentTarget.value as GuestCategoriesProps['card_style'])}
    >
      <option value="default">default</option>
      <option value="outlined">outlined</option>
      <option value="filled">filled</option>
    </select>
  </label>

  <div class="flex items-center gap-2">
    <input
      type="checkbox"
      checked={dataSource === 'manual'}
      onchange={(e) => update('data_source', e.currentTarget.checked ? 'manual' : 'context')}
      id="categories-manual"
    />
    <label for="categories-manual" class="text-sm text-[var(--color-text)]">Use manual items</label>
  </div>

  {#if dataSource === 'manual'}
    <GuestCardListEditor
      items={props.items ?? []}
      onchange={(items) => update('items', items)}
      add={defaultCategory}
      id="categories"
    >
      {#snippet card(cat: GuestHomeCategory, updateCard, removeCard)}
        <div class="space-y-2">
          <input
            class={inputClass}
            value={cat.title}
            oninput={(e) => updateCard({ ...cat, title: e.currentTarget.value })}
            placeholder="Title"
          />
          <input
            class={inputClass}
            value={cat.description}
            oninput={(e) => updateCard({ ...cat, description: e.currentTarget.value })}
            placeholder="Description"
          />
          <input
            class={inputClass}
            value={cat.href}
            oninput={(e) => updateCard({ ...cat, href: e.currentTarget.value })}
            placeholder="Link"
          />
          <div class="grid grid-cols-2 gap-2">
            <input
              class={inputClass}
              value={cat.icon}
              oninput={(e) => updateCard({ ...cat, icon: e.currentTarget.value })}
              placeholder="Icon"
            />
            <select
              class={inputClass}
              value={cat.color}
              onchange={(e) => updateCard({ ...cat, color: e.currentTarget.value })}
            >
              <option value="blue">blue</option>
              <option value="green">green</option>
              <option value="purple">purple</option>
              <option value="orange">orange</option>
              <option value="teal">teal</option>
            </select>
          </div>
        </div>
      {/snippet}
    </GuestCardListEditor>
  {:else}
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Max items</span>
      <input
        type="number"
        min={1}
        max={50}
        class={inputClass}
        value={props.max_items ?? 8}
        oninput={(e) => update('max_items', Number(e.currentTarget.value))}
      />
    </label>
  {/if}
</div>
