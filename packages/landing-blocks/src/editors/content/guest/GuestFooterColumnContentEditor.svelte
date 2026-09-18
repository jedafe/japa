<script lang="ts">
  import type { GuestFooterColumnProps, Block } from '../../../types';
  import NestedBlocksEditor from '../../NestedBlocksEditor.svelte';

  interface Props {
    props: GuestFooterColumnProps;
    onchange: (v: GuestFooterColumnProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestFooterColumnProps>(key: K, value: GuestFooterColumnProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function updateChildren(children: Block[]) {
    onchange({ ...props, children });
  }
</script>

<div class="space-y-4">
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Column title</span>
    <input class={inputClass} value={props.title ?? ''} oninput={(e) => update('title', e.currentTarget.value)} placeholder="Platform" />
  </label>

  <div class="space-y-2">
    <span class="text-xs font-medium text-[var(--color-muted)]">Links</span>
    <NestedBlocksEditor
      blocks={props.children ?? []}
      allowedTypes={['guest_footer_link']}
      onchange={updateChildren}
    />
  </div>
</div>
