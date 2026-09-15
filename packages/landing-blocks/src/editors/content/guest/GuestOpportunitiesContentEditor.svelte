<script lang="ts">
  import type { GuestOpportunitiesProps } from '../../../types';

  interface Props {
    props: GuestOpportunitiesProps;
    onchange: (v: GuestOpportunitiesProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestOpportunitiesContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
