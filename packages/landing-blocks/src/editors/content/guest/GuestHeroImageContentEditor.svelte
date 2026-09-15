<script lang="ts">
  import type { GuestHeroImageProps } from '../../../types';

  interface Props {
    props: GuestHeroImageProps;
    onchange: (v: GuestHeroImageProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestHeroImageContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
