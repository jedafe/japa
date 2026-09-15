<script lang="ts">
  import type { GuestToolsProps } from '../../../types';

  interface Props {
    props: GuestToolsProps;
    onchange: (v: GuestToolsProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestToolsContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
