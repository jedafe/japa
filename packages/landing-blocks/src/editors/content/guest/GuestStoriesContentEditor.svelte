<script lang="ts">
  import type { GuestStoriesProps, GuestHomeStory } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';

  interface Props {
    props: GuestStoriesProps;
    onchange: (v: GuestStoriesProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestStoriesProps>(key: K, value: GuestStoriesProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function defaultStory(): GuestHomeStory {
    return { id: crypto.randomUUID(), title: '', quote: '', author_name: '', author_role: '', author_avatar_url: '', href: '' };
  }

  const dataSource = $derived(props.data_source ?? 'context');
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} placeholder="Success Stories That Inspire" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subtitle</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.subtitle ?? ''} oninput={(e) => update('subtitle', e.currentTarget.value || undefined)} /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">View all link</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.view_all_href ?? ''} oninput={(e) => update('view_all_href', e.currentTarget.value || undefined)} placeholder="/stories" /></label>

  <div class="grid grid-cols-2 gap-2">
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</span>
      <select class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.layout ?? 'slider'} onchange={(e) => update('layout', e.currentTarget.value as 'grid' | 'slider')}>
        <option value="grid">Grid</option>
        <option value="slider">Slider</option>
      </select>
    </label>
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Columns</span><input type="number" min=1 max=6 class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.columns ?? 3} oninput={(e) => update('columns', Number(e.currentTarget.value))} /></label>
  </div>

  <div class="grid grid-cols-2 gap-2">
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Tablet columns</span><input type="number" min=1 max=6 class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.responsive_columns?.tablet ?? ''} oninput={(e) => update('responsive_columns', { ...props.responsive_columns, tablet: e.currentTarget.value ? Number(e.currentTarget.value) : undefined })} /></label>
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Mobile columns</span><input type="number" min=1 max=6 class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.responsive_columns?.mobile ?? ''} oninput={(e) => update('responsive_columns', { ...props.responsive_columns, mobile: e.currentTarget.value ? Number(e.currentTarget.value) : undefined })} /></label>
  </div>

  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Gap</span>
    <select class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.gap ?? 'md'} onchange={(e) => update('gap', e.currentTarget.value as GuestStoriesProps['gap'])}>
      <option value="none">none</option>
      <option value="xs">xs</option>
      <option value="sm">sm</option>
      <option value="md">md</option>
      <option value="lg">lg</option>
      <option value="xl">xl</option>
      <option value="2xl">2xl</option>
    </select>
  </label>

  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Card style</span>
    <select class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.card_style ?? 'default'} onchange={(e) => update('card_style', e.currentTarget.value as GuestStoriesProps['card_style'])}>
      <option value="default">default</option>
      <option value="outlined">outlined</option>
      <option value="filled">filled</option>
    </select>
  </label>

  <div class="flex items-center gap-2">
    <input type="checkbox" checked={dataSource === 'manual'} onchange={(e) => update('data_source', e.currentTarget.checked ? 'manual' : 'context')} id="stories-manual" />
    <label for="stories-manual" class="text-sm text-[var(--color-text)]">Use manual items</label>
  </div>

  {#if dataSource === 'manual'}
    <GuestCardListEditor items={props.items ?? []} onchange={(items) => update('items', items)} add={defaultStory} id="stories">
      {#snippet card(story: GuestHomeStory, updateStory, removeStory)}
        <div class="space-y-2">
          <textarea class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' rows=2 value={story.quote} oninput={(e) => updateStory({ ...story, quote: e.currentTarget.value })} placeholder="Quote"></textarea>
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={story.author_name} oninput={(e) => updateStory({ ...story, author_name: e.currentTarget.value })} placeholder="Author name" />
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={story.author_role} oninput={(e) => updateStory({ ...story, author_role: e.currentTarget.value })} placeholder="Author role" />
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={story.author_avatar_url} oninput={(e) => updateStory({ ...story, author_avatar_url: e.currentTarget.value })} placeholder="Avatar URL" />
          <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={story.href} oninput={(e) => updateStory({ ...story, href: e.currentTarget.value })} placeholder="Link" />
        </div>
      {/snippet}
    </GuestCardListEditor>
  {:else}
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Max items</span><input type="number" min=1 max=50 class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.max_items ?? 6} oninput={(e) => update('max_items', Number(e.currentTarget.value))} /></label>
  {/if}
</div>
