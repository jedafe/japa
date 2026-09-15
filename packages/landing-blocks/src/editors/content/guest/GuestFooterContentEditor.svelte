<script lang="ts">
  import type { GuestFooterProps } from '../../../types';

  interface Props {
    props: GuestFooterProps;
    onchange: (v: GuestFooterProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestFooterContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
