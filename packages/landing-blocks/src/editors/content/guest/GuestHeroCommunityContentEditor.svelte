<script lang="ts">
  import type { GuestHeroCommunityProps } from '../../../types';

  interface Props {
    props: GuestHeroCommunityProps;
    onchange: (v: GuestHeroCommunityProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestHeroCommunityContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
