<script lang="ts">
  import type { GuestToolsProps, GuestHomeTool, Block } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';
  import NestedBlocksEditor from '../../NestedBlocksEditor.svelte';

  interface Props {
    props: GuestToolsProps;
    onchange: (v: GuestToolsProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestToolsProps>(key: K, value: GuestToolsProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function defaultTool(): GuestHomeTool {
    return { id: crypto.randomUUID(), name: '', description: '', icon: 'Wrench', href: '', slug: '', accent: '' };
  }

  function toolToBlock(tool: GuestHomeTool): Block {
    return {
      id: crypto.randomUUID(),
      type: 'guest_tool_item',
      visible: true,
      props: {
        name: tool.name,
        slug: tool.slug,
        description: tool.description,
        href: tool.href,
        icon: tool.icon,
        badge: tool.badge,
        accent: tool.accent,
      },
    } as Block;
  }

  function convertToNested() {
    const children = (props.items ?? []).map(toolToBlock);
    onchange({ ...props, items: undefined, children });
  }

  // If children exist without an explicit source, assume manual so the sidebar
  // blocks actually drive the preview. When there are no children, default to context.
  const dataSource = $derived(
    props.data_source ?? (props.children && props.children.length > 0 ? 'manual' : 'context')
  );
  const hasChildren = $derived((props.children?.length ?? 0) > 0);
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} placeholder="Powerful Tools & Resources" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subtitle</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.subtitle ?? ''} oninput={(e) => update('subtitle', e.currentTarget.value || undefined)} /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">View all link</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.view_all_href ?? ''} oninput={(e) => update('view_all_href', e.currentTarget.value || undefined)} placeholder="/tools" /></label>

  <div class="grid grid-cols-2 gap-2">
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Layout</span>
      <select class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.layout ?? 'grid'} onchange={(e) => update('layout', e.currentTarget.value as 'grid' | 'slider')}>
        <option value="grid">Grid</option>
        <option value="slider">Slider</option>
      </select>
    </label>
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Columns</span><input type="number" min=1 max=6 class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.columns ?? 4} oninput={(e) => update('columns', Number(e.currentTarget.value))} /></label>
  </div>

  <div class="grid grid-cols-2 gap-2">
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Tablet columns</span><input type="number" min=1 max=6 class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.responsive_columns?.tablet ?? ''} oninput={(e) => update('responsive_columns', { ...props.responsive_columns, tablet: e.currentTarget.value ? Number(e.currentTarget.value) : undefined })} /></label>
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Mobile columns</span><input type="number" min=1 max=6 class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.responsive_columns?.mobile ?? ''} oninput={(e) => update('responsive_columns', { ...props.responsive_columns, mobile: e.currentTarget.value ? Number(e.currentTarget.value) : undefined })} /></label>
  </div>

  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Gap</span>
    <select class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.gap ?? 'md'} onchange={(e) => update('gap', e.currentTarget.value as GuestToolsProps['gap'])}>
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
    <select class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.card_style ?? 'default'} onchange={(e) => update('card_style', e.currentTarget.value as GuestToolsProps['card_style'])}>
      <option value="default">default</option>
      <option value="outlined">outlined</option>
      <option value="filled">filled</option>
    </select>
  </label>

  <div class="flex items-center gap-2">
    <input type="checkbox" checked={dataSource === 'manual'} onchange={(e) => update('data_source', e.currentTarget.checked ? 'manual' : 'context')} id="tools-manual" />
    <label for="tools-manual" class="text-sm text-[var(--color-text)]">Use manual items</label>
  </div>

  {#if dataSource === 'manual'}
    {#if hasChildren}
      <div class="space-y-2">
        <p class="text-xs text-[var(--color-muted)]">Each tool is a nested block. Select one in the sidebar to edit its content and style.</p>
        <NestedBlocksEditor blocks={props.children ?? []} onchange={(children) => update('children', children)} allowedTypes={['guest_tool_item']} />
      </div>
    {:else}
      <p class="text-xs text-[var(--color-muted)]">Add tool blocks from the sidebar, or use the legacy list below.</p>
      <GuestCardListEditor items={props.items ?? []} onchange={(items) => update('items', items)} add={defaultTool} id="tools">
        {#snippet card(tool: GuestHomeTool, updateTool, removeTool)}
          <div class="space-y-2">
            <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={tool.name} oninput={(e) => updateTool({ ...tool, name: e.currentTarget.value })} placeholder="Name" />
            <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={tool.description} oninput={(e) => updateTool({ ...tool, description: e.currentTarget.value })} placeholder="Description" />
            <div class="grid grid-cols-2 gap-2">
              <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={tool.icon} oninput={(e) => updateTool({ ...tool, icon: e.currentTarget.value })} placeholder="Icon" />
              <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={tool.accent} oninput={(e) => updateTool({ ...tool, accent: e.currentTarget.value })} placeholder="Accent color" />
            </div>
            <div class="grid grid-cols-2 gap-2">
              <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={tool.href} oninput={(e) => updateTool({ ...tool, href: e.currentTarget.value })} placeholder="Link" />
              <input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={tool.slug} oninput={(e) => updateTool({ ...tool, slug: e.currentTarget.value })} placeholder="Slug" />
            </div>
          </div>
        {/snippet}
      </GuestCardListEditor>
      {#if (props.items?.length ?? 0) > 0}
        <button
          type="button"
          onclick={convertToNested}
          class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface)]"
        >
          Convert to block items
        </button>
      {/if}
    {/if}
  {:else}
    <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Max items</span><input type="number" min=1 max=50 class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.max_items ?? 8} oninput={(e) => update('max_items', Number(e.currentTarget.value))} /></label>

    <div class="space-y-2">
      <p class="text-xs text-[var(--color-muted)]">
        Optional: add a <b>Tool Item</b> block below as a template. It will be cloned for each tool from the geo context, with name, description, icon, and accent injected.
        <span class="block text-[var(--color-danger)]" class:hidden={!hasChildren}>Only the first child is used as the template.</span>
      </p>
      <NestedBlocksEditor blocks={props.children ?? []} onchange={(children) => update('children', children)} allowedTypes={['guest_tool_item']} />
    </div>
  {/if}
</div>
