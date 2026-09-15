<script lang="ts">
  import type { GuestNewsletterProps } from '../../../types';

  interface Props {
    props: GuestNewsletterProps;
    onchange: (v: GuestNewsletterProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestNewsletterContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
