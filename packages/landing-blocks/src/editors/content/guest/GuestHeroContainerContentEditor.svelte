<script lang="ts">
  import type { GuestHeroContainerProps } from '../../../types';

  interface Props {
    props: GuestHeroContainerProps;
    onchange: (v: GuestHeroContainerProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestHeroContainerContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
