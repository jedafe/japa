<script lang="ts">
  import type { GuestHeroStatsProps } from '../../../types';

  interface Props {
    props: GuestHeroStatsProps;
    onchange: (v: GuestHeroStatsProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestHeroStatsContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
