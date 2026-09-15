<script lang="ts">
  import type { GuestStoriesProps } from '../../../types';

  interface Props {
    props: GuestStoriesProps;
    onchange: (v: GuestStoriesProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestStoriesContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
