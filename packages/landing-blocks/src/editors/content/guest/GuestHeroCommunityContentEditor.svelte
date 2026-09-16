<script lang="ts">
  import type { GuestHeroCommunityProps } from '../../../types';
  import NestedBlocksEditor from '../../NestedBlocksEditor.svelte';

  interface Props {
    props: GuestHeroCommunityProps;
    onchange: (v: GuestHeroCommunityProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroCommunityProps>(key: K, value: GuestHeroCommunityProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function setDataSource(source: 'context' | 'manual') {
    onchange({ ...props, data_source: source });
  }

  function updateChildren(children: NonNullable<GuestHeroCommunityProps['children']>) {
    onchange({ ...props, children });
  }
</script>


<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Title template</span><input class={inputClass} value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value || undefined)} placeholder={'Join {value} {label}'} /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subtitle</span><input class={inputClass} value={props.subtitle ?? ''} oninput={(e) => update('subtitle', e.currentTarget.value || undefined)} placeholder="Your future starts with the right information." /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Value</span><input class={inputClass} value={props.value ?? ''} oninput={(e) => update('value', e.currentTarget.value || undefined)} placeholder="500K+" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Label</span><input class={inputClass} value={props.label ?? ''} oninput={(e) => update('label', e.currentTarget.value || undefined)} placeholder="members worldwide" /></label>

  <div class="space-y-2">
    <span class="text-xs font-medium text-[var(--color-muted)]">CTA</span>
    <input class={inputClass} value={props.cta?.label ?? ''} oninput={(e) => update('cta', { ...props.cta, label: e.currentTarget.value })} placeholder="Label" />
    <input class={inputClass} value={props.cta?.href ?? ''} oninput={(e) => update('cta', { ...props.cta, href: e.currentTarget.value })} placeholder="Href" />
    <input class={inputClass} value={props.cta?.icon ?? ''} oninput={(e) => update('cta', { ...props.cta, icon: e.currentTarget.value })} placeholder="Icon" />
  </div>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Max items</span>
    <input type="number" class={inputClass} value={props.max_items ?? ''} oninput={(e) => update('max_items', e.currentTarget.value ? Number(e.currentTarget.value) : undefined)} placeholder="4" />
  </label>

  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Data source</span>
    <select class={inputClass} value={props.data_source ?? ''} onchange={(e) => setDataSource((e.currentTarget.value || 'context') as 'context' | 'manual')}>
      <option value="context">Context (stories)</option>
      <option value="manual">Manual members</option>
    </select>
  </label>

  {#if (props.data_source ?? 'context') === 'manual'}
    <div class="space-y-2">
      <span class="text-xs font-medium text-[var(--color-muted)]">Members</span>
      <NestedBlocksEditor
        blocks={props.children ?? []}
        allowedTypes={['guest_hero_community_member']}
        onchange={(children) => updateChildren(children)}
      />
    </div>
  {:else if (props.children ?? []).length > 0}
    <p class="text-xs text-[var(--color-muted)]">In context mode, the first member below is used as a template for each story avatar.</p>
  {/if}
</div>
