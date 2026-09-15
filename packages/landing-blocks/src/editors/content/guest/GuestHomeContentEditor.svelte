<script lang="ts">
  import type { GuestHomeContainerProps } from '../../../types';

  interface Props {
    props: GuestHomeContainerProps;
    onchange: (v: GuestHomeContainerProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>This is the guest homepage container. Sections are managed as child blocks in the Page Builder tree.</p>
  <p>Children: {props.children?.length ?? 0}</p>
</div>
