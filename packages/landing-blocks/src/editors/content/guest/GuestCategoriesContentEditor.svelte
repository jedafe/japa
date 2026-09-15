<script lang="ts">
  import type { GuestCategoriesProps } from '../../../types';

  interface Props {
    props: GuestCategoriesProps;
    onchange: (v: GuestCategoriesProps) => void;
  }

  let { props, onchange }: Props = $props();
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>GuestCategoriesContentEditor</p>
  <p>Props: {JSON.stringify(props)}</p>
</div>
