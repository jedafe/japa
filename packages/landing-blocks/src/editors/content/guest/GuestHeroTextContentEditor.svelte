<script lang="ts">
  import type { GuestHeroTextProps } from '../../../types';

  interface Props {
    props: GuestHeroTextProps;
    onchange: (v: GuestHeroTextProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestHeroTextContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
