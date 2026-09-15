<script lang="ts">
  import type { GuestHomeContainerProps } from '../../../types';

  interface Props {
    props: GuestHomeContainerProps;
    onchange: (v: GuestHomeContainerProps) => void;
  }
  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>This block renders the full guest homepage as a set of nested, editable sections.</p>
  <p>Children: {props.children?.length ?? 0}</p>
  <button
    type="button"
    class="rounded border border-[var(--color-border)] px-2 py-1 text-xs"
    onclick={() => onchange({ ...props, children: [] })}
  >
    Reset default sections
  </button>
</div>
