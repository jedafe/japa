<script lang="ts">
  import type { GuestHeroDestinationsProps } from '../../../types';

  interface Props {
    props: GuestHeroDestinationsProps;
    onchange: (v: GuestHeroDestinationsProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestHeroDestinationsContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
